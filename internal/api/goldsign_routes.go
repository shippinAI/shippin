package api

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/goldsign"
	"shippin.cloud/console/internal/goldverify"
	"shippin.cloud/console/internal/hub"
)

// RegisterGoldSign mounts passkey approvals for Gold (approve-p1). Off unless goldSignStore finds
// GOLDSIGN_DIR, GOLDSIGN_FACTS_DIR, GOLDSIGN_OWNER_SUB, GOLDSIGN_OWNER_NAME and a public host.
func (a *API) RegisterGoldSign(mux Router) {
	s := goldSignStore(os.Getenv)
	if s == nil {
		return
	}
	nets, err := parseNets(os.Getenv("GOLDSIGN_SYNC_CIDRS"))
	if err != nil {
		log.Printf("gold-sign: OFF: %v (fix or unset it; nothing is mounted)", err)
		return
	}
	if len(nets) == 0 {
		log.Printf("gold-sign: GOLDSIGN_SYNC_CIDRS unset: /v1/gold-sign/outbox and /sync refuse every request (fail-closed)")
	}
	h := &goldSignHTTP{a: a, s: s, syncToken: os.Getenv("GOLDSIGN_SYNC_TOKEN"), syncNets: nets, board: a.approvalBoard()}
	mux.HandleFunc("GET /v1/gold-sign/bypass", a.flow.Require(h.bypass))
	mux.HandleFunc("GET /v1/gold-sign/tickets", a.flow.Require(h.owner("gold.ticket", h.tickets)))
	mux.HandleFunc("GET /v1/gold-sign/tickets/{id}", a.flow.Require(h.ticket))
	mux.HandleFunc("POST /v1/gold-sign/envelopes", a.flow.Require(h.relay))
	mux.HandleFunc("GET /v1/gold-sign/keys", a.flow.Require(h.owner("gold.passkey", h.keys)))
	mux.HandleFunc("POST /v1/gold-sign/keys/begin", a.flow.Require(h.owner("gold.passkey", h.fresh(h.begin))))
	mux.HandleFunc("POST /v1/gold-sign/keys", a.flow.Require(h.owner("gold.passkey", h.fresh(h.finish))))
	mux.HandleFunc("GET /v1/gold-sign/outbox", h.gold(h.outbox)) // Gold pulls envelopes
	mux.HandleFunc("PUT /v1/gold-sign/sync", h.gold(h.sync))     // Gold pushes facts, certs, revoked
}

type goldSignHTTP struct {
	a         *API
	s         *goldsign.Store
	syncToken string
	syncNets  []*net.IPNet
	board     approval.Board
}

// owner admits callers the approval policy allows for action. With no policy loaded, the
// configured GOLDSIGN_OWNER_SUB remains the gate (store.IsOwner).
func (h *goldSignHTTP) owner(action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := h.allow(w, r, action, ""); !ok {
			return
		}
		next(w, r)
	}
}

func (h *goldSignHTTP) allow(w http.ResponseWriter, r *http.Request, action, host string) (*caller, bool) {
	c, ok := h.a.caller(w, r)
	if !ok {
		return nil, false
	}
	if h.a.cfg.Approval.Enabled {
		if h.a.cfg.Approval.Allows(approverSubject(h.a, c), action, host) {
			return c, true
		}
	} else if h.s.IsOwner(c.id.Sub) {
		return c, true
	}
	auth.Problem(w, http.StatusForbidden, "Only a configured Gold approver can do this.")
	return nil, false
}

// fresh requires a Nomen sign-in in the last 10 minutes before a passkey can be registered.
func (h *goldSignHTTP) fresh(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		at := h.a.authTime(r)
		if at.IsZero() || time.Since(at) > 10*time.Minute {
			auth.Problem(w, http.StatusPreconditionRequired, "Sign in to Nomen again before adding a passkey.")
			return
		}
		next(w, r)
	}
}

func (h *goldSignHTTP) bypass(w http.ResponseWriter, r *http.Request) {
	c, ok := h.a.caller(w, r)
	if !ok {
		return
	}
	writeJSON(w, map[string]any{"status": h.s.Bypass(), "can_revoke": h.s.IsOwner(c.id.Sub),
		"rp_id": h.s.RPID, "keys": credIDs(h.s.Keys())})
}

type ticketOut struct {
	goldsign.Facts
	BoardStatus string `json:"board_status,omitempty"`
}

func (h *goldSignHTTP) tickets(w http.ResponseWriter, r *http.Request) {
	c, ok := h.allow(w, r, "gold.ticket", "")
	if !ok {
		return
	}
	items, err := h.s.Pending()
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Ticket facts are unavailable.")
		return
	}
	views := make([]ticketOut, len(items))
	for i, it := range items {
		views[i] = ticketOut{Facts: it, BoardStatus: h.postTicket(r, c, it)}
	}
	last, fresh := h.s.LastSync()
	writeJSON(w, map[string]any{"items": views, "rp_id": h.s.RPID, "keys": credIDs(h.s.Keys()), "sync_at": last.At, "fresh": fresh})
}

