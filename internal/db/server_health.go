package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrServerEndpointChanged = errors.New("server endpoint changed during health probe")

// ServerHealthObservation is the latest provider API reachability result for one server.
// It says nothing about guest or application health.
type ServerHealthObservation struct {
	ServerID string    `json:"server_id"`
	Revision int64     `json:"endpoint_revision"`
	State    string    `json:"state"`
	Checked  time.Time `json:"checked_at"`
	Latency  *int64    `json:"latency_ms,omitempty"`
	Version  string    `json:"version,omitempty"`
	Detail   string    `json:"detail,omitempty"`
}

// RecordServerHealthObservation replaces the latest row only if the endpoint is still
// attached at the revision used by the probe. Database row locking fences edits racing it.
func (d *DB) RecordServerHealthObservation(ctx context.Context, o ServerHealthObservation) error {
	if len(o.Version) > 120 {
		o.Version = string([]rune(o.Version)[:120])
	}
	if len(o.Detail) > 500 {
		o.Detail = o.Detail[:500]
	}
	tag, err := d.Pool.Exec(ctx, `INSERT INTO shippin_server_health_observations
		(server_id,endpoint_revision,state,observed_at,latency_ms,version,detail)
		SELECT s.id,s.hv_revision,$3,$4,$5,$6,$7 FROM servers s
		WHERE s.id=$1::uuid AND s.hv_revision=$2 AND s.hv_kind IS NOT NULL
		ON CONFLICT (server_id) DO UPDATE SET endpoint_revision=EXCLUDED.endpoint_revision,
			state=EXCLUDED.state,observed_at=EXCLUDED.observed_at,latency_ms=EXCLUDED.latency_ms,
			version=EXCLUDED.version,detail=EXCLUDED.detail
		WHERE shippin_server_health_observations.endpoint_revision <= EXCLUDED.endpoint_revision`,
		o.ServerID, o.Revision, o.State, o.Checked, o.Latency, o.Version, o.Detail)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrServerEndpointChanged
	}
	return nil
}

// LatestServerHealthObservations returns only observations matching the current attached endpoint revision.
func (d *DB) LatestServerHealthObservations(ctx context.Context) (map[string]ServerHealthObservation, error) {
	rows, err := d.Pool.Query(ctx, `SELECT o.server_id::text,o.endpoint_revision,o.state,o.observed_at,o.latency_ms,o.version,o.detail
		FROM shippin_server_health_observations o JOIN servers s ON s.id=o.server_id
		WHERE s.hv_kind IS NOT NULL AND s.hv_revision=o.endpoint_revision`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ServerHealthObservation{}
	for rows.Next() {
		var o ServerHealthObservation
		if err := rows.Scan(&o.ServerID, &o.Revision, &o.State, &o.Checked, &o.Latency, &o.Version, &o.Detail); err != nil {
			return nil, err
		}
		out[o.ServerID] = o
	}
	return out, rows.Err()
}

// ServerHealthObservation returns the current revision's saved result, if present.
func (d *DB) ServerHealthObservation(ctx context.Context, id string) (ServerHealthObservation, error) {
	var o ServerHealthObservation
	err := d.Pool.QueryRow(ctx, `SELECT o.server_id::text,o.endpoint_revision,o.state,o.observed_at,o.latency_ms,o.version,o.detail
		FROM shippin_server_health_observations o JOIN servers s ON s.id=o.server_id
		WHERE o.server_id=$1::uuid AND s.hv_kind IS NOT NULL AND s.hv_revision=o.endpoint_revision`, id).
		Scan(&o.ServerID, &o.Revision, &o.State, &o.Checked, &o.Latency, &o.Version, &o.Detail)
	if errors.Is(err, pgx.ErrNoRows) {
		return ServerHealthObservation{}, ErrNotFound
	}
	return o, err
}
