package agenthub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const actionHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const policyHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const ticketHash = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

func TestAuthorizeBindsCurrentReviewedActionAndPolicy(t *testing.T) {
	var policyReads, ticketReads int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/policy/doc":
			policyReads++
			if r.URL.Query().Get("tenant") != "tenant-1" {
				t.Errorf("policy tenant = %q", r.URL.Query().Get("tenant"))
			}
			_, _ = w.Write([]byte(policyJSON))
		case "/api/v1/change-tickets/ticket-1":
			ticketReads++
			_, _ = w.Write([]byte(ticketJSON))
		case "/api/v1/policy/decide":
			var in decisionRequest
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Errorf("decode decision request: %v", err)
			}
			if in != (decisionRequest{Tool: "shippin:guest.start", Command: `{"resource":"guest-9","generation":4}`, Machine: "machine-1", TicketID: "ticket-1"}) {
				t.Errorf("decision request = %+v", in)
			}
			_, _ = w.Write([]byte(decisionJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	c, err := newClient(Config{BaseURL: server.URL, Token: "service-token"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Authorize(context.Background(), Request{TenantID: "tenant-1", MachineID: "machine-1", TicketID: "ticket-1",
		Tool: "shippin:guest.start", Command: `{"resource":"guest-9","generation":4}`, ActionSHA256: actionHash})
	if err != nil {
		t.Fatal(err)
	}
	if got.DecisionID != "decision-1" || got.TicketSHA256 != ticketHash || got.PolicySHA256 != policyHash || got.PolicyVersion != 2 || len(got.MatchedRules) != 1 || got.MatchedRules[0] != "review-provider-actions" {
		t.Fatalf("unexpected authorization provenance: %+v", got)
	}
	if policyReads != 2 || ticketReads != 2 {
		t.Fatalf("expected policy and ticket to be read before and after decision, got policy=%d ticket=%d", policyReads, ticketReads)
	}
}

func TestAuthorizeFailsClosed(t *testing.T) {
	cases := []struct {
		name                       string
		policy                     string
		ticket                     string
		decision                   string
		changePolicyAfterFirstRead bool
		changeTicketAfterFirstRead bool
	}{
		{name: "no matched rules", policy: policyJSON, ticket: ticketJSON, decision: strings.Replace(decisionJSON, `"matched":["review-provider-actions"]`, `"matched":[]`, 1)},
		{name: "matched rule lacks human approval", policy: strings.Replace(policyJSON, `"effect":"require","require":[{"check":"approval"},{"check":"review","min":2,"role":"reviewer"}]`, `"effect":"require","require":[{"check":"review","min":2,"role":"reviewer"}]`, 1), ticket: ticketJSON, decision: decisionJSON},
		{name: "wrong system", policy: policyJSON, ticket: strings.Replace(ticketJSON, `"machine":"machine-1"`, `"machine":"other-machine"`, 1), decision: decisionJSON},
		{name: "wrong action hash", policy: policyJSON, ticket: strings.Replace(ticketJSON, actionHash, strings.Repeat("b", 64), 1), decision: decisionJSON},
		{name: "policy changed after decision", policy: policyJSON, ticket: ticketJSON, decision: decisionJSON, changePolicyAfterFirstRead: true},
		{name: "ticket changed after decision", policy: policyJSON, ticket: ticketJSON, decision: decisionJSON, changeTicketAfterFirstRead: true},
		{name: "top-level decision denied", policy: policyJSON, ticket: ticketJSON, decision: strings.Replace(decisionJSON, `"allow":true`, `"allow":false`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var policyReads, ticketReads int
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer service-token" {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/policy/doc":
					policyReads++
					body := tc.policy
					if tc.changePolicyAfterFirstRead && policyReads > 1 {
						body = strings.Replace(tc.policy, `"sha256":"`+policyHash+`"`, `"sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"`, 1)
					}
					_, _ = w.Write([]byte(body))
				case "/api/v1/change-tickets/ticket-1":
					ticketReads++
					body := tc.ticket
					if tc.changeTicketAfterFirstRead && ticketReads > 1 {
						body = strings.Replace(tc.ticket, `"sha256":"`+ticketHash+`"`, `"sha256":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"`, 1)
					}
					_, _ = w.Write([]byte(body))
				case "/api/v1/policy/decide":
					_, _ = w.Write([]byte(tc.decision))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			c, err := newClient(Config{BaseURL: server.URL, Token: "service-token"}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Authorize(context.Background(), Request{TenantID: "tenant-1", MachineID: "machine-1", TicketID: "ticket-1",
				Tool: "shippin:guest.start", Command: `{"resource":"guest-9","generation":4}`, ActionSHA256: actionHash})
			if err == nil {
				t.Fatal("unsafe or changed authorization was accepted")
			}
		})
	}
}

func TestNewClientRejectsUnsafeConfiguration(t *testing.T) {
	for _, cfg := range []Config{
		{BaseURL: "http://agenthub.example", Token: "secret"},
		{BaseURL: "https://user:pass@agenthub.example", Token: "secret"},
		{BaseURL: "https://agenthub.example?next=elsewhere", Token: "secret"},
		{BaseURL: "https://agenthub.example", Token: ""},
	} {
		if _, err := NewClient(cfg); err == nil {
			t.Fatalf("unsafe config was accepted: %+v", cfg)
		}
	}
}

func TestCheckPolicyReadsOnlyExplicitTenantPolicy(t *testing.T) {
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/policy/doc" || r.URL.Query().Get("tenant") != "tenant-1" {
			t.Errorf("unexpected connection check request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer service-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(policyJSON))
	}))
	defer server.Close()
	c, err := newClient(Config{BaseURL: server.URL, Token: "service-token"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CheckPolicy(context.Background(), "tenant-1")
	if err != nil || got.TenantID != "tenant-1" || got.Version != 2 || got.SHA256 != policyHash || calls != 1 {
		t.Fatalf("policy check=%+v calls=%d err=%v", got, calls, err)
	}
	if _, err := c.CheckPolicy(context.Background(), "bad tenant"); err == nil || calls != 1 {
		t.Fatalf("invalid tenant should fail locally without another call, calls=%d err=%v", calls, err)
	}
}

const policyJSON = `{"tenant_id":"tenant-1","version":2,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","doc":{"version":2,"rules":[{"id":"review-provider-actions","enabled":true,"effect":"require","require":[{"check":"approval"},{"check":"review","min":2,"role":"reviewer"}]}]}}`

const ticketJSON = `{"ticket":{"id":"ticket-1","tenant_id":"tenant-1","machine":"machine-1","command":"{\"resource\":\"guest-9\",\"generation\":4}","fields":{"shippin_action_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","status":"passed"},"evidence":[],"reviews":[]}`

const decisionJSON = `{"decision_id":"decision-1","tenant_id":"tenant-1","policy_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","allow":true,"decision":{"allow":true,"policy_version":2,"matched":["review-provider-actions"]}}`
