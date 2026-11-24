// Package agenthub contains Shippin's authenticated client for AgentHub-owned
// policy and human review. It does not make provider calls.
package agenthub

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 5 * time.Second
const maxResponseBytes = 1 << 20
const maxCommandBytes = 32 << 10

var ErrDenied = errors.New("AgentHub did not authorize this provider action")

// Config is installation-specific. Token is supplied by the Shippin secret
// resolver; callers must not persist it with this configuration.
type Config struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// Request identifies the exact already-reviewed action. Command is a stable,
// canonical, secret-free representation of the operation. ActionSHA256 binds
// the ticket to Shippin's immutable action snapshot.
type Request struct {
	TenantID     string
	MachineID    string
	TicketID     string
	Tool         string
	Command      string
	ActionSHA256 string
}

// TicketDraft is the exact change snapshot Shippin asks AgentHub to review.
// Values must be secret-free; secrets are referenced through the installation's
// configured secret owner and never copied into ticket fields or evidence.
type TicketDraft struct {
	TenantID string              `json:"-"`
	ID       string              `json:"id"`
	Title    string              `json:"title"`
	Machine  string              `json:"machine"`
	Command  string              `json:"command"`
	Fields   map[string]string   `json:"fields"`
	Lists    map[string][]string `json:"lists"`
}

type TicketEvidence struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	Text string `json:"text"`
}

type TicketSummary struct {
	ID       string            `json:"id"`
	TenantID string            `json:"tenant_id"`
	Machine  *string           `json:"machine,omitempty"`
	Command  string            `json:"command"`
	Fields   map[string]string `json:"fields"`
	SHA256   string            `json:"sha256"`
	Status   string            `json:"status"`
	Version  int64             `json:"version"`
}

// Provenance is the evidence Shippin must persist with an authorized action.
type Provenance struct {
	DecisionID    string   `json:"decision_id"`
	TenantID      string   `json:"tenant_id"`
	MachineID     string   `json:"machine_id"`
	TicketID      string   `json:"ticket_id"`
	TicketSHA256  string   `json:"ticket_sha256"`
	PolicySHA256  string   `json:"policy_sha256"`
	PolicyVersion int64    `json:"policy_version"`
	MatchedRules  []string `json:"matched_rules"`
}

// PolicyStatus is safe metadata returned by the explicit read-only connection check.
type PolicyStatus struct {
	TenantID string `json:"tenant_id"`
	Version  int64  `json:"policy_version"`
	SHA256   string `json:"policy_sha256"`
}

// SystemMapping is a read-only verification that a registered AgentHub machine
// belongs to the tenant saved on the Shippin system profile.
type SystemMapping struct {
	TenantID      string `json:"tenant_id"`
	MachineID     string `json:"machine_id"`
	PolicyVersion int64  `json:"policy_version"`
	PolicySHA256  string `json:"policy_sha256"`
}

// Client consumes the reviewed AgentHub REST contract. A client is safe to
// construct from operator configuration; redirect following is disabled so
// bearer credentials cannot be forwarded to another origin.
type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	return newClient(cfg, nil)
}

