// Package hub is the agent checkout board: tasks that agents claim under a lease, work, and
// optionally send for the owner's sign-off.
package hub

import (
	"errors"
	"time"
)

// Status values, in board order.
const (
	Open          = "open"
	Claimed       = "claimed"
	InProgress    = "in_progress"
	NeedsApproval = "needs_approval"
	Approved      = "approved"
	Done          = "done"
	Rejected      = "rejected"
)

// Statuses is the board's column order.
var Statuses = []string{Open, Claimed, InProgress, NeedsApproval, Approved, Done, Rejected}

// Task is one card on the board.
type Task struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Status       string     `json:"status"`
	Progress     string     `json:"progress"`
	LeaseOwner   string     `json:"lease_owner,omitempty"`
	LeaseUntil   *time.Time `json:"lease_until,omitempty"`
	ApprovalNote string     `json:"approval_note"`
	DecidedBy    string     `json:"decided_by,omitempty"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

var (
	// ErrNotFound: no such task in this org.
	ErrNotFound = errors.New("task not found")
	// ErrConflict: the task is not in a state (or lease) that allows the transition.
	ErrConflict = errors.New("task state does not allow this")
)
