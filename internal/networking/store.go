package networking

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ Pool *pgxpool.Pool }

const connectionColumns = `id::text, label, provider, config, credential_ciphertext, sync_interval_seconds,
	status, last_checked_at, coalesce(last_error,''), created_at, updated_at`

const claimedConnectionColumns = `c.id::text, c.label, c.provider, c.config, c.credential_ciphertext, c.sync_interval_seconds,
	c.status, c.last_checked_at, coalesce(c.last_error,''), c.created_at, c.updated_at`

func (s *Store) List(ctx context.Context) ([]Connection, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+connectionColumns+` FROM shippin_network_connections ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Due returns a bounded batch whose configured observation interval elapsed.
func (s *Store) Due(ctx context.Context, limit int) ([]Connection, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.Pool.Query(ctx, `WITH due AS (
		SELECT id FROM shippin_network_connections
		WHERE (lease_until IS NULL OR lease_until <= now())
		  AND (last_checked_at IS NULL OR last_checked_at <= now()-make_interval(secs => sync_interval_seconds))
		ORDER BY coalesce(last_checked_at,'-infinity'::timestamptz), created_at
		LIMIT $1 FOR UPDATE SKIP LOCKED
	)
	UPDATE shippin_network_connections c SET lease_until=now()+interval '10 minutes'
	FROM due WHERE c.id=due.id
	RETURNING `+claimedConnectionColumns, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id string) (Connection, error) {
	if uuid.Validate(id) != nil {
		return Connection{}, ErrNotFound
	}
	c, err := scanConnection(s.Pool.QueryRow(ctx, `SELECT `+connectionColumns+` FROM shippin_network_connections WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrNotFound
	}
	return c, err
}

func scanConnection(row pgx.Row) (Connection, error) {
	var c Connection
	var raw []byte
	err := row.Scan(&c.ID, &c.Label, &c.Provider, &raw, &c.Credential, &c.SyncIntervalSeconds, &c.Status, &c.LastChecked, &c.LastError, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrNotFound
	}
	c.Config = raw
	c.HasCredential = len(c.Credential) > 0
	c.Credential = nil
	return c, err
}

func (s *Store) Create(ctx context.Context, c Connection, credential []byte, actor string) (Connection, error) {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO shippin_network_connections(id,label,provider,config,credential_ciphertext,sync_interval_seconds,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, c.ID, c.Label, c.Provider, c.Config, credential, c.SyncIntervalSeconds, actor)
	if err != nil {
		return Connection{}, err
	}
	return s.Get(ctx, c.ID)
}

// Update keeps the stored credential when credential is nil; an empty credential clears it only
// when clearCredential is explicitly true.
func (s *Store) Update(ctx context.Context, c Connection, credential []byte, clearCredential bool) (Connection, error) {
	if uuid.Validate(c.ID) != nil {
		return Connection{}, ErrNotFound
	}
	var err error
	switch {
	case clearCredential:
		err = s.exec(ctx, `UPDATE shippin_network_connections SET label=$2,provider=$3,config=$4,sync_interval_seconds=$5,credential_ciphertext=NULL,status='unknown',last_checked_at=NULL,last_error='',updated_at=now() WHERE id=$1`, c.ID, c.Label, c.Provider, c.Config, c.SyncIntervalSeconds)
	case credential != nil:
		err = s.exec(ctx, `UPDATE shippin_network_connections SET label=$2,provider=$3,config=$4,sync_interval_seconds=$5,credential_ciphertext=$6,status='unknown',last_checked_at=NULL,last_error='',updated_at=now() WHERE id=$1`, c.ID, c.Label, c.Provider, c.Config, c.SyncIntervalSeconds, credential)
	default:
		err = s.exec(ctx, `UPDATE shippin_network_connections SET label=$2,provider=$3,config=$4,sync_interval_seconds=$5,status='unknown',last_checked_at=NULL,last_error='',updated_at=now() WHERE id=$1`, c.ID, c.Label, c.Provider, c.Config, c.SyncIntervalSeconds)
	}
	if err != nil {
		return Connection{}, err
	}
	return s.Get(ctx, c.ID)
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if uuid.Validate(id) != nil {
		return ErrNotFound
	}
	return s.exec(ctx, `DELETE FROM shippin_network_connections WHERE id=$1`, id)
}

