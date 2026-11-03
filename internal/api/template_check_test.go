package api

import (
	"strings"
	"testing"

	"shippin.cloud/console/internal/db"
)

func goodTemplate() db.Template {
	return db.Template{Slug: "node-vite", Name: "Node", Image: "shippin/tpl-node", CPU: 2, MemoryGiB: 4, DiskGiB: 20,
		Ports: []int{5173}, Repo: "https://github.com/vitejs/vite-plugin-react"}
}

func TestCheckTemplate(t *testing.T) {
	c := testWS()
	g := goodTemplate()
	if msg := checkTemplate(&g, c); msg != "" {
		t.Fatalf("good template rejected: %s", msg)
	}
	if g.Icon != "box" || g.Toolchain == nil {
		t.Fatal("defaults not filled")
	}
	mut := map[string]func(*db.Template){
		"slug caps":     func(t *db.Template) { t.Slug = "Node" },
		"no name":       func(t *db.Template) { t.Name = " " },
		"foreign image": func(t *db.Template) { t.Image = "images:debian/13" },
		"image escape":  func(t *db.Template) { t.Image = "shippin/../x y" },
		"cpu":           func(t *db.Template) { t.CPU = 33 },
		"disk small":    func(t *db.Template) { t.DiskGiB = 1 },
		"repo host":     func(t *db.Template) { t.Repo = "https://evil.example/x/y" },
		"repo creds":    func(t *db.Template) { t.Repo = "https://u:p@github.com/x/y" },
		"dup ports":     func(t *db.Template) { t.Ports = []int{5173, 5173} },
		"ide port":      func(t *db.Template) { t.Ports = []int{3000} },
		"startup big":   func(t *db.Template) { t.Startup = strings.Repeat("x", 16385) },
		"icon":          func(t *db.Template) { t.Icon = "<svg>" },
		"too many port": func(t *db.Template) {
			t.Ports = []int{2001, 2002, 2003, 2004, 2005, 2006, 2007, 2008, 2009, 2010, 2011}
		},
	}
	for name, m := range mut {
		x := goodTemplate()
		m(&x)
		if checkTemplate(&x, c) == "" {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParseSeed(t *testing.T) {
	raw := []byte(`[
	 {"slug":"node-vite","name":"Node","image":"shippin/tpl-node","cpu":2,"memory_gib":4,"disk_gib":20,"ports":[5173]},
	 {"slug":"node-vite","name":"Dup","image":"shippin/tpl-node","cpu":2,"memory_gib":4,"disk_gib":20},
	 {"slug":"bad","name":"Bad","image":"docker.io/x","cpu":2,"memory_gib":4,"disk_gib":20},
	 {"id":"01a1424a-d04a-734f-abdb-9f8195327f66","slug":"blank","name":"Blank","image":"shippin/tpl-blank","cpu":2,"memory_gib":4,"disk_gib":20}
	]`)
	ts, errs := parseSeed(raw, testWS())
	if len(ts) != 2 || len(errs) != 2 {
		t.Fatalf("got %d templates, %d errors; want 2, 2 (%v)", len(ts), len(errs), errs)
	}
	if ts[0].ID == "" || ts[1].ID != "01a1424a-d04a-734f-abdb-9f8195327f66" {
		t.Fatal("ids: generated for missing, kept when valid")
	}
	if _, errs := parseSeed([]byte("{"), testWS()); len(errs) != 1 {
		t.Fatal("broken JSON must be reported")
	}
}

// The shipped seed file must pass the same validation the operator API applies.
func TestShippedSeedIsValid(t *testing.T) {
	raw := mustRead(t, "../../config/templates.json")
	ts, errs := parseSeed(raw, testWS())
	if len(errs) > 0 || len(ts) < 4 {
		t.Fatalf("seed: %d templates, errors %v", len(ts), errs)
	}
}
