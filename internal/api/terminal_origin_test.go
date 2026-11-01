package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/config"
)

// F-1 / B4-3: the terminal refuses a foreign, null or missing Origin before anything else runs
// (no tenant lookup, no audit row, no hostd dial: the API below has no DB or hostd at all).
func TestTerminalOriginFirst(t *testing.T) {
	a := &API{cfg: &config.Config{ConsoleHost: "console.shippin.cloud"}}
	for _, o := range []string{"", "null", "https://evil--site.ws.shippin.cloud", "https://console.shippin.cloud.evil.example", "http://console.shippin.cloud"} {
		r := httptest.NewRequest(http.MethodGet, "https://console.shippin.cloud/v1/tenants/t/workspaces/w/terminal", nil)
		if o != "" {
			r.Header.Set("Origin", o)
		}
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		w := httptest.NewRecorder()
		a.terminalSocket(w, r) // would panic on the nil DB/hostd if it got past the origin check
		if w.Code != http.StatusForbidden {
			t.Errorf("origin %q: got %d, want 403", o, w.Code)
		}
	}
}
