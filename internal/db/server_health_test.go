package db

import (
	"errors"
	"testing"
	"time"
)

func TestServerHealthObservationIsFencedByEndpointRevision(t *testing.T) {
	d, ctx := testDB(t)
	tenant, _ := seed(t, d, ctx)
	id := newID()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hv_kind,hv_api_url)
		VALUES($1::uuid,$2::uuid,'health-host','Health host','lab','planned','test','incus','https://provider.invalid')`, id, tenant.ID); err != nil {
		t.Fatal(err)
	}
	latency := int64(7)
	first := ServerHealthObservation{ServerID: id, Revision: 1, State: "operational", Checked: time.Now().UTC(), Latency: &latency, Version: "test-version"}
	if err := d.RecordServerHealthObservation(ctx, first); err != nil {
		t.Fatal(err)
	}
	latest, err := d.ServerHealthObservation(ctx, id)
	if err != nil || latest.Revision != 1 || latest.State != "operational" || latest.Latency == nil || *latest.Latency != latency {
		t.Fatalf("saved observation=%+v err=%v", latest, err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE servers SET hv_revision=hv_revision+1 WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordServerHealthObservation(ctx, first); !errors.Is(err, ErrServerEndpointChanged) {
		t.Fatalf("late observation error=%v", err)
	}
	if _, err := d.ServerHealthObservation(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old revision must not be returned as current: %v", err)
	}
	second := ServerHealthObservation{ServerID: id, Revision: 2, State: "down", Checked: time.Now().UTC(), Detail: "Provider API check failed."}
	if err := d.RecordServerHealthObservation(ctx, second); err != nil {
		t.Fatal(err)
	}
	latest, err = d.ServerHealthObservation(ctx, id)
	if err != nil || latest.Revision != 2 || latest.State != "down" {
		t.Fatalf("current observation=%+v err=%v", latest, err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE servers SET hv_kind=NULL,hv_revision=hv_revision+1 WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ServerHealthObservation(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("detached endpoint observation must be hidden: %v", err)
	}
}
