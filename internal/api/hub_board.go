package api

import (
	"context"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/hub"
)

// hubBoard keeps approval cards in this panel's own hub.
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

func (b hubBoard) UseCard(ctx context.Context, org, key string) (bool, error) {
	if org == "" {
		org = b.org
	}
	return b.store.UseApproval(ctx, org, key)
}

func (b hubBoard) CardID(ctx context.Context, org, key string) (string, error) {
	if org == "" {
		org = b.org
	}
	t, err := b.store.ApprovalByKey(ctx, org, key)
	if err != nil {
		return "", err
	}
	return t.ID, nil
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
