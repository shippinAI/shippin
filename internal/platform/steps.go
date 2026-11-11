package platform

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
)

// The built-in services an owner acknowledges during setup. What each one means is the panel's wording.
var knownSteps = []string{"postgres", "clickhouse", "dragonfly"}

// Step is one acknowledgement an owner has given.
type Step struct {
	Step           string    `json:"step"`
	AcknowledgedBy string    `json:"acknowledged_by"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
}

func (h *Handler) listSteps(w http.ResponseWriter, r *http.Request, _ string) {
	rows, err := h.Pool.Query(r.Context(), `SELECT step, acknowledged_by, acknowledged_at FROM shippin_setup_steps ORDER BY acknowledged_at`)
	if err != nil {
		slog.Error("platform: setup steps not readable", "error", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The setup steps could not be read.")
		return
	}
	defer rows.Close()
	out := []Step{}
	for rows.Next() {
		var s Step
		if err := rows.Scan(&s.Step, &s.AcknowledgedBy, &s.AcknowledgedAt); err != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "The setup steps could not be read.")
			return
		}
		out = append(out, s)
	}
	writeJSON(w, map[string]any{"items": out})
}

func (h *Handler) acknowledgeStep(w http.ResponseWriter, r *http.Request, actor string) {
	step := r.PathValue("step")
	if !slices.Contains(knownSteps, step) {
		auth.Problem(w, http.StatusNotFound, "There is no setup step with that name.")
		return
	}
	var in confirmation
	if !decode(w, r, &in) || !in.check(w) {
		return
	}
	if !h.Audit(w, r, "platform.setup_step", step+" · "+strings.TrimSpace(in.Reason)) {
		return
	}
	if _, err := h.Pool.Exec(r.Context(), `INSERT INTO shippin_setup_steps(step, acknowledged_by) VALUES($1,$2)
	  ON CONFLICT (step) DO UPDATE SET acknowledged_by=$2, acknowledged_at=now()`, step, actor); err != nil {
		slog.Error("platform: setup step not saved", "error", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The step could not be saved.")
		return
	}
	writeJSON(w, map[string]string{"step": step})
}
