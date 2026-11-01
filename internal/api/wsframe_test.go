package api

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWsPopupAndDone(t *testing.T) {
	a := &API{cfg: testCfg()}
	a.cfg.ConsoleHost = "console.example.test"
	rec := httptest.NewRecorder()
	a.wsPopup(rec, httptest.NewRequest("GET", "https://dev-abc123.ws.example.test/_shippin/popup", nil))
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Host != "console.example.test" || loc.Path != "/auth/ws-login" ||
		loc.Query().Get("rd") != "https://dev-abc123.ws.example.test/_shippin/done" {
		t.Fatalf("popup redirect %q", rec.Header().Get("Location"))
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://dev-abc123.ws.example.test/_shippin/done", nil)
	req.Host = `x"</script><script>alert(1)//.ws.example.test`
	a.wsDone(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `"https://console.example.test")`) || strings.Contains(body, "<script>alert") {
		t.Fatalf("done page not escaped / wrong origin: %s", body)
	}
}

func TestWsStatusWithoutCookie(t *testing.T) {
	a := &API{cfg: testCfg()}
	a.cfg.ConsoleHost = "console.example.test"
	rec := httptest.NewRecorder()
	a.wsStatus(rec, httptest.NewRequest("GET", "https://dev-abc123.ws.example.test/_shippin/status", nil))
	if rec.Code != 401 || rec.Header().Get("Access-Control-Allow-Origin") != "https://console.example.test" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("status %d headers %v", rec.Code, rec.Header())
	}
}
