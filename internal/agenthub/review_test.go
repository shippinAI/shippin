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
		{name: "matched rule lacks reviewer quorum", policy: strings.Replace(policyJSON, `,{"check":"review","min":2,"role":"reviewer"}`, "", 1), ticket: ticketJSON, decision: decisionJSON},
		{name: "matched rule has no positive reviewer count", policy: strings.Replace(policyJSON, `"min":2`, `"min":0`, 1), ticket: ticketJSON, decision: decisionJSON},
		{name: "matched rule has no reviewer role", policy: strings.Replace(policyJSON, `,"role":"reviewer"`, "", 1), ticket: ticketJSON, decision: decisionJSON},
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

func TestRequiresHumanReviewRequiresQuorumOnEveryMatchedRule(t *testing.T) {
	withQuorum := `{"id":"one","enabled":true,"effect":"require","require":[{"check":"approval"},{"check":"review","min":2,"role":"reviewer"}]}`
	withoutQuorum := `{"id":"two","enabled":true,"effect":"require","require":[{"check":"approval"}]}`
	for _, tc := range []struct {
		name    string
		rules   string
		matched []string
		want    bool
	}{
		{name: "one explicit human quorum", rules: withQuorum, matched: []string{"one"}, want: true},
		{name: "all matched rules must require quorum", rules: withQuorum + "," + withoutQuorum, matched: []string{"one", "two"}},
		{name: "unknown match rejected", rules: withQuorum, matched: []string{"missing"}},
		{name: "permissive match rejected", rules: withQuorum + `,{"id":"audit","enabled":true,"effect":"audit"}`, matched: []string{"one", "audit"}},
		{name: "no matches rejected", rules: withQuorum, matched: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(`{"rules":[` + tc.rules + `]}`)
			if got := requiresHumanReview(raw, tc.matched); got != tc.want {
				t.Fatalf("requiresHumanReview() = %v, want %v", got, tc.want)
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

func TestCheckSystemMappingUsesMachineAndTenantDryRunWithoutTicket(t *testing.T) {
	var policyReads, machineReads, decisions int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer service-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/policy/doc":
			policyReads++
			if r.URL.Query().Get("tenant") != "tenant-1" {
				t.Errorf("unexpected policy tenant: %q", r.URL.Query().Get("tenant"))
			}
			_, _ = w.Write([]byte(policyJSON))
		case "/api/v1/machines/machine-1":
			machineReads++
			_, _ = w.Write([]byte(`{"id":"machine-1"}`))
		case "/api/v1/policy/decide":
			decisions++
			var in decisionRequest
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Errorf("decode mapping dry-run: %v", err)
			}
			if in.Machine != "machine-1" || in.TicketID != "" || !in.DryRun || in.Tool != "shippin:identity.mapping.validate" {
				t.Errorf("mapping request was not read-only: %+v", in)
			}
			_, _ = w.Write([]byte(decisionJSON))
		default:
			t.Errorf("unexpected mapping check endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := newClient(Config{BaseURL: server.URL, Token: "service-token"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CheckSystemMapping(context.Background(), "tenant-1", "machine-1", "019a0000-0000-7000-8000-0000000000a1")
	if err != nil || got.TenantID != "tenant-1" || got.MachineID != "machine-1" || got.PolicyVersion != 2 || got.PolicySHA256 != policyHash {
		t.Fatalf("mapping=%+v err=%v", got, err)
	}
	if policyReads != 2 || machineReads != 1 || decisions != 1 {
		t.Fatalf("expected stable policy + machine lookup + one dry-run, got policy=%d machine=%d decide=%d", policyReads, machineReads, decisions)
	}
}

func TestCheckSystemMappingRejectsCrossTenantDryRun(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/policy/doc":
			_, _ = w.Write([]byte(policyJSON))
		case "/api/v1/machines/machine-1":
			_, _ = w.Write([]byte(`{"id":"machine-1"}`))
		case "/api/v1/policy/decide":
			_, _ = w.Write([]byte(strings.Replace(decisionJSON, `"tenant_id":"tenant-1"`, `"tenant_id":"tenant-2"`, 1)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := newClient(Config{BaseURL: server.URL, Token: "service-token"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CheckSystemMapping(context.Background(), "tenant-1", "machine-1", "system-1"); err == nil {
		t.Fatal("cross-tenant machine mapping was accepted")
	}
}

func TestTicketLifecycleUsesIdempotencyAndBindsExactDraft(t *testing.T) {
	command := `{"operation":"deployment.apply","deployment_id":"deploy-1","generation":4,"action_sha256":"` + actionHash + `"}`
	created := `{"id":"ticket-1","tenant_id":"tenant-1","machine":"machine-1","command":` + mustJSONString(t, command) + `,"fields":{"shippin_action_sha256":"` + actionHash + `"},"sha256":"` + ticketHash + `","status":"draft","version":1}`
	withEvidence := `{"id":"ticket-1","tenant_id":"tenant-1","machine":"machine-1","command":` + mustJSONString(t, command) + `,"fields":{"shippin_action_sha256":"` + actionHash + `"},"sha256":"` + policyHash + `","status":"draft","version":2}`
	submitted := `{"id":"ticket-1","tenant_id":"tenant-1","machine":"machine-1","command":` + mustJSONString(t, command) + `,"fields":{"shippin_action_sha256":"` + actionHash + `"},"sha256":"` + strings.Repeat("d", 64) + `","status":"pending_review","version":3}`
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if len(r.Header.Get("Idempotency-Key")) < 16 {
			t.Errorf("missing idempotency key on %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		calls++
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/change-tickets":
			var in TicketDraft
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Errorf("decode create: %v", err)
			}
			if in.ID != "ticket-1" || in.Machine != "machine-1" || in.TenantID != "" || in.Command != command || in.Fields["shippin_action_sha256"] != actionHash {
				t.Errorf("ticket draft payload mismatch: %+v", in)
			}
			_, _ = w.Write([]byte(created))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/change-tickets/ticket-1/evidence":
			var in TicketEvidence
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Errorf("decode evidence: %v", err)
			}
			if in.Kind != "snapshot" || in.Path != "/shippin/deployments/deploy-1/generations/4/review.json" || strings.Contains(in.Text, "s3cr3t") {
				t.Errorf("evidence payload invalid or unredacted: %+v", in)
			}
			_, _ = w.Write([]byte(withEvidence))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/change-tickets/ticket-1/submit":
			if r.ContentLength != 0 {
				t.Errorf("submit unexpectedly included a request body")
			}
			_, _ = w.Write([]byte(submitted))
		default:
			t.Errorf("unexpected ticket request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c, err := newClient(Config{BaseURL: server.URL, Token: "service-token"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	draft := TicketDraft{TenantID: "tenant-1", ID: "ticket-1", Title: "Review deployment generation 4", Machine: "machine-1", Command: command,
		Fields: map[string]string{"shippin_action_sha256": actionHash}, Lists: map[string][]string{"rollback": {"restore previous generation"}}}
	createdResult, err := c.CreateTicket(context.Background(), "create-idempotency-0001", draft)
	if err != nil || createdResult.Status != "draft" {
		t.Fatalf("create result=%+v err=%v", createdResult, err)
	}
	evidenceResult, err := c.AddTicketEvidence(context.Background(), "evidence-idempotency-0001", "ticket-1", TicketEvidence{
		Kind: "snapshot", Path: "/shippin/deployments/deploy-1/generations/4/review.json", Text: `{"config":{"credential":"vaultix://platform/deploy/db#password"}}`,
	})
	if err != nil || evidenceResult.SHA256 != policyHash {
		t.Fatalf("evidence result=%+v err=%v", evidenceResult, err)
	}
	submittedResult, err := c.SubmitTicket(context.Background(), "submit-idempotency-0001", "ticket-1")
	if err != nil || submittedResult.Status != "pending_review" || calls != 3 {
		t.Fatalf("submit result=%+v calls=%d err=%v", submittedResult, calls, err)
	}
}

func TestTicketLifecycleRejectsUnsafeInputBeforeNetwork(t *testing.T) {
	c, err := NewClient(Config{BaseURL: "https://agenthub.example", Token: "service-token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddTicketEvidence(context.Background(), "evidence-idempotency-0001", "ticket-1", TicketEvidence{Kind: "output", Path: "/tmp/../secret", Text: "not sent"}); err == nil {
		t.Fatal("path traversal evidence was accepted")
	}
	if _, err := c.AddTicketEvidence(context.Background(), "evidence-idempotency-0001", "ticket-1", TicketEvidence{Kind: "shell", Path: "/tmp/evidence", Text: "not sent"}); err == nil {
		t.Fatal("unsupported evidence kind was accepted")
	}
	draft := TicketDraft{TenantID: "tenant-1", ID: "ticket-1", Title: "Review deployment", Machine: "machine-1", Command: `{}`, Fields: map[string]string{}}
	if _, err := c.CreateTicket(context.Background(), "short", draft); err == nil {
		t.Fatal("short idempotency key was accepted")
	}
}

func mustJSONString(t *testing.T, value string) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const policyJSON = `{"tenant_id":"tenant-1","version":2,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","doc":{"version":2,"rules":[{"id":"review-provider-actions","enabled":true,"effect":"require","require":[{"check":"approval"},{"check":"review","min":2,"role":"reviewer"}]}]}}`

const ticketJSON = `{"ticket":{"id":"ticket-1","tenant_id":"tenant-1","machine":"machine-1","command":"{\"resource\":\"guest-9\",\"generation\":4}","fields":{"shippin_action_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","status":"passed"},"evidence":[],"reviews":[]}`

const decisionJSON = `{"decision_id":"decision-1","tenant_id":"tenant-1","policy_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","allow":true,"decision":{"allow":true,"policy_version":2,"matched":["review-provider-actions"]}}`
