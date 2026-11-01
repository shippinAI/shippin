package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// staffAPI is an API on fakes; who returns the caller for a request (by X-Test-User).
func staffAPI(store *fakeStore, idp *fakeIdP) *API {
	a := &API{cfg: testCfg(), staffDB: store, staffIdP: idp}
	a.cfg.Access.LogWindow = "1ns"
	a.flow = &auth.Flow{}
	a.resolveCaller = func(_ http.ResponseWriter, r *http.Request) (*caller, bool) {
		u := r.Header.Get("X-Test-User")
		c := &caller{id: &auth.Identity{Sub: u, Name: "User " + u}}
		if r.Header.Get("X-Test-IAM") != "" {
			c.op = "op_superadmin"
		}
		return c, true
	}
	return a
}

func testID(sub string) *auth.Identity { return &auth.Identity{Sub: sub, Name: "User " + sub} }
