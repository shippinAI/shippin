package mcpl

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// plumbing is what every context shares and none owns (docs/architecture.md): the composition root,
// the action runner and its generated registry, the extension seam, the host ports, the settings
// sections and the two databases. hv is the Infrastructure context's provider drivers.
var plumbing = []string{"actions", "app", "clickhouse", "db", "ext", "hv", "mcpl", "ports", "settings"}

// TestInternalPackagesAreContextsOrListed keeps internal/ to the contexts and the plumbing. A package
// from before the code map is listed in testdata/unplaced-packages.txt until it moves into its
// context; that list only shrinks. New code is an entity folder inside a context
// (internal/<domain>/<entity>/), not another package beside them.
func TestInternalPackagesAreContextsOrListed(t *testing.T) {
	raw, err := os.ReadFile("testdata/unplaced-packages.txt")
	if err != nil {
		t.Fatal(err)
	}
	var unplaced []string
	for _, l := range strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			unplaced = append(unplaced, l)
		}
	}
	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("internal/%s: a file beside the packages; it belongs in one", e.Name())
			continue
		}
		have = append(have, e.Name())
		switch {
		case slices.Contains(Domains, e.Name()), slices.Contains(plumbing, e.Name()):
			if slices.Contains(unplaced, e.Name()) {
				t.Errorf("testdata/unplaced-packages.txt lists %q, which is a context or plumbing; remove the line", e.Name())
			}
		case !slices.Contains(unplaced, e.Name()):
			t.Errorf("internal/%s is a new top-level package. Put it inside its context as internal/<domain>/<entity>/ "+
				"(docs/architecture.md); testdata/unplaced-packages.txt only shrinks", e.Name())
		}
	}
	for _, u := range unplaced {
		if !slices.Contains(have, u) {
			t.Errorf("testdata/unplaced-packages.txt lists %q, which is gone; remove the line", u)
		}
	}
	if !slices.IsSorted(unplaced) {
		t.Error("testdata/unplaced-packages.txt is not sorted")
	}
}
