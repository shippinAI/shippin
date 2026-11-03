package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

// A fake Nomen: grants from the operator org, another org, another project and an inactive
// one; and a password-only user.
func fakeNomen(t *testing.T) *nomenStaff {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-nomen-orgid") != "op-org" || r.Header.Get("Authorization") != "Bearer team" {
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/management/v1/users/grants/_search":
			_, _ = w.Write([]byte(`{"result":[
			 {"id":"g1","userId":"u1","projectId":"p1","roleKeys":["staff.support"],"details":{"resourceOwner":"op-org"}},
			 {"id":"g2","userId":"u1","projectId":"p1","roleKeys":["staff.owner"],"details":{"resourceOwner":"customer-org"}},
			 {"id":"g3","userId":"u2","projectId":"p2","roleKeys":["staff.owner"],"details":{"resourceOwner":"op-org"}},
			 {"id":"g4","userId":"u3","projectId":"p1","state":"USER_GRANT_STATE_INACTIVE","roleKeys":["staff.owner"],"details":{"resourceOwner":"op-org"}}]}`))
		case "/v2/users/u1/authentication_methods":
			_, _ = w.Write([]byte(`{"authMethodTypes":["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]}`))
		case "/v2/users/u2/authentication_methods":
			_, _ = w.Write([]byte(`{"authMethodTypes":["AUTHENTICATION_METHOD_TYPE_PASSWORD"]}`))
		case "/v2/users/c9":
			_, _ = w.Write([]byte(`{"user":{"userId":"c9","details":{"resourceOwner":"customer-org"}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	f := auth.NewFlow(&config.Config{IDP: config.IDP{Issuer: srv.URL}}, nil)
	f.Client = srv.Client()
	return &nomenStaff{flow: f, tok: "team", org: "op-org", project: "p1"}
}

func TestNomenGrantsScopedToOperatorOrgAndProject(t *testing.T) {
	z, ctx := fakeNomen(t), context.Background()
	all, err := z.AllGrants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all["u1"].ID != "g1" {
		t.Fatalf("only the operator org's active grant on the staff project counts: %v", all)
	}
	if g, _ := z.Grant(ctx, "u1"); len(g.Roles) != 1 || g.Roles[0] != "staff.support" {
		t.Fatalf("a customer org's grant must not add staff.owner: %v", g)
	}
	if ok, _ := z.MFA(ctx, "u1"); !ok {
		t.Fatal("TOTP is a second factor")
	}
	if ok, _ := z.MFA(ctx, "u2"); ok {
		t.Fatal("a password alone is not MFA")
	}
	if u, err := z.User(ctx, "c9"); err != nil || u != nil {
		t.Fatal("users of other orgs can't be Team targets")
	}
	if _, err := (&nomenStaff{flow: z.flow}).Grant(ctx, "u1"); err != errNoTeamToken {
		t.Fatal("no team token: fail closed")
	}
}
