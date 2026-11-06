package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/servers"
)

func TestServerProbeCommandIsTargetBoundDurableAndIdempotent(t *testing.T) {
	deploymentStore, tenantID, serverID := deploymentTestStore(t)
	ctx := context.Background()
	created, _, err := deploymentStore.Create(ctx, tenantID, "operator@example.test", CreateInput{
		ServiceID: "postgres", TargetSystemID: serverID, DesiredState: DesiredPresent,
		IdempotencyKey: uuid.NewString(),
		Config:         json.RawMessage(`{"port":5432,"password":"vaultix://production/services/postgres/password"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	credential := "shph_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if _, err := deploymentStore.pool.Exec(ctx, `UPDATE servers SET cred_sha256=$2,cred_expires_at=now()+interval '1 hour' WHERE id=$1`, serverID, servers.Hash(credential)); err != nil {
		t.Fatal(err)
	}
	hosts := &servers.Store{Pool: deploymentStore.pool}
	payload := json.RawMessage(`{"deployment_id":"` + created.ID + `","generation":1,"kind":"tcp","port":5432}`)
	command, err := hosts.QueueProbeCommand(ctx, serverID, tenantID, created.ID, 1,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", payload, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := hosts.QueueProbeCommand(ctx, serverID, tenantID, created.ID, 1, command.PlanSHA256,
		command.IdempotencyKey, payload, time.Minute)
	if err != nil || replayed.ID != command.ID {
		t.Fatalf("command replay = %+v, %v", replayed, err)
	}
	pending, err := hosts.PendingCommand(ctx, serverID, credential)
	if err != nil || pending == nil || pending.ID != command.ID {
		t.Fatalf("pending command = %+v, %v", pending, err)
	}
	if _, err := hosts.PendingCommand(ctx, serverID, "shph_"+"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"); !errors.Is(err, servers.ErrBadCred) {
		t.Fatalf("wrong credential polled a server command: %v", err)
	}
	if _, err := deploymentStore.pool.Exec(ctx, `UPDATE shippin_server_commands SET expires_at=now()-interval '1 second' WHERE id=$1`, command.ID); err != nil {
		t.Fatal(err)
	}
	renewed, err := hosts.PendingCommand(ctx, serverID, credential)
	if err != nil || renewed == nil || renewed.ID != command.ID || !renewed.ExpiresAt.After(time.Now()) {
		t.Fatalf("expired current-generation command was not safely redelivered: %+v, %v", renewed, err)
	}
	result := json.RawMessage(`{"status":"passing","duration_ms":12}`)
	if _, err := hosts.CompleteCommand(ctx, "shph_"+"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", command.ID, result); !errors.Is(err, servers.ErrBadCred) {
		t.Fatalf("wrong server credential result accepted: %v", err)
	}
	completed, err := hosts.CompleteCommand(ctx, credential, command.ID, result)
	if err != nil || completed.DeploymentID != created.ID || completed.Generation != 1 {
		t.Fatalf("complete command = %+v, %v", completed, err)
	}
	if _, err := hosts.CompleteCommand(ctx, credential, command.ID, result); err != nil {
		t.Fatalf("idempotent result retry: %v", err)
	}
	if _, err := hosts.CompleteCommand(ctx, credential, command.ID, json.RawMessage(`{"status":"failing"}`)); !errors.Is(err, servers.ErrCommandConflict) {
		t.Fatalf("conflicting result accepted: %v", err)
	}
	stored, ok, err := hosts.ProbeCommandResult(ctx, tenantID, command.IdempotencyKey)
	var storedValue, expectedValue any
	_ = json.Unmarshal(stored, &storedValue)
	_ = json.Unmarshal(result, &expectedValue)
	if err != nil || !ok || !reflect.DeepEqual(storedValue, expectedValue) {
		t.Fatalf("stored result = %s, %t, %v", stored, ok, err)
	}
	if pending, err := hosts.PendingCommand(ctx, serverID, credential); err != nil || pending != nil {
		t.Fatalf("completed command redelivered: %+v, %v", pending, err)
	}
}
