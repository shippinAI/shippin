package auth

import "testing"

// Only a console sign-in session may authenticate the API; workspace-host sessions (whose cookie
// is presented to workspace hosts) and one-time codes must never be accepted as console sessions.
func TestSessionKinds(t *testing.T) {
	console := &Session{AccessToken: "t"}
	ws := &Session{AccessToken: "t", ReturnTo: "dev-1.ws.example.test"}
	code := &Session{AccessToken: "t", ReturnTo: "dev-1.ws.example.test", State: "code"}
	pending := &Session{Verifier: "v", State: "s", ReturnTo: "/"}
	var none *Session
	if !console.Console() || ws.Console() || code.Console() || pending.Console() || none.Console() {
		t.Fatal("Console(): only plain sign-in sessions")
	}
	if !ws.Workspace("dev-1.ws.example.test") || ws.Workspace("dev-2.ws.example.test") || code.Workspace("dev-1.ws.example.test") ||
		console.Workspace("") || none.Workspace("x") {
		t.Fatal("Workspace(host): only sessions bound to that exact host, never codes")
	}
}
