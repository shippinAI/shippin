package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrPlatformServiceLimit = errors.New("platform service registry is full")
var ErrPlatformServiceChanged = errors.New("platform service configuration changed during probe")

// PlatformService is an installation-owned health target. Credentials are deliberately absent.
type PlatformService struct {
	ID, Name, Kind, Alias, URL, Host, TCP, Instance, ServerID, GuestID string
	TimeoutSeconds, IntervalSeconds                                    int
	Enabled                                                            bool
	Revision                                                           int64
	CreatedAt, UpdatedAt                                               time.Time
}

type PlatformServiceObservation struct {
	ID         string          `json:"id"`
	ServiceID  string          `json:"service_id"`
	Revision   int64           `json:"config_revision"`
	Configured bool            `json:"configured"`
	Healthy    bool            `json:"healthy"`
	LatencyMs  *int64          `json:"latency_ms,omitempty"`
	Detail     string          `json:"detail"`
	Resource   json.RawMessage `json:"resource,omitempty"`
	ObservedAt time.Time       `json:"observed_at"`
}

func (d *DB) PlatformServices(ctx context.Context) ([]PlatformService, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id::text,name,kind,coalesce(alias,''),coalesce(url,''),
		coalesce(host_header,''),coalesce(tcp_address,''),instance,coalesce(server_id::text,''),
		coalesce(guest_id,''),timeout_seconds,interval_seconds,enabled,revision,created_at,updated_at
		FROM shippin_platform_services ORDER BY lower(name),id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlatformService{}
	for rows.Next() {
		var s PlatformService
		if err := rows.Scan(&s.ID, &s.Name, &s.Kind, &s.Alias, &s.URL, &s.Host, &s.TCP, &s.Instance, &s.ServerID, &s.GuestID,
			&s.TimeoutSeconds, &s.IntervalSeconds, &s.Enabled, &s.Revision, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SavePlatformService performs optimistic revision checking and audits in the same transaction.
func (d *DB) SavePlatformService(ctx context.Context, s PlatformService, create bool, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('shippin-platform-health-capacity'))`); err != nil {
			return 0, err
		}
		var active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM shippin_platform_services WHERE enabled`).Scan(&active); err != nil {
			return 0, err
		}
		if create {
			if active >= 50 {
				return 0, ErrPlatformServiceLimit
			}
			tag, err := tx.Exec(ctx, `INSERT INTO shippin_platform_services
				(id,name,kind,alias,url,host_header,tcp_address,instance,server_id,guest_id,timeout_seconds,interval_seconds,updated_by)
				VALUES($1::uuid,$2,$3,nullif($4,''),nullif($5,''),nullif($6,''),nullif($7,''),$8,nullif($9,'')::uuid,nullif($10,''),$11,$12,$13)
				ON CONFLICT (id) DO NOTHING`, s.ID, s.Name, s.Kind, s.Alias, s.URL, s.Host, s.TCP, s.Instance, s.ServerID, s.GuestID, s.TimeoutSeconds, s.IntervalSeconds, esActor(es))
			if err != nil {
				return 0, err
			}
			return tag.RowsAffected(), nil
		}
		var enabled bool
		if err := tx.QueryRow(ctx, `SELECT enabled FROM shippin_platform_services WHERE id=$1::uuid`, s.ID).Scan(&enabled); err != nil {
			if err == pgx.ErrNoRows {
				return 0, ErrNotFound
			}
			return 0, err
		}
		if !enabled && active >= 50 {
			return 0, ErrPlatformServiceLimit
		}
		tag, err := tx.Exec(ctx, `UPDATE shippin_platform_services SET name=$2,kind=$3,alias=nullif($4,''),url=nullif($5,''),
			host_header=nullif($6,''),tcp_address=nullif($7,''),instance=$8,server_id=nullif($9,'')::uuid,
			guest_id=nullif($10,''),timeout_seconds=$11,interval_seconds=$12,enabled=true,revision=revision+1,next_probe_at=now(),
			lease_until=NULL,updated_by=$13,updated_at=now()
			WHERE id=$1::uuid AND revision=$14`, s.ID, s.Name, s.Kind, s.Alias, s.URL, s.Host, s.TCP, s.Instance, s.ServerID, s.GuestID,
			s.TimeoutSeconds, s.IntervalSeconds, esActor(es), s.Revision)
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	})
}

