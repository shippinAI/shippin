package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/ext"
	"shippin.cloud/console/internal/mcpl"
)

type extProbeIn struct{}

type extProbeOut struct {
	Module string `json:"module"`
}

// A module registered with internal/ext is served by the panel's own routes and listed in its manifest,
// with nothing in the panel naming it (docs/editions.md).
func TestAModuleActionIsServedByThePanel(t *testing.T) {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	d, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	g := mcpl.Register("shippin.public.ext_probe.get", mcpl.Def[extProbeIn, extProbeOut]{
		Spec: mcpl.Spec{Summary: "Answer the probe.", Access: mcpl.Public, Singleton: true},
		Run:  func(*mcpl.Call, extProbeIn) (extProbeOut, error) { return extProbeOut{Module: "probe"}, nil },
	})
	defer ext.Swap(ext.Module{Name: "probe", Actions: []mcpl.Registered{g}})()

	a := &API{cfg: testCfg(), db: d}
	mux := http.NewServeMux()
	a.mountActions(mux)
	method, path, _ := strings.Cut(g.Route, " ")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	var out extProbeOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); rec.Code != http.StatusOK || err != nil || out.Module != "probe" {
		t.Fatalf("%s: %d %s", g.Route, rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	a.mcplPublic(rec, httptest.NewRequest(http.MethodGet, "/v1/mcpl", nil))
	if !strings.Contains(rec.Body.String(), `"shippin.public.ext_probe.get"`) {
		t.Errorf("the manifest does not list the module's action: %s", rec.Body.String())
	}
}
