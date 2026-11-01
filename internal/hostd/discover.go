package hostd

import (
	"net/http"
	"path"
	"sort"
	"strings"
)

// discoverScript lists candidate project folders under /home/dev, read-only: names, manifest file
// names and the git origin URL only (no file contents beyond `git config --get`). Depth <= 6; a
// folder holding .git is a root: it is reported and not descended into (nested repos such as
// box-work/projects/* are found because box-work itself has no .git). Hidden dirs, node_modules,
// vendor, .cache, .venv and dist are skipped. At most 400 lines. One line per hit: KIND\tPATH[\tVALUE].
const discoverScript = `cd /home/dev 2>/dev/null || exit 0
find . -mindepth 1 -maxdepth 6 \( -type d \( -name node_modules -o -name vendor -o -name .cache -o -name .venv -o -name dist \) -prune \) \
  -o \( -type d -name .git -print -prune \) -o \( -type d -name '.*' -prune \) \
  -o \( -type d -exec test -e '{}/../.git' \; -prune \) \
  -o \( -type f \( -name package.json -o -name go.mod -o -name pyproject.toml -o -name requirements.txt -o -name Cargo.toml \
     -o -name Gemfile -o -name composer.json -o -name pom.xml -o -name build.gradle -o -name deno.json \) -print \) 2>/dev/null |
  head -400 | while read -r f; do
    d=$(dirname "$f"); b=$(basename "$f")
    if [ "$b" = .git ]; then printf 'git\t%s\t%s\n' "$d" "$(git config -f "$f/config" --get remote.origin.url 2>/dev/null)"
    else printf 'manifest\t%s\t%s\n' "$d" "$b"; fi
  done`

// Found is one suggested project folder.
type Found struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Git       bool     `json:"git"`
	Remote    string   `json:"remote,omitempty"`
	Manifests []string `json:"manifests"`
}

// parseDiscover folds the script's lines into one entry per folder. A folder nested inside a git
// repo that is itself a candidate is dropped (the repo is the project).
func parseDiscover(out string) []Found {
	by := map[string]*Found{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 3)
		if len(f) < 2 || f[1] == "" || strings.Contains(f[1], "..") {
			continue
		}
		p := path.Clean("/home/dev/" + strings.TrimPrefix(f[1], "./"))
		if p == "/home/dev" {
			continue
		}
		e := by[p]
		if e == nil {
			e = &Found{Name: path.Base(p), Path: p, Manifests: []string{}}
			by[p] = e
		}
		val := ""
		if len(f) == 3 {
			val = f[2]
		}
		switch f[0] {
		case "git":
			e.Git, e.Remote = true, scrubRemote(val)
		case "manifest":
			e.Manifests = append(e.Manifests, val)
		}
	}
	out2 := []Found{}
	for p, e := range by {
		if insideRepo(p, by) {
			continue
		}
		sort.Strings(e.Manifests)
		out2 = append(out2, *e)
	}
	sort.Slice(out2, func(i, j int) bool { return out2[i].Path < out2[j].Path })
	return out2
}

func insideRepo(p string, by map[string]*Found) bool {
	for d := path.Dir(p); d != "/home/dev" && d != "/" && d != "."; d = path.Dir(d) {
		if e := by[d]; e != nil && e.Git {
			return true
		}
	}
	return false
}

// scrubRemote drops credentials from a remote URL (https://user:token@host/... -> https://host/...).
func scrubRemote(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		if at := strings.LastIndex(u[i+3:], "@"); at >= 0 && !strings.Contains(u[i+3:i+3+at], "/") {
			return u[:i+3] + u[i+3+at+1:]
		}
	}
	return u
}

// discover answers GET /tenants/{t}/workspaces/{w}/discover.
func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	at, err := s.placeOf(r.Context(), r.PathValue("t"), r.PathValue("w"))
	if err != nil {
		fail(w, http.StatusConflict, "workspace is not reachable")
		return
	}
	out, err := at.do(r.Context(), "exec", Instance(r.PathValue("w")), "--user", "1000", "--group", "1000", "--", "sh", "-c", discoverScript)
	if err != nil {
		fail(w, http.StatusConflict, "workspace is not running")
		return
	}
	reply(w, map[string]any{"items": parseDiscover(out)})
}
