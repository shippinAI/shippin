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
	defaults := `{"port":5432,"password":"vaultix://production/services/postgres/password"}`
	source := `{"kind":"oci","reference":"registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","credential_ref":"sops://runtime/secrets.yaml#/registry/token"}`
	runtime := `{"version":1,"kind":"incus_oci_container","providers":["incus"]}`
	if _, err := database.Pool.Exec(ctx, `INSERT INTO catalog_services(id,label,kind,config_schema,default_config,deployment_source,runtime_contract)
		VALUES('postgres','Postgres','database',$1::jsonb,$2::jsonb,$3::jsonb,$4::jsonb)`, configSchema, defaults, source, runtime); err != nil {
		t.Fatal(err)
	}
	return NewStore(database.Pool), tenantID, serverID
}

func TestStoreCreateReplayTenantScopeEventsAndAppendOnlyHistory(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	key := uuid.NewString()
	input := CreateInput{ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: key, Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`)}
	created, isNew, err := store.Create(ctx, tenantID, "operator@example.test", input)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew || created.Generation != 1 || created.ObservedState != ObservedUnknown || created.TargetProvider != ProviderIncus {
		t.Fatalf("unexpected created deployment: new=%t deployment=%+v", isNew, created)
	}
	if created.Source.Kind != "oci" || created.Source.Reference != "registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("pinned source snapshot missing: %+v", created.Source)
	}
	outbox, err := store.PendingOutbox(ctx, 100)
	if err != nil || len(outbox) != 1 || outbox[0].TenantID != tenantID || outbox[0].DeploymentID != created.ID || outbox[0].Generation != 1 {
		t.Fatalf("create did not atomically add its generation wake-up: items=%+v err=%v", outbox, err)
	}
	replayed, isNew, err := store.Create(ctx, tenantID, "operator@example.test", input)
	if err != nil {
		t.Fatal(err)
	}
	if isNew || replayed.ID != created.ID || replayed.Generation != created.Generation {
		t.Fatalf("idempotency replay created new desired state: new=%t deployment=%+v", isNew, replayed)
	}
	if outbox, err = store.PendingOutbox(ctx, 100); err != nil || len(outbox) != 1 {
		t.Fatalf("idempotent create replay added an outbox row: items=%+v err=%v", outbox, err)
	}
	changed := input
	changed.Config = json.RawMessage(`{"port":5433,"password":"vaultix://production/services/postgres/password"}`)
	if _, _, err := store.Create(ctx, tenantID, "operator@example.test", changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("reused key with changed input: got %v", err)
	}
	update := UpdateInput{DesiredState: DesiredPresent, IdempotencyKey: uuid.NewString(),
		Config: json.RawMessage(`{"port":5433,"password":"vaultix://production/services/postgres/password"}`)}
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
	outbox, err = store.PendingOutbox(ctx, 100)
	if err != nil || len(outbox) != 3 || outbox[1].Generation != 2 || outbox[2].Generation != 3 {
		t.Fatalf("each new generation must have one ordered outbox row: items=%+v err=%v", outbox, err)
	}
	jobID := uuid.NewString()
	if err := store.MarkOutboxPublished(ctx, outbox[0].ID, jobID); err != nil {
		t.Fatalf("mark outbox published: %v", err)
	}
	if err := store.MarkOutboxPublished(ctx, outbox[0].ID, jobID); err != nil {
		t.Fatalf("same job publication replay must be idempotent: %v", err)
	}
	if err := store.MarkOutboxPublished(ctx, outbox[0].ID, uuid.NewString()); !errors.Is(err, ErrOutboxConflict) {
		t.Fatalf("different publication replay: got %v", err)
	}
	if pending, err := store.PendingOutbox(ctx, 100); err != nil || len(pending) != 2 {
		t.Fatalf("published outbox row was not removed from pending view: items=%+v err=%v", pending, err)
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

func TestStoreGenerationAndOutboxInsertAreAtomic(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	if _, err := store.pool.Exec(ctx, `CREATE FUNCTION shippin_test_reject_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test outbox failure'; END; $$;
		CREATE TRIGGER shippin_test_reject_outbox BEFORE INSERT ON shippin_deployment_outbox
		FOR EACH ROW EXECUTE FUNCTION shippin_test_reject_outbox()`); err != nil {
		t.Fatal(err)
	}
	_, _, err := store.Create(ctx, tenantID, "operator@example.test", CreateInput{
		ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`),
	})
	if err == nil {
		t.Fatal("create succeeded when its transactional outbox insert failed")
	}
	for _, table := range []string{"shippin_deployments", "shippin_deployment_generations", "shippin_deployment_events", "shippin_deployment_outbox"} {
		var count int
		if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("outbox failure left partial intent in %s: count=%d err=%v", table, count, err)
		}
	}
	if _, err := store.pool.Exec(ctx, `DROP TRIGGER shippin_test_reject_outbox ON shippin_deployment_outbox`); err != nil {
		t.Fatal(err)
	}
	created, _, err := store.Create(ctx, tenantID, "operator@example.test", CreateInput{
		ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `CREATE TRIGGER shippin_test_reject_outbox BEFORE INSERT ON shippin_deployment_outbox
		FOR EACH ROW EXECUTE FUNCTION shippin_test_reject_outbox()`); err != nil {
		t.Fatal(err)
	}
	_, _, err = store.Update(ctx, tenantID, created.ID, "operator@example.test", 1, UpdateInput{
		DesiredState: DesiredPresent, IdempotencyKey: uuid.NewString(),
		Config: json.RawMessage(`{"port":5433,"password":"vaultix://production/services/postgres/password"}`),
	})
	if err == nil {
		t.Fatal("update succeeded when its transactional outbox insert failed")
	}
	current, err := store.Get(ctx, tenantID, created.ID)
	if err != nil || current.Generation != 1 {
		t.Fatalf("outbox failure advanced the desired generation: current=%+v err=%v", current, err)
	}
	for table, expected := range map[string]int{"shippin_deployments": 1, "shippin_deployment_generations": 1,
		"shippin_deployment_events": 1, "shippin_deployment_outbox": 1} {
		var count int
		if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != expected {
			t.Fatalf("outbox failure left partial update in %s: count=%d want=%d err=%v", table, count, expected, err)
		}
	}
}

