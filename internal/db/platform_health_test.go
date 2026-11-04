package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestPlatformHealthMigrationBackfillsLegacySettings(t *testing.T) {
	d, ctx := testDB(t)
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := d.migrateFS(ctx, migrations, "migrations/*.sql", "046_shippin_platform_health.sql"); err != nil {
		t.Fatal(err)
	}
	legacy := `[{"name":"Legacy database","kind":"database","url":"http://127.0.0.1/health","server_id":"019a0000-0000-7000-8000-0000000000a1","guest_id":"101","timeout_seconds":5},{"name":"Invalid","kind":"database","url":"file:///etc/passwd"}]`
	if _, err := d.Pool.Exec(ctx, `INSERT INTO platform_settings(key,value,updated_by) VALUES('services.probes',$1::jsonb,'test')`, legacy); err != nil {
		t.Fatal(err)
	}
	if err := d.migrateFS(ctx, migrations, "migrations/*.sql"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM shippin_platform_services`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy rows imported=%d err=%v", count, err)
	}
	var serverID, guestID string
	if err := d.Pool.QueryRow(ctx, `SELECT coalesce(server_id::text,''),coalesce(guest_id,'') FROM shippin_platform_services WHERE name='Legacy database'`).Scan(&serverID, &guestID); err != nil {
		t.Fatal(err)
	}
	if serverID != "" || guestID != "" {
		t.Fatalf("orphan provider reference was retained: server=%q guest=%q", serverID, guestID)
	}
	var imported int
	if err := d.Pool.QueryRow(ctx, `SELECT imported_count FROM shippin_platform_health_imports WHERE source='services.probes.setting'`).Scan(&imported); err != nil || imported != 1 {
		t.Fatalf("legacy import marker count=%d err=%v", imported, err)
	}
}

func TestPlatformHealthRegistryLeaseAndObservation(t *testing.T) {
	d, ctx := testDB(t)
	definition := PlatformService{ID: newID(), Name: "registry-test", Kind: "custom", URL: "https://health.example.test/ready", TimeoutSeconds: 3, IntervalSeconds: 30, Enabled: true}
	if err := d.ImportPlatformHealthConfig(ctx, []PlatformService{definition}); err != nil {
		t.Fatal(err)
	}
	if err := d.ImportPlatformHealthConfig(ctx, []PlatformService{definition}); err != nil {
		t.Fatal(err)
	}
	rows, err := d.PlatformServices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != definition.ID || rows[0].Revision != 1 {
		t.Fatalf("unexpected imported registry: %#v", rows)
	}
	audit := AuditEntry{Actor: "test-operator", ActorName: "test", Action: "platform.service"}
	rows[0].Alias = "ready"
	audit.Detail = "update probe alias"
	if err := d.SavePlatformService(ctx, rows[0], false, audit); err != nil {
		t.Fatal(err)
	}
	if err := d.SavePlatformService(ctx, rows[0], false, audit); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale revision update error=%v", err)
	}
	claimed, err := d.ClaimDuePlatformServices(ctx, time.Now().UTC(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("first lease claimed %d rows", len(claimed))
	}
	second, err := d.ClaimDuePlatformServices(ctx, time.Now().UTC(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("overlapping lease claimed %d rows", len(second))
	}
	latency := int64(12)
	resource, _ := json.Marshal(map[string]any{"observed": true})
	obs := PlatformServiceObservation{ID: newID(), ServiceID: definition.ID, Revision: claimed[0].Revision, Configured: true, Healthy: true, LatencyMs: &latency, Detail: "ok", Resource: resource, ObservedAt: time.Now().UTC()}
	if err := d.RecordPlatformServiceObservation(ctx, obs, 30); err != nil {
		t.Fatal(err)
	}
	latest, err := d.LatestPlatformServiceObservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 1 {
		t.Fatalf("latest observation count=%d", len(latest))
	}
	var got map[string]any
	if err := json.Unmarshal(latest[definition.ID], &got); err != nil {
		t.Fatal(err)
	}
	if got["healthy"] != true || got["latency_ms"] != float64(12) {
		t.Fatalf("observation mismatch: %#v", got)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO shippin_platform_service_observations
		(id,service_id,config_revision,observed_at,configured,healthy,detail)
		VALUES($1::uuid,$2::uuid,1,now()-interval '48 hours',true,false,'old')`, newID(), definition.ID); err != nil {
		t.Fatal(err)
	}
	pruned, err := d.PrunePlatformServiceObservations(ctx, time.Now().Add(-24*time.Hour), 1)
	if err != nil || pruned != 1 {
		t.Fatalf("bounded retention prune=%d err=%v", pruned, err)
	}
	var observations int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM shippin_platform_service_observations WHERE service_id=$1::uuid`, definition.ID).Scan(&observations); err != nil || observations != 1 {
		t.Fatalf("retention removed recent observation: count=%d err=%v", observations, err)
	}
	if err := d.DisablePlatformService(ctx, definition.ID, 2, audit); err != nil {
		t.Fatal(err)
	}
	rows, err = d.PlatformServices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Enabled {
		t.Fatalf("soft disable lost or enabled row: %#v", rows)
	}
}

func TestPlatformHealthLeasesAreExclusiveAcrossPools(t *testing.T) {
	d1, ctx := testDB(t)
	d2, err := Open(ctx, os.Getenv("SHIPPIN_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d2.Pool.Close)
	const count = 24
	definitions := make([]PlatformService, 0, count)
	for i := 0; i < count; i++ {
		definitions = append(definitions, PlatformService{ID: newID(), Name: fmt.Sprintf("lease-%02d", i), Kind: "custom",
			URL: "https://health.example.test/ready", TimeoutSeconds: 3, IntervalSeconds: 30, Enabled: true})
	}
	if err := d1.ImportPlatformHealthConfig(ctx, definitions); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	type result struct {
		services []PlatformService
		err      error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, replica := range []*DB{d1, d2} {
		wg.Add(1)
		go func(replica *DB) {
			defer wg.Done()
			<-start
			services, err := replica.ClaimDuePlatformServices(ctx, time.Now().Add(time.Minute), count)
			results <- result{services: services, err: err}
		}(replica)
	}
	close(start)
	wg.Wait()
	close(results)

	claimed := map[string]bool{}
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		for _, service := range got.services {
			if claimed[service.ID] {
				t.Fatalf("service %s was claimed by multiple console replicas", service.ID)
			}
			claimed[service.ID] = true
		}
	}
	if len(claimed) != count {
		t.Fatalf("replicas claimed %d unique services, want %d", len(claimed), count)
	}
}