func (h *goldSignHTTP) ticket(w http.ResponseWriter, r *http.Request) {
	t, err := h.s.Ticket(r.PathValue("id"))
	if err != nil {
		auth.Problem(w, http.StatusNotFound, "No such ticket.")
		return
	}
	host := ""
	if t != nil {
		host = t.Host
	}
	c, ok := h.allow(w, r, "gold.ticket", host)
	if !ok {
		return
	}
	writeJSON(w, ticketOut{Facts: *t, BoardStatus: h.postTicket(r, c, *t)})
}

func (h *goldSignHTTP) relay(w http.ResponseWriter, r *http.Request) {
	var e goldverify.Envelope
	if !decodeStrict(w, r, &e) {
		return
	}
	action, host := "gold.relay", ""
	if e.Payload.Decision == "revoke" {
		action = "gold.bypass"
	} else if t, err := h.s.Ticket(e.Payload.TicketID); err == nil {
		host = t.Host
	}
	c, ok := h.allow(w, r, action, host)
	if !ok {
		return
	}
	if h.board != nil && h.a.cfg.Approval.NeedsBoard(action, host) {
		d, err := h.board.Status(r.Context(), h.boardOrg(c), goldKey(e))
		if err != nil || d.Status != "approved" {
			auth.Problem(w, http.StatusConflict, "This change is waiting on the factory board.")
			return
		}
	}
	if err := h.s.Relay(&e); err != nil {
		code := http.StatusConflict
		if errors.Is(err, goldsign.ErrNotFound) {
			code = http.StatusNotFound
		}
		auth.Problem(w, code, err.Error())
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]string{"status": "sent to Gold for verification"})
}

func (h *goldSignHTTP) keys(w http.ResponseWriter, r *http.Request) { writeJSON(w, h.s.Keys()) }

func (h *goldSignHTTP) begin(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"challenge": h.s.BeginRegistration(), "rp_id": h.s.RPID,
		"user_id": h.s.OwnerSub, "user_name": h.s.OwnerName, "exclude": credIDs(h.s.Keys())})
}

func (h *goldSignHTTP) postTicket(r *http.Request, c *caller, it goldsign.Facts) string {
	if h.board == nil || !h.a.cfg.Approval.NeedsBoard("gold.ticket", it.Host) {
		return ""
	}
	d, err := h.board.Post(r.Context(), approval.Request{
		Key: "gold.ticket:" + it.ID + ":" + it.SHA256, OrgID: h.boardOrg(c), Action: "gold.ticket", Host: it.Host,
		Title: "Approve " + it.ID, Body: it.AgentSummary, By: c.id.Sub,
	})
	if err != nil {
		log.Printf("gold-sign: board post %s: %v", it.ID, err)
		return "unavailable"
	}
	return d.Status
}

func (h *goldSignHTTP) boardOrg(c *caller) string {
	if org := h.a.cfg.Approval.Board.OrgID; org != "" {
		return org
	}
	if c.id.OrgID != "" {
		return c.id.OrgID
	}
	return "gold"
}

func goldKey(e goldverify.Envelope) string {
	if e.Payload.Decision == "revoke" {
		return "gold.bypass:" + e.Payload.CID
	}
	return "gold.ticket:" + e.Payload.TicketID + ":" + e.Payload.TicketSHA256
}

func (a *API) approvalBoard() approval.Board {
	b := a.cfg.Approval.Board
	switch b.Driver {
	case "http":
		if b.BaseURL == "" {
			return nil
		}
		token := ""
		if b.TokenEnv != "" {
			token = os.Getenv(b.TokenEnv)
		}
		return approval.HTTPBoard{BaseURL: b.BaseURL, Token: token, OrgID: b.OrgID}
	case "hub":
		if a.db == nil {
			return nil
		}
		return hubBoard{store: &hub.Store{Pool: a.db.Pool}, org: b.OrgID}
	default:
		return nil
	}
}

type hubBoard struct {
	store *hub.Store
	org   string
}

func (b hubBoard) Post(ctx context.Context, req approval.Request) (approval.Decision, error) {
	if req.OrgID == "" {
		req.OrgID = b.org
	}
	t, err := b.store.UpsertApproval(ctx, req.OrgID, req.Key, req.Title, req.Body, req.By)
	if err != nil {
		return approval.Decision{}, err
	}
	return approval.Decision{Key: req.Key, Status: t.Status, By: t.DecidedBy, Note: t.ApprovalNote}, nil
}

func (b hubBoard) Status(ctx context.Context, org, key string) (approval.Decision, error) {
	if org == "" {
		org = b.org
	}
	t, err := b.store.ApprovalByKey(ctx, org, key)
	if err != nil {
		return approval.Decision{}, err
	}
	return approval.Decision{Key: key, Status: t.Status, By: t.DecidedBy, Note: t.ApprovalNote}, nil
}

func (h *goldSignHTTP) finish(w http.ResponseWriter, r *http.Request) {
	var reg goldsign.Registration
	if !decodeStrict(w, r, &reg) {
		return
	}
	k, err := h.s.FinishRegistration(reg)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeStatus(w, http.StatusCreated, k)
}