func TestRecordRuntimeUnavailableIsIdempotentAndGenerationFenced(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	item, _, err := store.Create(ctx, tenantID, "operator@example.test", CreateInput{
		ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded, err := store.RecordRuntimeUnavailable(ctx, tenantID, item.ID, "operator@example.test", 1); err != nil || !recorded {
		t.Fatalf("first unsupported result: recorded=%t err=%v", recorded, err)
	}
	if recorded, err := store.RecordRuntimeUnavailable(ctx, tenantID, item.ID, "operator@example.test", 1); err != nil || recorded {
		t.Fatalf("replayed unsupported result: recorded=%t err=%v", recorded, err)
	}
	updated, _, err := store.Update(ctx, tenantID, item.ID, "operator@example.test", 1, UpdateInput{
		DesiredState: DesiredPresent, IdempotencyKey: uuid.NewString(),
		Config: json.RawMessage(`{"port":5433,"password":"vaultix://production/services/postgres/password"}`),
	})
	if err != nil || updated.Generation != 2 || updated.ObservedState != ObservedPending {
		t.Fatalf("next desired generation: deployment=%+v err=%v", updated, err)
	}
	if recorded, err := store.RecordRuntimeUnavailable(ctx, tenantID, item.ID, "operator@example.test", 1); err != nil || recorded {
		t.Fatalf("stale worker result: recorded=%t err=%v", recorded, err)
	}
	current, err := store.Get(ctx, tenantID, item.ID)
	if err != nil || current.Generation != 2 || current.ObservedState != ObservedPending {
		t.Fatalf("stale result changed current state: deployment=%+v err=%v", current, err)
	}
	events, err := store.Events(ctx, tenantID, item.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	failed := 0
	for _, event := range events {
		if event.Type == EventOperationFailed {
			failed++
			if event.Generation != 1 || event.FailureCode != "runtime_unsupported" {
				t.Fatalf("unsupported event was not generation scoped: %+v", event)
			}
		}
	}
	if failed != 1 {
		t.Fatalf("expected one idempotent runtime_unsupported event, got %d", failed)
	}
}

func TestStoreCreateRejectsUnassignedTargetAndUnsafeConfig(t *testing.T) {
	store, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	input := CreateInput{ServiceID: "postgres", TargetSystemID: uuid.NewString(), DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`)}
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
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`),
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
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`),
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

func TestStoreRejectsProviderIncompatibleRuntimeTarget(t *testing.T) {
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
		IdempotencyKey: uuid.NewString(), Config: json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`)}
	item, created, err := store.Create(ctx, tenantID, "operator@example.test", input)
	if !errors.Is(err, ErrInvalidConfig) || created || item != nil {
		t.Fatalf("Incus-only service must reject Proxmox target: created=%t item=%+v err=%v", created, item, err)
	}
	outbox, err := store.PendingOutbox(ctx, 100)
	if err != nil || len(outbox) != 0 {
		t.Fatalf("incompatible target must not create deployment work: outbox=%+v err=%v", outbox, err)
	}
}