func newClient(cfg Config, transport *http.Client) (*Client, error) {
	base, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || base == nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("AgentHub URL must be an HTTPS origin or path without credentials, query, or fragment")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("AgentHub service credential is not configured")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	httpClient := &http.Client{
		Timeout:       cfg.Timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	if transport != nil {
		httpClient.Transport = transport.Transport
	}
	return &Client{base: base, token: cfg.Token, http: &http.Client{
		Timeout: httpClient.Timeout, CheckRedirect: httpClient.CheckRedirect, Transport: httpClient.Transport,
	}}, nil
}

type policyDocument struct {
	TenantID string          `json:"tenant_id"`
	Version  int64           `json:"version"`
	SHA256   string          `json:"sha256"`
	Doc      json.RawMessage `json:"doc"`
}

type policyBody struct {
	Rules []policyRule `json:"rules"`
}

type policyRule struct {
	ID      string        `json:"id"`
	Enabled bool          `json:"enabled"`
	Effect  string        `json:"effect"`
	Require []policyCheck `json:"require"`
}

type policyCheck struct {
	Kind string `json:"check"`
	Min  *int   `json:"min"`
	Role string `json:"role"`
}

type ticketResponse struct {
	Ticket TicketSummary `json:"ticket"`
}

type decisionRequest struct {
	Tool     string `json:"tool"`
	Command  string `json:"command"`
	Machine  string `json:"machine"`
	TicketID string `json:"ticket_id"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

type machineResponse struct {
	ID string `json:"id"`
}

type decisionResponse struct {
	DecisionID string `json:"decision_id"`
	TenantID   string `json:"tenant_id"`
	PolicySHA  string `json:"policy_sha256"`
	Allow      bool   `json:"allow"`
	Decision   struct {
		Allow   bool     `json:"allow"`
		Version int64    `json:"policy_version"`
		Matched []string `json:"matched"`
	} `json:"decision"`
}

// Authorize allows only a current AgentHub decision that can be traced to the
// exact Shippin action, stable system, reviewed ticket and policy version.
// Every matched policy rule must explicitly require both a human approval and
// a reviewer quorum. This rejects AgentHub's default allow-on-no-match behavior,
// permissive/audit matches, and policies that only check for one human PASS.
func (c *Client) Authorize(ctx context.Context, in Request) (Provenance, error) {
	if c == nil || c.base == nil || c.http == nil {
		return Provenance{}, errors.New("AgentHub client is not configured")
	}
	if in.TenantID == "" || in.MachineID == "" || in.TicketID == "" || in.Tool == "" || in.Command == "" || in.ActionSHA256 == "" {
		return Provenance{}, errors.New("AgentHub authorization requires tenant, machine, ticket, tool, command, and action hash")
	}
	if !validID(in.TenantID) || !validID(in.MachineID) || !validID(in.TicketID) {
		return Provenance{}, errors.New("AgentHub tenant, machine, and ticket IDs must use letters, digits, dot, underscore, or hyphen")
	}
	if len(in.Command) > maxCommandBytes || !json.Valid([]byte(in.Command)) {
		return Provenance{}, errors.New("AgentHub action command must be valid JSON within the size limit")
	}
	if len(in.ActionSHA256) != 64 {
		return Provenance{}, errors.New("AgentHub action hash must be a SHA-256 digest")
	}
	if _, err := hex.DecodeString(in.ActionSHA256); err != nil || strings.ToLower(in.ActionSHA256) != in.ActionSHA256 {
		return Provenance{}, errors.New("AgentHub action hash must be lowercase hexadecimal")
	}

	beforePolicy, err := c.policy(ctx, in.TenantID)
	if err != nil {
		return Provenance{}, err
	}
	beforeTicket, err := c.ticket(ctx, in.TicketID)
	if err != nil {
		return Provenance{}, err
	}
	if err := bindTicket(beforeTicket, in); err != nil {
		return Provenance{}, err
	}

	var answer decisionResponse
	err = c.doJSON(ctx, http.MethodPost, "/api/v1/policy/decide", decisionRequest{
		Tool: in.Tool, Command: in.Command, Machine: in.MachineID, TicketID: in.TicketID,
	}, &answer, nil)
	if err != nil {
		return Provenance{}, err
	}
	if !answer.Allow || !answer.Decision.Allow || answer.DecisionID == "" || answer.TenantID != in.TenantID ||
		answer.PolicySHA != beforePolicy.SHA256 || answer.Decision.Version != beforePolicy.Version {
		return Provenance{}, ErrDenied
	}
	if !requiresHumanReview(beforePolicy.Doc, answer.Decision.Matched) {
		return Provenance{}, ErrDenied
	}

	afterPolicy, err := c.policy(ctx, in.TenantID)
	if err != nil {
		return Provenance{}, err
	}
	afterTicket, err := c.ticket(ctx, in.TicketID)
	if err != nil {
		return Provenance{}, err
	}
	if afterPolicy.SHA256 != beforePolicy.SHA256 || afterPolicy.Version != beforePolicy.Version ||
		afterTicket.Ticket.SHA256 != beforeTicket.Ticket.SHA256 {
		return Provenance{}, ErrDenied
	}
	if err := bindTicket(afterTicket, in); err != nil {
		return Provenance{}, err
	}
	if afterTicket.Ticket.Status != "passed" {
		return Provenance{}, ErrDenied
	}
	return Provenance{DecisionID: answer.DecisionID, TenantID: in.TenantID, MachineID: in.MachineID,
		TicketID: in.TicketID, TicketSHA256: afterTicket.Ticket.SHA256, PolicySHA256: beforePolicy.SHA256,
		PolicyVersion: beforePolicy.Version, MatchedRules: append([]string(nil), answer.Decision.Matched...)}, nil
}

// CheckPolicy verifies that the configured credential can read the current policy
// for an operator-supplied tenant. It performs no ticket or provider mutation.
func (c *Client) CheckPolicy(ctx context.Context, tenant string) (PolicyStatus, error) {
	if c == nil || c.base == nil || c.http == nil {
		return PolicyStatus{}, errors.New("AgentHub client is not configured")
	}
	if !validID(tenant) {
		return PolicyStatus{}, errors.New("AgentHub tenant ID must use letters, digits, dot, underscore, or hyphen")
	}
	doc, err := c.policy(ctx, tenant)
	if err != nil {
		return PolicyStatus{}, err
	}
	return PolicyStatus{TenantID: doc.TenantID, Version: doc.Version, SHA256: doc.SHA256}, nil
}

// CheckSystemMapping confirms machine existence and asks AgentHub for a dry-run
// decision to resolve the machine's authoritative tenant. The dry run is not
// authorization and is not recorded by AgentHub.
func (c *Client) CheckSystemMapping(ctx context.Context, tenant, machine, systemID string) (SystemMapping, error) {
	if c == nil || c.base == nil || c.http == nil {
		return SystemMapping{}, errors.New("AgentHub client is not configured")
	}
	if !validID(tenant) || !validID(machine) || !validID(systemID) {
		return SystemMapping{}, errors.New("AgentHub tenant, machine and Shippin system IDs must use letters, digits, dot, underscore, or hyphen")
	}
	before, err := c.policy(ctx, tenant)
	if err != nil {
		return SystemMapping{}, err
	}
	var remote machineResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/machines/"+url.PathEscape(machine), nil, &remote, nil); err != nil {
		return SystemMapping{}, err
	}
	if remote.ID != machine {
		return SystemMapping{}, ErrDenied
	}
	command, _ := json.Marshal(struct {
		Operation string `json:"operation"`
		SystemID  string `json:"system_id"`
	}{"mapping_validation", systemID})
	var answer decisionResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/policy/decide", decisionRequest{
		Tool: "shippin:identity.mapping.validate", Command: string(command), Machine: machine, DryRun: true,
	}, &answer, nil); err != nil {
		return SystemMapping{}, err
	}
	after, err := c.policy(ctx, tenant)
	if err != nil {
		return SystemMapping{}, err
	}
	if answer.TenantID != tenant || answer.PolicySHA != before.SHA256 || answer.Decision.Version != before.Version ||
		after.SHA256 != before.SHA256 || after.Version != before.Version {
		return SystemMapping{}, ErrDenied
	}
	return SystemMapping{TenantID: tenant, MachineID: machine, PolicyVersion: before.Version, PolicySHA256: before.SHA256}, nil
}

func bindTicket(t ticketResponse, in Request) error {
	if t.Ticket.ID != in.TicketID || t.Ticket.TenantID != in.TenantID || t.Ticket.Machine == nil ||
		*t.Ticket.Machine != in.MachineID || t.Ticket.Command != in.Command ||
		t.Ticket.Fields["shippin_action_sha256"] != in.ActionSHA256 || !validSHA256(t.Ticket.SHA256) {
		return ErrDenied
	}
	return nil
}

func requiresHumanReview(raw json.RawMessage, matched []string) bool {
	if len(matched) == 0 {
		return false
	}
	var doc policyBody
	if json.Unmarshal(raw, &doc) != nil {
		return false
	}
	ids := make(map[string]struct{}, len(matched))
	for _, id := range matched {
		ids[id] = struct{}{}
	}
	rules := make(map[string]policyRule, len(doc.Rules))
	for _, rule := range doc.Rules {
		rules[rule.ID] = rule
	}
	for id := range ids {
		rule, ok := rules[id]
		if !ok || !rule.Enabled || rule.Effect != "require" {
			return false
		}
		approval, quorum := false, false
		for _, check := range rule.Require {
			if check.Kind == "approval" {
				approval = true
			}
			if check.Kind == "review" && check.Min != nil && *check.Min > 0 && strings.TrimSpace(check.Role) != "" {
				quorum = true
			}
		}
		if !approval || !quorum {
			return false
		}
	}
	return true
}

func (c *Client) policy(ctx context.Context, tenant string) (policyDocument, error) {
	var out policyDocument
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/policy/doc", nil, &out, url.Values{"tenant": []string{tenant}}); err != nil {
		return out, err
	}
	if out.TenantID != tenant || out.Version < 1 || !validSHA256(out.SHA256) || len(out.Doc) == 0 {
		return out, errors.New("AgentHub returned an incomplete policy document")
	}
	return out, nil
}

func (c *Client) ticket(ctx context.Context, id string) (ticketResponse, error) {
	var out ticketResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/change-tickets/"+url.PathEscape(id), nil, &out, nil); err != nil {
		return out, err
	}
	return out, nil
}

// GetTicket returns only safe ticket metadata; attached evidence and review
// contents stay in AgentHub and are not duplicated into Shippin responses.
func (c *Client) GetTicket(ctx context.Context, id string) (TicketSummary, error) {
	if !validID(id) {
		return TicketSummary{}, errors.New("AgentHub ticket ID is invalid")
	}
	result, err := c.ticket(ctx, id)
	if err != nil {
		return TicketSummary{}, err
	}
	if result.Ticket.ID != id || !validSHA256(result.Ticket.SHA256) || result.Ticket.Status == "" {
		return TicketSummary{}, errors.New("AgentHub returned incomplete ticket metadata")
	}
	return result.Ticket, nil
}

// CreateTicket creates an AgentHub-owned draft using a caller-supplied stable
// idempotency key. It does not authorize or perform the described operation.
func (c *Client) CreateTicket(ctx context.Context, idempotencyKey string, in TicketDraft) (TicketSummary, error) {
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return TicketSummary{}, err
	}
	if !validID(in.TenantID) || !validID(in.ID) || len(strings.TrimSpace(in.Title)) < 3 || len(in.Title) > 200 ||
		!validID(in.Machine) || len(in.Command) == 0 || len(in.Command) > maxCommandBytes || !json.Valid([]byte(in.Command)) {
		return TicketSummary{}, errors.New("AgentHub ticket draft is invalid")
	}
	if !validSHA256(in.Fields["shippin_action_sha256"]) {
		return TicketSummary{}, errors.New("AgentHub ticket draft requires a Shippin action digest")
	}
	var out TicketSummary
	if err := c.doJSONWithIdempotency(ctx, http.MethodPost, "/api/v1/change-tickets", in, &out, nil, idempotencyKey); err != nil {
		return TicketSummary{}, err
	}
	if out.ID != in.ID || out.TenantID != in.TenantID || out.Machine == nil || *out.Machine != in.Machine || out.Command != in.Command ||
		out.Fields["shippin_action_sha256"] != in.Fields["shippin_action_sha256"] || !validSHA256(out.SHA256) {
		return TicketSummary{}, ErrDenied
	}
	return out, nil
}

// AddTicketEvidence attaches one bounded, caller-redacted evidence artifact.
// The caller owns secret redaction before supplying Text.
func (c *Client) AddTicketEvidence(ctx context.Context, idempotencyKey, ticketID string, in TicketEvidence) (TicketSummary, error) {
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return TicketSummary{}, err
	}
	if !validID(ticketID) || !validEvidenceKind(in.Kind) ||
		!strings.HasPrefix(in.Path, "/") || strings.Contains(in.Path, "..") || len(in.Path) > 1024 ||
		len(in.Text) == 0 || len(in.Text) > 20_000 {
		return TicketSummary{}, errors.New("AgentHub ticket evidence is invalid or exceeds its size limit")
	}
	var out TicketSummary
	path := "/api/v1/change-tickets/" + url.PathEscape(ticketID) + "/evidence"
	if err := c.doJSONWithIdempotency(ctx, http.MethodPost, path, in, &out, nil, idempotencyKey); err != nil {
		return TicketSummary{}, err
	}
	if out.ID != ticketID || !validSHA256(out.SHA256) {
		return TicketSummary{}, ErrDenied
	}
	return out, nil
}

// SubmitTicket moves the caller-owned AgentHub draft into its review queue.
func (c *Client) SubmitTicket(ctx context.Context, idempotencyKey, ticketID string) (TicketSummary, error) {
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return TicketSummary{}, err
	}
	if !validID(ticketID) {
		return TicketSummary{}, errors.New("AgentHub ticket ID is invalid")
	}
	var out TicketSummary
	path := "/api/v1/change-tickets/" + url.PathEscape(ticketID) + "/submit"
	if err := c.doJSONWithIdempotency(ctx, http.MethodPost, path, nil, &out, nil, idempotencyKey); err != nil {
		return TicketSummary{}, err
	}
	if out.ID != ticketID || out.Status != "pending_review" || !validSHA256(out.SHA256) {
		return TicketSummary{}, ErrDenied
	}
	return out, nil
}

func validEvidenceKind(kind string) bool {
	switch kind {
	case "snapshot", "topology", "research", "output", "doc":
		return true
	default:
		return false
	}
}

func validateIdempotencyKey(key string) error {
	if len(key) < 16 || len(key) > 128 || strings.TrimSpace(key) != key || strings.ContainsAny(key, "\r\n\x00") {
		return errors.New("AgentHub write requires a bounded idempotency key")
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, out any, query url.Values) error {
	return c.doJSONWithIdempotency(ctx, method, path, body, out, query, "")
}

func (c *Client) doJSONWithIdempotency(ctx context.Context, method, path string, body, out any, query url.Values, idempotencyKey string) error {
	endpoint := *c.base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = query.Encode()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode AgentHub request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return fmt.Errorf("build AgentHub request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("AgentHub request failed: %w", redactTransportError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("AgentHub returned HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return errors.New("AgentHub response exceeds the configured size limit")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(out); err != nil {
		return errors.New("AgentHub returned an invalid JSON response")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("AgentHub returned an oversized or trailing JSON response")
	}
	return nil
}

// Avoid returning server error bodies, which may contain sensitive ticket data.
func redactTransportError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return errors.New("transport or TLS error")
	}
	return err
}

func validID(s string) bool {
	if len(s) < 3 || len(s) > 64 || !isASCIIAlphaNum(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !isASCIIAlphaNum(c) && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlphaNum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func validSHA256(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
