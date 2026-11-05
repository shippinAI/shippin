package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
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
	for _, event := range events {
		if event.RecordedAt.IsZero() {
			t.Fatalf("event timestamp missing: %+v", event)
		}
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

func TestDeploymentReviewIsDurableIdempotentAndGenerationBound(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	created, _, err := store.Create(ctx, tenantID, "operator@example.test", CreateInput{
		ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://org/project/env/postgres"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	reviewID := uuid.NewString()
	ticketID := "shp-" + strings.ReplaceAll(reviewID, "-", "")
	key := uuid.NewString()
	request, createdNew, err := store.ReserveReview(ctx, tenantID, created.ID, created.Generation, "operator@example.test", key,
		reviewID, ticketID, strings.Repeat("a", 64))
	if err != nil || !createdNew || request.State != "preparing" || request.AgentHubTicket != ticketID {
		t.Fatalf("reserve request=%+v created=%t err=%v", request, createdNew, err)
	}
	replayed, createdNew, err := store.ReserveReview(ctx, tenantID, created.ID, created.Generation, "operator@example.test", key,
		uuid.NewString(), "shp-ffffffffffffffffffffffffffffffff", strings.Repeat("a", 64))
	if err != nil || createdNew || replayed.ID != request.ID || replayed.AgentHubTicket != ticketID {
		t.Fatalf("review idempotency replay=%+v created=%t err=%v", replayed, createdNew, err)
	}
	if _, _, err := store.ReserveReview(ctx, tenantID, created.ID, created.Generation, "operator@example.test", uuid.NewString(),
		uuid.NewString(), "shp-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", strings.Repeat("b", 64)); !errors.Is(err, ErrReviewConflict) {
		t.Fatalf("same generation with a different action hash was accepted: %v", err)
	}
	updated, err := store.SetReviewState(ctx, request.ID, "operator@example.test", "pending_review", strings.Repeat("c", 64), "")
	if err != nil || updated.State != "pending_review" || updated.TicketSHA256 != strings.Repeat("c", 64) {
		t.Fatalf("update review state=%+v err=%v", updated, err)
	}
	loaded, err := store.GetReview(ctx, tenantID, created.ID, created.Generation)
	if err != nil || loaded.ID != request.ID || loaded.CreatedAt.IsZero() || loaded.UpdatedAt.IsZero() {
		t.Fatalf("load review=%+v err=%v", loaded, err)
	}
	var events int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM shippin_deployment_review_events WHERE review_id=$1`, request.ID).Scan(&events); err != nil || events != 2 {
		t.Fatalf("review transition history count=%d err=%v", events, err)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE shippin_deployment_review_events SET actor='changed' WHERE review_id=$1`, request.ID); err == nil {
		t.Fatal("review event history is not append-only")
	}
	if _, _, err := store.ReserveReview(ctx, uuid.NewString(), created.ID, created.Generation, "operator@example.test", uuid.NewString(),
		uuid.NewString(), "shp-dddddddddddddddddddddddddddddddd", strings.Repeat("a", 64)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant review reservation: %v", err)
	}
}

func TestReviewAuthorizationProvenanceIsFencedAppendOnlyAndIdempotent(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	deployment, _, err := store.Create(ctx, tenantID, "operator@example.test", CreateInput{
		ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://org/project/env/postgres"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	reviewID := uuid.NewString()
	ticketID := "shp-" + strings.ReplaceAll(reviewID, "-", "")
	if _, _, err := store.ReserveReview(ctx, tenantID, deployment.ID, deployment.Generation, "operator@example.test", uuid.NewString(),
		reviewID, ticketID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	ticketSHA := strings.Repeat("c", 64)
	if _, err := store.SetReviewState(ctx, reviewID, "operator@example.test", "pending_review", ticketSHA, ""); err != nil {
		t.Fatal(err)
	}
	provenance := ReviewDecisionProvenance{
		DecisionID: "decision-1", TenantID: "agenthub-tenant", MachineID: "machine-1", TicketID: ticketID,
		TicketSHA256: ticketSHA, PolicySHA256: strings.Repeat("b", 64), PolicyVersion: 3,
		MatchedRules: []string{"deployment-review"}, ActionSHA256: strings.Repeat("a", 64),
	}
	key := uuid.NewString()
	event, err := store.RecordReviewDecision(ctx, tenantID, deployment.ID, deployment.Generation,
		"operator@example.test", key, true, provenance, "")
	if err != nil || event.Type != EventReviewAllowed || event.ReviewProvenance == nil || event.RecordedAt.IsZero() {
		t.Fatalf("review authorization event=%+v err=%v", event, err)
	}
	provenance.DecisionID = "decision-replay"
	replayed, err := store.RecordReviewDecision(ctx, tenantID, deployment.ID, deployment.Generation,
		"operator@example.test", key, true, provenance, "")
	if err != nil || replayed.ID != event.ID || replayed.ReviewProvenance == nil || replayed.ReviewProvenance.DecisionID != "decision-1" {
		t.Fatalf("authorization idempotent replay=%+v err=%v", replayed, err)
	}
	denied, err := store.RecordReviewDecision(ctx, tenantID, deployment.ID, deployment.Generation,
		"operator@example.test", uuid.NewString(), false, ReviewDecisionProvenance{}, "agenthub_review_denied")
	if err != nil || denied.Type != EventReviewDenied || denied.ReviewProvenance != nil {
		t.Fatalf("denial event=%+v err=%v", denied, err)
	}
	events, err := store.Events(ctx, tenantID, deployment.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	var allowedFound, deniedFound bool
	for _, got := range events {
		allowedFound = allowedFound || got.Type == EventReviewAllowed && got.ReviewProvenance != nil && got.ReviewProvenance.PolicyVersion == 3
		deniedFound = deniedFound || got.Type == EventReviewDenied && got.FailureCode == "agenthub_review_denied"
	}
	if !allowedFound || !deniedFound {
		t.Fatalf("review decision provenance missing from event history: %+v", events)
	}
	if _, err := store.pool.Exec(ctx, `UPDATE shippin_deployment_events SET actor='changed' WHERE id=$1`, event.ID); err == nil {
		t.Fatal("review authorization event is not append-only")
	}
	if _, _, err := store.Update(ctx, tenantID, deployment.ID, "operator@example.test", deployment.Generation,
		UpdateInput{DesiredState: DesiredPresent, IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordReviewDecision(ctx, tenantID, deployment.ID, deployment.Generation,
		"operator@example.test", uuid.NewString(), true, provenance, ""); !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("stale generation authorization event was accepted: %v", err)
	}
}

func TestStoreAcceptsAssignedPlannedProxmoxTargetAsDesiredIntent(t *testing.T) {
	store, tenantID, _ := deploymentTestStore(t)
	targetID := uuid.NewString()
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hv_kind)
		VALUES($1,$2,'planned-proxmox','Proxmox API target','lab','planned','test','proxmox')`, targetID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `INSERT INTO server_assignments(server_id,tenant_id,role,created_by)
		VALUES($1,$2,'owner','test')`, targetID, tenantID); err != nil {
		t.Fatal(err)
	}
	input := CreateInput{ServiceID: "postgres", TargetSystemID: targetID, DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://org/project/env/postgres"}`)}
	item, created, err := store.Create(ctx, tenantID, "operator@example.test", input)
	if err != nil || !created || item.TargetProvider != ProviderProxmox || item.ObservedState != ObservedUnknown {
		t.Fatalf("planned Proxmox request should record intent only: created=%t item=%+v err=%v", created, item, err)
	}
}
