package api

import "net/http"

// Carries says what a transport route carries instead of an action. Actions are files under
// internal/<domain>/<entity>/<verb>.go (docs/mcpl.md); a route that is not one says why, where it is
// mounted, so nothing served is left unaccounted for.
type Carries string

const (
	Stream      Carries = "stream"       // server-sent events
	Socket      Carries = "socket"       // a WebSocket
	Redirect    Carries = "redirect"     // a browser step in a sign-in or connect flow
	Webhook     Carries = "webhook"      // another system calling in, signed by it
	ForwardAuth Carries = "forward-auth" // the edge asking whether a request to a published site may pass
	File        Carries = "file"         // a download that isn't JSON
	Protocol    Carries = "protocol"     // another protocol's endpoint, such as MCP
)

// transport mounts a route that is not an action and records what it carries.
func (a *API) transport(mux Router, pattern string, carries Carries, h http.HandlerFunc) {
	if a.transports == nil {
		a.transports = map[string]Carries{}
	}
	a.transports[pattern] = carries
	mux.HandleFunc(pattern, h)
}
