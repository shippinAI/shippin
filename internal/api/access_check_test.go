package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const tid = "01a14263-479a-7d81-80f1-4afca958c629"

func TestCheckRequest(t *testing.T) {
	cfg := testCfg().Access
	bad := map[string]accessIn{
		"no tenant":     {Reason: "debugging a failed build", Duration: "1h"},
		"bad ws":        {TenantID: tid, WorkspaceID: "../x", Reason: "debugging a failed build", Duration: "1h"},
		"short reason":  {TenantID: tid, Reason: "  pls  ", Duration: "1h"},
		"long reason":   {TenantID: tid, Reason: strings.Repeat("a", 501), Duration: "1h"},
		"control char":  {TenantID: tid, Reason: "debugging \x1b[2Ja build", Duration: "1h"},
		"bidi override": {TenantID: tid, Reason: "debugging \u202e a build", Duration: "1h"},
		"odd duration":  {TenantID: tid, Reason: "debugging a failed build", Duration: "2h"},
		"raw duration":  {TenantID: tid, Reason: "debugging a failed build", Duration: "3600s"},
	}
	for name, in := range bad {
		if _, msg := checkRequest(cfg, &in); msg == "" {
			t.Errorf("%s: accepted", name)
		}
	}
	in := accessIn{TenantID: tid, Reason: "  Customer ticket #42:\nbuild fails  ", Duration: "4h"}
	if d, msg := checkRequest(cfg, &in); msg != "" || d != 4*time.Hour || strings.HasPrefix(in.Reason, " ") {
		t.Fatalf("valid request refused: %q %v", msg, d)
	}
}

func TestCheckBreakGlass(t *testing.T) {
	cfg, now := testCfg().Access, time.Now()
	ok := func() accessIn {
		return accessIn{TenantID: tid, Category: "security_incident", Justification: "Active credential stuffing from this tenant"}
	}
	fresh := now.Add(-2 * time.Minute)
	cases := map[string]struct {
		mod    func(*accessIn)
		authAt time.Time
		reauth bool
	}{
		"unknown category":       {func(i *accessIn) { i.Category = "curiosity" }, fresh, false},
		"short justification":    {func(i *accessIn) { i.Justification = "because" }, fresh, false},
		"withhold not legal":     {func(i *accessIn) { i.Withhold = true }, fresh, false},
		"bearer token (no time)": {func(*accessIn) {}, time.Time{}, true},
		"stale login":            {func(*accessIn) {}, now.Add(-11 * time.Minute), true},
		"future login":           {func(*accessIn) {}, now.Add(5 * time.Minute), true},
	}
	for name, c := range cases {
		in := ok()
		c.mod(&in)
		_, msg, reauth := checkBreakGlass(cfg, &in, c.authAt, now)
		if msg == "" || reauth != c.reauth {
			t.Errorf("%s: msg=%q reauth=%v", name, msg, reauth)
		}
	}
	in := ok()
	if cat, msg, _ := checkBreakGlass(cfg, &in, fresh, now); msg != "" || cat.ID != "security_incident" {
		t.Fatalf("valid activation refused: %q", msg)
	}
	in = ok()
	in.Category, in.Withhold = "legal_hold", true
	if _, msg, _ := checkBreakGlass(cfg, &in, fresh, now); msg != "" {
		t.Fatalf("legal hold withhold refused: %q", msg)
	}
}

func TestNoticeAt(t *testing.T) {
	cats, now := testCfg().Access.BreakGlass, time.Now()
	hold, _ := cats.Category("legal_hold")
	sec, _ := cats.Category("security_incident")
	if !noticeAt(hold, true, now).Equal(now.Add(720*time.Hour)) || !noticeAt(hold, false, now).Equal(now) || !noticeAt(sec, true, now).Equal(now) {
		t.Fatal("only a legal-hold activation that asks may delay the notice, by the configured delay")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.10:5555"
	if clientIP(r) != "203.0.113.10" {
		t.Fatal("peer address")
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 203.0.113.10")
	if clientIP(r) != "203.0.113.7" {
		t.Fatal("first forwarded hop")
	}
	r.Header.Set("X-Forwarded-For", "<script>")
	if clientIP(r) != "203.0.113.10" {
		t.Fatal("garbage forwarded header must fall back to the peer")
	}
}

func TestSeenCacheDedupes(t *testing.T) {
	var s seenCache
	now := time.Now()
	if !s.due("k", time.Minute, now) || s.due("k", time.Minute, now.Add(30*time.Second)) || !s.due("k", time.Minute, now.Add(61*time.Second)) {
		t.Fatal("one entry per window")
	}
	s.forget("k")
	if !s.due("k", time.Minute, now.Add(62*time.Second)) {
		t.Fatal("a failed write must be retried")
	}
}
