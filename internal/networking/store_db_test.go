package networking

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
)

func TestResourceAttachmentDesiredStateSurvivesObservationGaps(t *testing.T) {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	database, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Pool.Close()
	newID := func() string { id, _ := uuid.NewV7(); return id.String() }
	connectionID, tenantID, serverID, revokedID := newID(), newID(), newID(), newID()
	slug := "network-test-" + connectionID[:8]
	if _, err := database.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES($1,$2,'Network DB test','starter','active')`, tenantID, "network-test-"+connectionID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO shippin_network_connections
		(id,label,provider,config,created_by) VALUES($1,'network db test','custom','{"inventory_url":"https://inventory.example.test"}','test')`, connectionID); err != nil {
		t.Fatal(err)
	}
	for _, server := range []struct{ id, state, slug string }{
		{serverID, "active", slug}, {revokedID, "revoked", slug + "-revoked"},
	} {
		if _, err := database.Pool.Exec(ctx, `INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by) VALUES($1,$2,$3,$3,'test',$4,'test')`, server.id, tenantID, server.slug, server.state); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = database.Pool.Exec(context.Background(), `DELETE FROM shippin_network_connections WHERE id=$1`, connectionID)
		_, _ = database.Pool.Exec(context.Background(), `DELETE FROM servers WHERE id IN ($1,$2)`, serverID, revokedID)
		_, _ = database.Pool.Exec(context.Background(), `DELETE FROM tenants WHERE id=$1`, tenantID)
	})

	store := &Store{Pool: database.Pool}
	resource := Resource{ID: "tunnel/external-1", Provider: Custom, Kind: "tunnel", Name: "edge", Details: json.RawMessage(`{}`), ObservedAt: time.Now().UTC()}
	if err := store.ReplaceResources(ctx, connectionID, []Resource{resource}); err != nil {
		t.Fatal(err)
	}
	if err := store.AttachResource(ctx, connectionID, "not-observed", serverID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unobserved resource attach error=%v, want ErrNotFound", err)
	}
	if err := store.AttachResource(ctx, connectionID, resource.ID, revokedID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked server attach error=%v, want ErrNotFound", err)
	}
	if err := store.AttachResource(ctx, connectionID, resource.ID, serverID); err != nil {
		t.Fatal(err)
	}
	items, err := store.Resources(ctx, connectionID)
	if err != nil || len(items) != 1 || items[0].ServerID != serverID || !items[0].Observed || items[0].ServerLabel != slug {
		t.Fatalf("attached resource projection=%+v err=%v", items, err)
	}
	if err := store.ReplaceResources(ctx, connectionID, nil); err != nil {
		t.Fatal(err)
	}
	items, err = store.Resources(ctx, connectionID)
	if err != nil || len(items) != 1 || items[0].ServerID != serverID || items[0].Observed || items[0].Name != resource.ID {
		t.Fatalf("missing observation did not retain desired association: %+v err=%v", items, err)
	}
	if err := store.AttachResource(ctx, connectionID, resource.ID, serverID); err != nil {
		t.Fatalf("existing desired association should remain attachable after an observation gap: %v", err)
	}
	if err := store.ReplaceResources(ctx, connectionID, []Resource{resource}); err != nil {
		t.Fatal(err)
	}
	items, err = store.Resources(ctx, connectionID)
	if err != nil || len(items) != 1 || items[0].ServerID != serverID || !items[0].Observed {
		t.Fatalf("reattached projection=%+v err=%v", items, err)
	}
	if err := store.DetachResource(ctx, connectionID, resource.ID); err != nil {
		t.Fatal(err)
	}
	items, err = store.Resources(ctx, connectionID)
	if err != nil || len(items) != 1 || items[0].ServerID != "" || !items[0].Observed {
		t.Fatalf("detached projection=%+v err=%v", items, err)
	}
	if err := store.DetachResource(ctx, connectionID, resource.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing detach error=%v, want ErrNotFound", err)
	}
}
