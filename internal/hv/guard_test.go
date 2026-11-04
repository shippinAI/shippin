package hv

import (
	"context"
	"errors"
	"testing"

	"shippin.cloud/console/internal/approval"
)

type memBoard struct{ status string }

func (m memBoard) Status(context.Context, string, string) (string, error) { return m.status, nil }

func TestGuardRequiresRoleAndBoard(t *testing.T) {
	pol := approval.Policy{Enabled: true, Board: approval.BoardConfig{Driver: "hub"}, Rules: []approval.Rule{
		{ID: "test-lifecycle", Actions: []string{"hv.lifecycle"}, ApproverRoles: []string{"operator"}, NeedsApproval: true},
	}}
	g := Guard{Policy: pol, Subject: approval.Subject{Sub: "other"}, Host: "pve-a", Org: "ops"}
	if err := g.Allow(context.Background(), "start", "100"); !errors.Is(err, ErrDenied) {
		t.Fatalf("stranger: %v", err)
	}
	g.Subject = approval.Subject{Roles: []string{"operator"}}
	if err := g.Allow(context.Background(), "start", "100"); !errors.Is(err, ErrWaiting) {
		t.Fatalf("founder without board: %v", err)
	}
	g.Board = memBoard{status: "needs_approval"}
	if err := g.Allow(context.Background(), "start", "100"); !errors.Is(err, ErrWaiting) {
		t.Fatalf("pending card: %v", err)
	}
	g.Board = memBoard{status: "approved"}
	if err := g.Allow(context.Background(), "start", "100"); err != nil {
		t.Fatal(err)
	}
}

func TestGuardSkipsBoardWhenOff(t *testing.T) {
	pol := approval.Policy{Enabled: true, Board: approval.BoardConfig{Driver: "off"}, Rules: []approval.Rule{
		{ID: "test-lifecycle", Actions: []string{"hv.lifecycle"}, ApproverRoles: []string{"operator"}},
	}}
	g := Guard{Policy: pol, Subject: approval.Subject{Roles: []string{"operator"}}, Host: "pve-a"}
	if err := g.Allow(context.Background(), "stop", "100"); err != nil {
		t.Fatal(err)
	}
}