func esActor(es []AuditEntry) string {
	if len(es) > 0 && es[0].Actor != "" {
		return es[0].Actor
	}
	return "operator"
}

func (d *DB) DisablePlatformService(ctx context.Context, id string, revision int64, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `UPDATE shippin_platform_services SET enabled=false,revision=revision+1,lease_until=NULL,updated_by=$3,updated_at=now()
			WHERE id=$1::uuid AND revision=$2 AND enabled`, id, revision, esActor(es))
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	})
}

// ImportPlatformHealthConfig imports file defaults once. Existing database definitions win,
// including a deliberately disabled ID, so restart cannot resurrect operator-disabled services.
func (d *DB) ImportPlatformHealthConfig(ctx context.Context, rows []PlatformService) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('shippin-platform-health-config-import'))`); err != nil {
		return err
	}
	var done bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shippin_platform_health_imports WHERE source='services.probes.config')`).Scan(&done); err != nil {
		return err
	}
	if done {
		return tx.Commit(ctx)
	}
	for _, s := range rows {
		if _, err = tx.Exec(ctx, `INSERT INTO shippin_platform_services
			(id,name,kind,alias,url,host_header,tcp_address,instance,server_id,guest_id,timeout_seconds,interval_seconds,created_by,updated_by)
			SELECT $1::uuid,$2,$3,nullif($4,''),nullif($5,''),nullif($6,''),nullif($7,''),$8,servers.id,
			CASE WHEN servers.id IS NULL THEN NULL ELSE nullif($10,'') END,$11,$12,'config-import','config-import'
			FROM (SELECT nullif($9,'')::uuid AS requested_id) requested LEFT JOIN servers ON servers.id=requested.requested_id
			ON CONFLICT(id) DO NOTHING`, s.ID, s.Name, s.Kind, s.Alias, s.URL, s.Host, s.TCP, s.Instance, s.ServerID, s.GuestID, s.TimeoutSeconds, s.IntervalSeconds); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO shippin_platform_health_imports(source,imported_count) VALUES('services.probes.config',$1)`, len(rows)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// LatestPlatformServiceObservations returns only each service's latest durable result.
func (d *DB) LatestPlatformServiceObservations(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := d.Pool.Query(ctx, `SELECT DISTINCT ON(service_id) service_id::text,
		jsonb_build_object('config_revision',config_revision,'observed_at',observed_at,'configured',configured,'healthy',healthy,'latency_ms',latency_ms,'detail',detail,'resource',resource)
		FROM shippin_platform_service_observations ORDER BY service_id,observed_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var id string
		var b []byte
		if err := rows.Scan(&id, &b); err != nil {
			return nil, err
		}
		out[id] = b
	}
	return out, rows.Err()
}

