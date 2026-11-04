package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
)

func deploymentTestStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	connection := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	database, err := db.Open(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		database.Pool.Close()
		t.Fatal(err)
	}
	database.Pool.Close()
	database, err = db.Open(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Pool.Close)

	tenantID := uuid.NewString()
	serverID := uuid.NewString()
	if _, err := database.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan,status)
		VALUES($1,$2,'Deployment test','starter','active')`, tenantID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hv_kind)
		VALUES($1,$2,'deployment-test','Deployment target','test','active','test','incus')`, serverID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, `INSERT INTO server_assignments(server_id,tenant_id,role,created_by)
		VALUES($1,$2,'owner','test')`, serverID, tenantID); err != nil {
		t.Fatal(err)
	}
	configSchema := `{"type":"object","properties":{"port":{"type":"integer","minimum":1,"maximum":65535},"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false}`
	defaults := `{"port":5432,"password":"vaultix://org/project/env/postgres"}`
	source := `{"kind":"github","reference":"https://github.com/acme/postgres","commit":"0123456789abcdef0123456789abcdef01234567","credential_ref":"sops://runtime/secrets.yaml#/registry/token"}`
	if _, err := database.Pool.Exec(ctx, `INSERT INTO catalog_services(id,label,kind,config_schema,default_config,deployment_source)
		VALUES('postgres','Postgres','database',$1::jsonb,$2::jsonb,$3::jsonb)`, configSchema, defaults, source); err != nil {
		t.Fatal(err)
	}
	return NewStore(database.Pool), tenantID, serverID
}

func TestStoreCreateReplayTenantScopeEventsAndAppendOnlyHistory(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	key := uuid.NewString()
	input := CreateInput{ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: key, Config: json.RawMessage(`{"port":5432,"password":"vaultix://org/project/env/postgres"}`)}
	created, isNew, err := store.Create(ctx, tenantID, "operator@example.test", input)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew || created.Generation != 1 || created.ObservedState != ObservedUnknown || created.TargetProvider != ProviderIncus {
		t.Fatalf("unexpected created deployment: new=%t deployment=%+v", isNew, created)
	}
	if created.Source.Kind != "github" || created.Source.Commit != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("pinned source snapshot missing: %+v", created.Source)
	}
	replayed, isNew, err := store.Create(ctx, tenantID, "operator@example.test", input)
	if err != nil {
		t.Fatal(err)
	}
	if isNew || replayed.ID != created.ID || replayed.Generation != created.Generation {
		t.Fatalf("idempotency replay created new desired state: new=%t deployment=%+v", isNew, replayed)
	}
	changed := input
	changed.Config = json.RawMessage(`{"port":5433,"password":"vaultix://org/project/env/postgres"}`)
	if _, _, err := store.Create(ctx, tenantID, "operator@example.test", changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("reused key with changed input: got %v", err)
	}
	update := UpdateInput{DesiredState: DesiredPresent, IdempotencyKey: uuid.NewString(),
		Config: json.RawMessage(`{"port":5433,"password":"vaultix://org/project/env/postgres"}`)}
	updated, isNew, err := store.Update(ctx, tenantID, created.ID, "operator@example.test", 1, update)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew || updated.Generation != 2 || updated.ObservedState != ObservedPending {
		t.Fatalf("unexpected updated generation: new=%t deployment=%+v", isNew, updated)
	}
	replayedUpdate, isNew, err := store.Update(ctx, tenantID, created.ID, "operator@example.test", 1, update)
	if err != nil || isNew || replayedUpdate.Generation != 2 {
		t.Fatalf("update replay: new=%t deployment=%+v err=%v", isNew, replayedUpdate, err)
	}
	if _, _, err := store.Update(ctx, tenantID, created.ID, "operator@example.test", 1,
		UpdateInput{DesiredState: DesiredPresent, IdempotencyKey: uuid.NewString(), Config: update.Config}); !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("stale generation update: got %v", err)
	}
	secondUpdate := UpdateInput{DesiredState: DesiredPresent, IdempotencyKey: uuid.NewString(), Config: update.Config}
	if later, isNew, err := store.Update(ctx, tenantID, created.ID, "operator@example.test", 2, secondUpdate); err != nil || !isNew || later.Generation != 3 {
		t.Fatalf("second update: new=%t deployment=%+v err=%v", isNew, later, err)
	}
	createReplayAfterUpdates, isNew, err := store.Create(ctx, tenantID, "operator@example.test", input)
	if err != nil || isNew || createReplayAfterUpdates.Generation != 1 {
		t.Fatalf("delayed create replay: new=%t deployment=%+v err=%v", isNew, createReplayAfterUpdates, err)
	}
	updateReplayAfterUpdates, isNew, err := store.Update(ctx, tenantID, created.ID, "operator@example.test", 1, update)
	if err != nil || isNew || updateReplayAfterUpdates.Generation != 2 {
		t.Fatalf("delayed update replay: new=%t deployment=%+v err=%v", isNew, updateReplayAfterUpdates, err)
	}
	if _, err := store.Get(ctx, uuid.NewString(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant read: got %v", err)
	}
	rows, err := store.List(ctx, tenantID, 20)
	if err != nil || len(rows) != 1 || rows[0].ID != created.ID || rows[0].Generation != 3 {
		t.Fatalf("tenant list: rows=%v err=%v", rows, err)
	}
	events, err := store.Events(ctx, tenantID, created.ID, 20)
	if err != nil || len(events) != 3 || events[0].Type != EventRequested || events[2].Generation != 3 {
		t.Fatalf("request events: rows=%v err=%v", events, err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE shippin_deployment_events SET actor='changed' WHERE id=$1`, events[0].ID); err == nil {
		t.Fatal("append-only event update succeeded")
	}
	if _, err := store.pool.Exec(ctx, `DELETE FROM shippin_deployment_generations WHERE deployment_id=$1`, created.ID); err == nil {
		t.Fatal("append-only generation delete succeeded")
	}
}

func TestStoreCreateRejectsUnassignedTargetAndUnsafeConfig(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	input := CreateInput{ServiceID: "postgres", TargetSystemID: uuid.NewString(), DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://org/project/env/postgres"}`)}
	if _, _, err := store.Create(ctx, tenantID, "operator@example.test", input); !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("unassigned target: got %v", err)
	}
	input.TargetSystemID = serverID
	input.IdempotencyKey = uuid.NewString()
	input.Config = json.RawMessage(`{"port":5432,"password":"plaintext"}`)
	if _, _, err := store.Create(ctx, tenantID, "operator@example.test", input); err == nil {
		t.Fatal("unsafe config was persisted")
	}
}
