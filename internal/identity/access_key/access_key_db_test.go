package access_key_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/identity/access_key"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

const base = "/v1/me/identity/access-keys"

func TestAccessKeys(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.identity.access_key.", mcpltest.As{})
	ctx := context.Background()
	uid := uuid.NewString()
	if _, err := h.Pool.Exec(ctx, `INSERT INTO identity_users(id, email, name, role, active) VALUES($1, 'ada@example.test', 'Ada', 'member', true)`, uid); err != nil {
		t.Fatal(err)
	}

	h.As.Actor = "idp-subject"
	h.Call("POST", base, `{"name":"claude"}`, 403) // keys are for this directory's accounts

	h.As.Actor, h.As.Via = "local:"+uid, "passkey"
	h.Call("POST", base, `{"name":""}`, 400)
	h.Call("POST", base, `{"name":"claude","days":91}`, 400)
	k := h.Call("POST", base, `{"name":"claude desktop","days":7}`, 201)
	token, _ := k["token"].(string)
	if !strings.HasPrefix(token, access_key.Prefix) || !strings.HasPrefix(token, k["prefix"].(string)) {
		t.Fatalf("created: %v", k)
	}
	items := h.Call("GET", base, "", 200)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["token"] != nil {
		t.Fatalf("list shows the key itself or the wrong keys: %v", items)
	}

	// The key acts as Ada, with the method of the session that made it.
	user, method, ok := access_key.Resolve(ctx, h.Pool, token)
	if !ok || user != uid || method != "passkey" {
		t.Fatalf("resolve: %q %q %v", user, method, ok)
	}
	if _, _, ok := access_key.Resolve(ctx, h.Pool, token+"x"); ok {
		t.Fatal("a wrong key resolved")
	}

	// A key can't mint another.
	h.As.Via = "access_key"
	h.Call("POST", base, `{"name":"another"}`, 403)

	// Someone else can't revoke it; a deactivated account's key stops; revoking ends it.
	h.As.Actor, h.As.Via = "local:"+uuid.NewString(), "password"
	h.Call("DELETE", base+"/"+k["id"].(string), "", 404)
	_, _ = h.Pool.Exec(ctx, `UPDATE identity_users SET active=false WHERE id=$1`, uid)
	if _, _, ok := access_key.Resolve(ctx, h.Pool, token); ok {
		t.Fatal("a deactivated account's key still works")
	}
	_, _ = h.Pool.Exec(ctx, `UPDATE identity_users SET active=true WHERE id=$1`, uid)
	h.As.Actor = "local:" + uid
	h.Call("DELETE", base+"/"+k["id"].(string), "", 204)
	if _, _, ok := access_key.Resolve(ctx, h.Pool, token); ok {
		t.Fatal("a revoked key still works")
	}
	_, _ = h.Pool.Exec(ctx, `INSERT INTO access_keys(id, user_id, name, prefix, hash, method, expires_at) VALUES($1, $2, 'old', 'shp_x', 'h', 'password', now() - interval '1 day')`, uuid.NewString(), uid)
	if len(h.Events) != 2 || h.Events[0].ID != "shippin.identity.access_key.created" || h.Events[1].ID != "shippin.identity.access_key.revoked" {
		t.Fatalf("events: %+v", h.Events)
	}
}
