package api

import (
	"context"
	"time"
)

func (a *API) utilityFailed(ctx context.Context, homeID string) error {
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return a.subs.UtilityFailed(markCtx, homeID)
}

func (a *API) utilityStopped(ctx context.Context, homeID string) error {
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return a.subs.UtilityStopped(markCtx, homeID)
}