func (s *Store) Credential(ctx context.Context, id string) ([]byte, error) {
	if uuid.Validate(id) != nil {
		return nil, ErrNotFound
	}
	var b []byte
	err := s.Pool.QueryRow(ctx, `SELECT credential_ciphertext FROM shippin_network_connections WHERE id=$1`, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

func (s *Store) RecordCheck(ctx context.Context, id, status, message string, checked time.Time) error {
	return s.exec(ctx, `UPDATE shippin_network_connections SET status=$2,last_error=$3,last_checked_at=$4,lease_until=NULL,updated_at=now() WHERE id=$1`, id, status, message, checked)
}

func (s *Store) ReleaseClaim(ctx context.Context, id string) error {
	return s.exec(ctx, `UPDATE shippin_network_connections SET lease_until=NULL WHERE id=$1`, id)
}

func (s *Store) ReplaceResources(ctx context.Context, id string, items []Resource) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, item := range items {
		if _, err := tx.Exec(ctx, `INSERT INTO shippin_network_resources(connection_id,external_id,provider,kind,name,state,address,details,observed_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(connection_id,external_id) DO UPDATE SET
			provider=EXCLUDED.provider,kind=EXCLUDED.kind,name=EXCLUDED.name,state=EXCLUDED.state,address=EXCLUDED.address,details=EXCLUDED.details,observed_at=EXCLUDED.observed_at`, id, item.ID, item.Provider, item.Kind, item.Name, item.State, item.Address, rawObject(item.Details), item.ObservedAt); err != nil {
			return err
		}
	}
	// Delete only observations absent from this successful snapshot. Desired server
	// attachments are kept independently, including through temporary provider omissions.
	if len(items) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM shippin_network_resources WHERE connection_id=$1`, id); err != nil {
			return err
		}
	} else {
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM shippin_network_resources WHERE connection_id=$1 AND NOT (external_id = ANY($2::text[]))`, id, ids); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE shippin_network_connections SET status='healthy',last_error='',last_checked_at=now(),lease_until=NULL,updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Resources(ctx context.Context, id string) ([]Resource, error) {
	if uuid.Validate(id) != nil {
		return nil, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `SELECT a.external_id,a.connection_id::text,coalesce(r.provider,c.provider),coalesce(r.kind,'unknown'),coalesce(r.name,a.external_id),coalesce(r.state,''),coalesce(r.address,''),coalesce(r.details,'{}'::jsonb),coalesce(r.observed_at,a.updated_at),coalesce(a.server_id::text,''),coalesce(s.label,''),r.external_id IS NOT NULL
		FROM shippin_network_resource_attachments a JOIN shippin_network_connections c ON c.id=a.connection_id
		LEFT JOIN shippin_network_resources r ON r.connection_id=a.connection_id AND r.external_id=a.external_id
		LEFT JOIN servers s ON s.id=a.server_id WHERE a.connection_id=$1
		UNION ALL
		SELECT r.external_id,r.connection_id::text,r.provider,r.kind,r.name,r.state,r.address,r.details,r.observed_at,coalesce(a.server_id::text,''),coalesce(s.label,''),true
		FROM shippin_network_resources r LEFT JOIN shippin_network_resource_attachments a ON a.connection_id=r.connection_id AND a.external_id=r.external_id
		LEFT JOIN servers s ON s.id=a.server_id WHERE r.connection_id=$1 AND a.connection_id IS NULL ORDER BY 3,5`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Resource{}
	for rows.Next() {
		var v Resource
		if err := rows.Scan(&v.ID, &v.ConnectionID, &v.Provider, &v.Kind, &v.Name, &v.State, &v.Address, &v.Details, &v.ObservedAt, &v.ServerID, &v.ServerLabel, &v.Observed); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AttachResource links an observed provider resource to an existing, non-revoked
// Shippin server without making a provider-side change.
func (s *Store) AttachResource(ctx context.Context, connectionID, externalID, serverID string) error {
	if uuid.Validate(connectionID) != nil || uuid.Validate(serverID) != nil || externalID == "" || len(externalID) > 512 {
		return ErrNotFound
	}
	tag, err := s.Pool.Exec(ctx, `INSERT INTO shippin_network_resource_attachments(connection_id,external_id,server_id)
	SELECT $1,$2,s.id FROM servers s WHERE s.id=$3 AND s.state<>'revoked'
	  AND (EXISTS (SELECT 1 FROM shippin_network_resources r WHERE r.connection_id=$1 AND r.external_id=$2)
	       OR EXISTS (SELECT 1 FROM shippin_network_resource_attachments a WHERE a.connection_id=$1 AND a.external_id=$2))
	ON CONFLICT(connection_id,external_id) DO UPDATE SET server_id=EXCLUDED.server_id,updated_at=now()`, connectionID, externalID, serverID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DetachResource(ctx context.Context, connectionID, externalID string) error {
	if uuid.Validate(connectionID) != nil || externalID == "" || len(externalID) > 512 {
		return ErrNotFound
	}
	return s.exec(ctx, `DELETE FROM shippin_network_resource_attachments WHERE connection_id=$1 AND external_id=$2`, connectionID, externalID)
}

func (s *Store) exec(ctx context.Context, q string, args ...any) error {
	tag, err := s.Pool.Exec(ctx, q, args...)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func rawObject(v json.RawMessage) []byte {
	if len(v) == 0 {
		return []byte(`{}`)
	}
	return v
}
