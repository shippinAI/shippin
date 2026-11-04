package hub

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/auth"
)

// Principal is the signed-in caller: a person or an agent service user.
type Principal struct {
	Sub   string
	Name  string
	OrgID string
	// Email counts for ownership only when the IdP marked it verified.
	Email         string
	EmailVerified bool
	// Roles are Nomen roles already mapped by the console (founder, operator, ...).
	Roles []string
}

// Handler serves /v1/hub. Who resolves the caller (Nomen in the console); approval decisions
// require an explicit configured policy. Lease is the checkout lease length; Live feeds the
// optional GET /v1/hub/stream route.
type Handler struct {
	Store  *Store
	Who    func(w http.ResponseWriter, r *http.Request) (*Principal, bool)
	Policy *approval.Policy
	Lease  time.Duration
	Live   *Live
}

// Router is the subset of http.ServeMux the handler needs.
type Router interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// Register mounts the hub routes.
func (h *Handler) Register(mux Router) {
	mux.HandleFunc("GET /v1/hub/me", h.with(h.me))
	mux.HandleFunc("GET /v1/hub/tasks", h.with(h.list))
	mux.HandleFunc("GET /v1/hub/stream", h.with(h.stream))
	mux.HandleFunc("GET /v1/hub/tasks/{id}/events", h.with(h.events))
	mux.HandleFunc("POST /v1/hub/tasks", h.with(h.create))
	mux.HandleFunc("POST /v1/hub/tasks/{id}/claim", h.with(h.claim))
	mux.HandleFunc("POST /v1/hub/tasks/{id}/progress", h.with(h.progress))
	mux.HandleFunc("POST /v1/hub/tasks/{id}/request-approval", h.with(h.request))
	mux.HandleFunc("POST /v1/hub/tasks/{id}/done", h.with(h.done))
	mux.HandleFunc("POST /v1/hub/tasks/{id}/decision", h.with(h.decide))
	mux.HandleFunc("POST /v1/hub/approvals", h.with(h.postApproval))
	mux.HandleFunc("GET /v1/hub/approvals/{key}", h.with(h.approvalStatus))
}

type call struct {
	w  http.ResponseWriter
	r  *http.Request
	p  *Principal
	in struct {
		Title, Body, Note string
		Key, Action, Host string
		Approve           bool
	}
}

func (h *Handler) with(fn func(*call)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := h.Who(w, r)
		if !ok {
			return
		}
		c := &call{w: w, r: r, p: p}
		if r.Method == http.MethodPost && r.ContentLength != 0 {
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&c.in) != nil {
				auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
				return
			}
		}
		fn(c)
	}
}

// IsOwner reports whether p may decide under an explicitly configured policy.
func (h *Handler) IsOwner(p *Principal) bool {
	if h.Policy == nil || p == nil || !h.Policy.Enabled || !h.Policy.HasAction("hub.decide") {
		return false
	}
	return h.Policy.Allows(approval.Subject{Sub: p.Sub, Roles: p.Roles, Email: p.Email, EmailVerified: p.EmailVerified}, "hub.decide", "")
}

func (h *Handler) postApproval(c *call) {
	key := strings.TrimSpace(c.in.Key)
	title := strings.TrimSpace(c.in.Title)
	if key == "" || len(key) > 200 {
		auth.Problem(c.w, http.StatusBadRequest, "An approval key is required.")
		return
	}
	if title == "" {
		title = key
	}
	t, err := h.Store.UpsertApproval(c.r.Context(), c.p.OrgID, key, title, c.in.Body, c.p.Sub)
	if err != nil {
		h.out(c, nil, err)
		return
	}
	reply(c.w, http.StatusOK, approval.Decision{Key: key, Status: t.Status, By: t.DecidedBy, Note: t.ApprovalNote})
}

func (h *Handler) approvalStatus(c *call) {
	key := c.r.PathValue("key")
	t, err := h.Store.ApprovalByKey(c.r.Context(), c.p.OrgID, key)
	if err != nil {
		h.out(c, nil, err)
		return
	}
	reply(c.w, http.StatusOK, approval.Decision{Key: key, Status: t.Status, By: t.DecidedBy, Note: t.ApprovalNote})
}

func (h *Handler) me(c *call) {
	reply(c.w, http.StatusOK, map[string]any{"sub": c.p.Sub, "name": c.p.Name, "is_owner": h.IsOwner(c.p), "statuses": Statuses})
}

func (h *Handler) list(c *call) {
	ts, err := h.Store.List(c.r.Context(), c.p.OrgID)
	h.out(c, map[string]any{"items": ts}, err)
}

func (h *Handler) create(c *call) {
	t := strings.TrimSpace(c.in.Title)
	if t == "" || len(t) > 200 || len(c.in.Body) > 8000 {
		auth.Problem(c.w, http.StatusBadRequest, "A title (1-200 characters) is required.")
		return
	}
	task, err := h.Store.Create(c.r.Context(), c.p.OrgID, t, c.in.Body, c.p.Sub)
	h.outStatus(c, http.StatusCreated, task, err)
}

func (h *Handler) claim(c *call) {
	t, err := h.Store.Claim(c.r.Context(), c.p.OrgID, c.r.PathValue("id"), c.p.Sub, h.Lease)
	h.out(c, t, err)
}

func (h *Handler) progress(c *call) {
	t, err := h.Store.Progress(c.r.Context(), c.p.OrgID, c.r.PathValue("id"), c.p.Sub, c.in.Note, h.Lease)
	h.out(c, t, err)
}

func (h *Handler) request(c *call) {
	t, err := h.Store.RequestApproval(c.r.Context(), c.p.OrgID, c.r.PathValue("id"), c.p.Sub, c.in.Note)
	h.out(c, t, err)
}

func (h *Handler) done(c *call) {
	t, err := h.Store.Finish(c.r.Context(), c.p.OrgID, c.r.PathValue("id"), c.p.Sub)
	h.out(c, t, err)
}

func (h *Handler) decide(c *call) {
	if !h.IsOwner(c.p) {
		auth.Problem(c.w, http.StatusForbidden, "Only the board owner can approve or reject.")
		return
	}
	t, err := h.Store.Decide(c.r.Context(), c.p.OrgID, c.r.PathValue("id"), c.p.Sub, c.in.Approve, c.in.Note)
	h.out(c, t, err)
}
