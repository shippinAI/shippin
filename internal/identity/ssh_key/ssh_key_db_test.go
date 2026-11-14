package ssh_key_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

func newKey(t *testing.T) string {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k)))
}

const base = "/v1/me/identity/ssh-keys"

func TestSSHKeyActions(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.identity.ssh_key.", mcpltest.As{Actor: "ada"})
	key := newKey(t)
	body, _ := json.Marshal(map[string]string{"name": "laptop", "public_key": key})
	k := h.Call("POST", base, string(body), 201)
	if k["fingerprint"] == "" || !strings.HasPrefix(k["fingerprint"].(string), "SHA256:") || k["name"] != "laptop" {
		t.Fatalf("created: %v", k)
	}
	h.Call("POST", base, string(body), 409)
	h.Call("POST", base, `{"name":"bad","public_key":"not a key"}`, 400)
	h.Call("POST", base, `{"Name":"old casing","PublicKey":"`+key+`"}`, 400) // fields are named exactly once
	if items := h.Call("GET", base, "", 200)["items"].([]any); len(items) != 1 {
		t.Fatalf("list: %v", items)
	}

	// Someone else neither sees nor removes it.
	h.As.Actor = "bob"
	if items := h.Call("GET", base, "", 200)["items"].([]any); len(items) != 0 {
		t.Fatalf("bob sees ada's key: %v", items)
	}
	h.Call("DELETE", base+"/"+k["id"].(string), "", 404)

	h.As.Actor = "ada"
	h.Call("DELETE", base+"/"+k["id"].(string), "", 204)
	h.Call("DELETE", base+"/"+k["id"].(string), "", 404)
	if len(h.Events) != 2 || h.Events[0].ID != "shippin.identity.ssh_key.created" || h.Events[0].Actor != "ada" ||
		h.Events[1].ID != "shippin.identity.ssh_key.deleted" || h.Events[1].Subject["sshKeyId"] != k["id"] {
		t.Fatalf("events: %+v", h.Events)
	}
	if len(h.Audits) != 2 {
		t.Fatalf("audits: %q", h.Audits)
	}
}
