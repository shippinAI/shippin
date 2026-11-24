package hostd

import (
	"regexp"
	"strings"
)

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\x1b[()][A-Z0-9]`)
	urlRe  = regexp.MustCompile(`https://[^\s"'<>\x1b]+`)
)

// loginPrompt pulls only the verification URL (restricted to the provider's url_prefixes) and
// the one-time user code out of CLI output. Nothing else from the output ever leaves hostd.
func loginPrompt(p ConnectProvider, out string) (url, code string) {
	clean := ansiRe.ReplaceAllString(out, "")
	for _, u := range urlRe.FindAllString(clean, -1) {
		u = strings.TrimRight(u, ".,);")
		for _, pre := range p.URLPrefixes {
			if strings.HasPrefix(u, pre) && url == "" {
				url = u
			}
		}
	}
	if m := regexp.MustCompile(p.CodePattern).FindStringSubmatch(clean); m != nil {
		code = m[len(m)-1]
	}
	return url, code
}

// pasteCodeRe bounds what a user may paste back into a CLI that asks for a code.
var pasteCodeRe = regexp.MustCompile(`^[A-Za-z0-9#_.~-]{4,512}$`)