// PlatformServiceObservationHistory returns the newest bounded slice of a service timeline.
func (d *DB) PlatformServiceObservationHistory(ctx context.Context, serviceID string, limit int) ([]PlatformServiceObservation, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id::text,service_id::text,config_revision,configured,healthy,latency_ms,detail,resource,observed_at
		FROM shippin_platform_service_observations WHERE service_id=$1::uuid ORDER BY observed_at DESC,id DESC LIMIT $2`, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlatformServiceObservation{}
	for rows.Next() {
		var o PlatformServiceObservation
		if err := rows.Scan(&o.ID, &o.ServiceID, &o.Revision, &o.Configured, &o.Healthy, &o.LatencyMs, &o.Detail, &o.Resource, &o.ObservedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// PrunePlatformServiceObservations deletes at most limit expired rows per call.
func (d *DB) PrunePlatformServiceObservations(ctx context.Context, before time.Time, limit int) (int64, error) {
	tag, err := d.Pool.Exec(ctx, `DELETE FROM shippin_platform_service_observations WHERE id IN
		(SELECT id FROM shippin_platform_service_observations WHERE observed_at<$1 ORDER BY observed_at,id LIMIT $2)`, before, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ClaimDuePlatformServices uses row locks and leases so multiple console replicas share work.
func (d *DB) ClaimDuePlatformServices(ctx context.Context, now time.Time, limit int) ([]PlatformService, error) {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `WITH due AS (
		SELECT id FROM shippin_platform_services
		WHERE enabled AND next_probe_at <= $1 AND (lease_until IS NULL OR lease_until <= $1)
		ORDER BY next_probe_at,id FOR UPDATE SKIP LOCKED LIMIT $2
	) UPDATE shippin_platform_services s SET lease_until=$1 + interval '2 minutes'
	FROM due WHERE s.id=due.id
	RETURNING s.id::text,s.name,s.kind,coalesce(s.alias,''),coalesce(s.url,''),coalesce(s.host_header,''),
	coalesce(s.tcp_address,''),s.instance,coalesce(s.server_id::text,''),coalesce(s.guest_id,''),
	s.timeout_seconds,s.interval_seconds,s.enabled,s.revision,s.created_at,s.updated_at`, now, limit)
	if err != nil {
		return nil, err
	}
	claimed, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (PlatformService, error) {
		var s PlatformService
		err := r.Scan(&s.ID, &s.Name, &s.Kind, &s.Alias, &s.URL, &s.Host, &s.TCP, &s.Instance, &s.ServerID, &s.GuestID, &s.TimeoutSeconds, &s.IntervalSeconds, &s.Enabled, &s.Revision, &s.CreatedAt, &s.UpdatedAt)
		return s, err
	})
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return claimed, nil
}

// ClaimPlatformServiceNow reserves one enabled service for an explicit operator check.
func (d *DB) ClaimPlatformServiceNow(ctx context.Context, id string, now time.Time) (PlatformService, error) {
	var s PlatformService
	err := d.Pool.QueryRow(ctx, `UPDATE shippin_platform_services SET lease_until=$2 + interval '2 minutes'
		WHERE id=$1::uuid AND enabled AND (lease_until IS NULL OR lease_until<=$2)
		RETURNING id::text,name,kind,coalesce(alias,''),coalesce(url,''),coalesce(host_header,''),coalesce(tcp_address,''),
		instance,coalesce(server_id::text,''),coalesce(guest_id,''),timeout_seconds,interval_seconds,enabled,revision,created_at,updated_at`, id, now).
		Scan(&s.ID, &s.Name, &s.Kind, &s.Alias, &s.URL, &s.Host, &s.TCP, &s.Instance, &s.ServerID, &s.GuestID, &s.TimeoutSeconds, &s.IntervalSeconds, &s.Enabled, &s.Revision, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return PlatformService{}, ErrNotFound
	}
	return s, err
}

// RecordPlatformServiceObservation appends a bounded result and advances that service's cadence.
func (d *DB) RecordPlatformServiceObservation(ctx context.Context, o PlatformServiceObservation, intervalSeconds int, es ...AuditEntry) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var revision int64
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT revision,enabled FROM shippin_platform_services WHERE id=$1::uuid FOR UPDATE`, o.ServiceID).Scan(&revision, &enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if !enabled || revision != o.Revision {
		return ErrPlatformServiceChanged
	}
	if _, err = tx.Exec(ctx, `INSERT INTO shippin_platform_service_observations
		(id,service_id,config_revision,observed_at,configured,healthy,latency_ms,detail,resource)
		VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,left($8,500),nullif($9,'')::jsonb)`, o.ID, o.ServiceID, o.Revision, o.ObservedAt, o.Configured, o.Healthy, o.LatencyMs, o.Detail, string(o.Resource)); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE shippin_platform_services SET lease_until=NULL,next_probe_at=$2::timestamptz + make_interval(secs => $3::double precision)
		WHERE id=$1::uuid AND revision=$4 AND enabled`, o.ServiceID, o.ObservedAt, intervalSeconds, o.Revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrPlatformServiceChanged
	}
	if err = addAuditTx(ctx, tx, es...); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
