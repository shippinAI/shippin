package api

import (
	"context"
	"log"
	"time"

	"shippin.cloud/console/internal/networking"
)

// RunNetworkInventory refreshes only integrations whose configured observation interval elapsed.
// The query is bounded and each provider request has a deadline, so one unreachable provider
// cannot hold up the console or an unbounded batch.
func (a *API) RunNetworkInventory(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.syncDueNetworkInventory(ctx)
		}
	}
}

func (a *API) syncDueNetworkInventory(ctx context.Context) {
	store := a.networkStore()
	if store == nil {
		return
	}
	items, err := store.Due(ctx, 20)
	if err != nil {
		log.Printf("network inventory due query: %v", err)
		return
	}
	for _, item := range items {
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := a.syncNetworkConnection(requestCtx, store, item)
		cancel()
		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		_ = store.ReleaseClaim(releaseCtx, item.ID)
		releaseCancel()
		if err != nil {
			log.Printf("network inventory refresh %s: %v", item.ID, safeProviderError(err))
		}
	}
}

func (a *API) syncNetworkConnection(ctx context.Context, store *networking.Store, item networking.Connection) error {
	ciphertext, err := store.Credential(ctx, item.ID)
	if err != nil {
		return err
	}
	var secret []byte
	if len(ciphertext) > 0 {
		sealer, err := a.sealer()
		if err != nil {
			return err
		}
		secret, err = sealer.Open(ciphertext, item.ID)
		if err != nil {
			return err
		}
	}
	resources, err := networking.Inventory(ctx, item, secret)
	if err != nil {
		_ = store.RecordCheck(context.WithoutCancel(ctx), item.ID, "degraded", safeProviderError(err), time.Now().UTC())
		return err
	}
	return store.ReplaceResources(ctx, item.ID, resources)
}
