package hostd

import (
	"context"
	"testing"
)

func TestDropFirewallOnlyThisWorkspace(t *testing.T) {
	acl := `{"ingress":[{"action":"allow","protocol":"tcp","source":"198.51.100.10","destination":"10.128.3.9","destination_port":"7682","description":"ws-` + wsID + `"},` +
		`{"action":"allow","protocol":"tcp","source":"198.51.100.10","destination":"10.128.3.8","destination_port":"7682","description":"ws-other"}]}`
	f := useFake(t, fakeReply{match: "/1.0/network-acls/", out: acl})
	if err := (&Server{cfg: &Config{}}).dropFirewall(context.Background(), tid, wsID); err != nil {
		t.Fatal(err)
	}
	if f.find("acl rule remove", "destination=10.128.3.9") == nil || f.find("acl rule remove", "destination=10.128.3.8") != nil {
		t.Fatalf("must remove exactly this workspace's rules: %v", f.calls)
	}
}
