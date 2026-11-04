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
}

type ticketResponse struct {
	Ticket struct {
		ID      string            `json:"id"`
		Tenant  string            `json:"tenant_id"`
		Machine *string           `json:"machine"`
		Command string            `json:"command"`
		Fields  map[string]string `json:"fields"`
		SHA256  string            `json:"sha256"`
		Status  string            `json:"status"`
	} `json:"ticket"`
}

type decisionRequest struct {
	Tool     string `json:"tool"`
	Command  string `json:"command"`
	Machine  string `json:"machine"`
	TicketID string `json:"ticket_id"`
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
// The policy must explicitly match at least one enabled require rule that
// contains an explicit human approval check. This rejects AgentHub's default
// allow-on-no-match behavior and plain allow/audit rules.
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

func bindTicket(t ticketResponse, in Request) error {
	if t.Ticket.ID != in.TicketID || t.Ticket.Tenant != in.TenantID || t.Ticket.Machine == nil ||
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
	for _, rule := range doc.Rules {
		if !rule.Enabled || rule.Effect != "require" {
			continue
		}
		if _, ok := ids[rule.ID]; !ok {
			continue
		}
		for _, check := range rule.Require {
			if check.Kind == "approval" {
				return true
			}
		}
	}
	return false
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

func (c *Client) doJSON(ctx context.Context, method, path string, body, out any, query url.Values) error {
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
