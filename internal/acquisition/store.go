package acquisition

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/mcpl"
)

// PostgresRepository uses only the caller's MCPL transaction for writes.
// In particular it never commits a page independently of its audit entry.
type PostgresRepository struct{}

var _ Repository = PostgresRepository{}
var _ RetentionRepository = PostgresRepository{}

func signerAAD(id string) string { return "acquisition:signer:v1:" + id }
func leadAAD(source, id string, revision uint64) string {
	return fmt.Sprintf("acquisition:lead:v1:%s:%s:%d", source, id, revision)
}
func validSourceID(id string) bool {
	u, e := uuid.Parse(id)
	return e == nil && u.String() == id
}
func validLabel(s string) bool {
	return s == strings.TrimSpace(s) && len(s) > 0 && len(s) <= 100 && !strings.ContainsAny(s, "\r\n\x00")
}

const sourceSummaryCols = `s.id::text,s.label,s.paused,s.config_generation,s.cursor,s.latest_success_at,
  (SELECT count(*) FROM shippin_acquisition_leads l WHERE l.source_id=s.id AND NOT l.deleted AND l.expires_at>now()),
  octet_length(s.sealed_signer)>0, length(s.ca_pem)>0`

func scanSummary(row pgx.Row) (SourceSummary, error) {
	var s SourceSummary
	var cursor int64
	err := row.Scan(&s.ID, &s.Label, &s.Paused, &s.ConfigGeneration, &cursor, &s.LatestSuccessAt,
		&s.ActiveLeadCount, &s.SignerConfigured, &s.CAConfigured)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	if err != nil || cursor < 0 {
		return SourceSummary{}, ErrUnavailable
	}
	s.Cursor = uint64(cursor)
	return s, nil
}

func (PostgresRepository) List(ctx context.Context, db mcpl.DB) ([]SourceSummary, error) {
	if db == nil {
		return nil, ErrUnavailable
	}
	rows, err := db.Query(ctx, `SELECT `+sourceSummaryCols+` FROM shippin_acquisition_sources s ORDER BY s.created_at,s.id`)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []SourceSummary{}
	for rows.Next() {
		s, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return out, nil
}

type sourceRecord struct {
	config       SourceConfig
	label        string
	generation   int64
	cursor       int64
	paused       bool
	sealedSigner []byte
}

func loadSource(ctx context.Context, db mcpl.DB, id string, lock bool) (sourceRecord, error) {
	var s sourceRecord
	if db == nil {
		return s, ErrUnavailable
	}
	if !validSourceID(id) {
		return s, ErrNotFound
	}
	q := `SELECT id::text,label,origin,host(dial_ip),port,source_tenant,source_project,issuer,audience,ca_pem,sealed_signer,config_generation,cursor,paused FROM shippin_acquisition_sources WHERE id=$1`
	if lock {
		q += " FOR UPDATE"
	}
	err := db.QueryRow(ctx, q, id).Scan(&s.config.ID, &s.label, &s.config.Origin, &s.config.DialIP, &s.config.Port,
		&s.config.SourceTenant, &s.config.SourceProject, &s.config.Issuer, &s.config.Audience, &s.config.CAPEM,
		&s.sealedSigner, &s.generation, &s.cursor, &s.paused)
	if errors.Is(err, pgx.ErrNoRows) {
		return sourceRecord{}, ErrNotFound
	}
	if err != nil || s.generation < 1 || s.cursor < 0 {
		return sourceRecord{}, ErrUnavailable
	}
	return s, nil
}

// Get obtains editable config and its generation from one statement snapshot.
// Separate SELECTs at READ COMMITTED can pair stale settings with a newer CAS generation.
func (PostgresRepository) Get(ctx context.Context, db mcpl.DB, id string) (SourceDetail, error) {
	if db == nil {
		return SourceDetail{}, ErrUnavailable
	}
	if !validSourceID(id) {
		return SourceDetail{}, ErrNotFound
	}
	var d SourceDetail
	var cursor int64
	err := db.QueryRow(ctx, `SELECT `+sourceSummaryCols+`,
 s.origin,host(s.dial_ip),s.port,s.source_tenant,s.source_project,s.issuer,s.audience,s.ca_pem
 FROM shippin_acquisition_sources s WHERE s.id=$1`, id).Scan(
		&d.Summary.ID, &d.Summary.Label, &d.Summary.Paused, &d.Summary.ConfigGeneration,
		&cursor, &d.Summary.LatestSuccessAt, &d.Summary.ActiveLeadCount,
		&d.Summary.SignerConfigured, &d.Summary.CAConfigured,
		&d.Origin, &d.DialIP, &d.Port, &d.SourceTenant, &d.SourceProject,
		&d.Issuer, &d.Audience, &d.CAPEM)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceDetail{}, ErrNotFound
	}
	if err != nil || cursor < 0 || d.Summary.ConfigGeneration < 1 {
		return SourceDetail{}, ErrUnavailable
	}
	d.Summary.Cursor = uint64(cursor)
	return d, nil
}

func (PostgresRepository) Create(ctx context.Context, db mcpl.DB, in CreateCommand, sealer Sealer) (SourceSummary, error) {
	if _, ok := db.(pgx.Tx); !ok || sealer == nil {
		return SourceSummary{}, ErrUnavailable
	}
	id := uuid.NewString()
	c := SourceConfig{ID: id, Origin: in.Origin, DialIP: in.DialIP, Port: in.Port, SourceTenant: in.SourceTenant,
		SourceProject: in.SourceProject, Issuer: in.Issuer, Audience: in.Audience, CAPEM: in.CAPEM}
	if !validLabel(in.Label) || len(in.SigningSeed) != ed25519.SeedSize {
		return SourceSummary{}, ErrInvalidConfig
	}
	if _, _, _, err := validateSource(c); err != nil {
		return SourceSummary{}, ErrInvalidConfig
	}
	sealed, err := sealer.Seal(in.SigningSeed, signerAAD(id))
	if err != nil || len(sealed) == 0 {
		return SourceSummary{}, ErrUnavailable
	}
	_, err = db.Exec(ctx, `INSERT INTO shippin_acquisition_sources(id,label,origin,dial_ip,port,source_tenant,source_project,issuer,audience,ca_pem,sealed_signer)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, in.Label, c.Origin, c.DialIP, c.Port, c.SourceTenant, c.SourceProject, c.Issuer, c.Audience, c.CAPEM, sealed)
	if err != nil {
		return SourceSummary{}, ErrUnavailable
	}
	return scanSummary(db.QueryRow(ctx, `SELECT `+sourceSummaryCols+` FROM shippin_acquisition_sources s WHERE s.id=$1`, id))
}

func (PostgresRepository) Update(ctx context.Context, db mcpl.DB, id string, in UpdateCommand, sealer Sealer) (SourceSummary, error) {
	if _, ok := db.(pgx.Tx); !ok || sealer == nil {
		return SourceSummary{}, ErrUnavailable
	}
	old, err := loadSource(ctx, db, id, false)
	if err != nil {
		return SourceSummary{}, err
	}
	if in.ExpectedConfigGeneration < 1 || old.generation != in.ExpectedConfigGeneration {
		return SourceSummary{}, ErrConflict
	}
	c := old.config
	c.Origin, c.DialIP, c.Port, c.CAPEM = in.Origin, in.DialIP, in.Port, in.CAPEM
	if !validLabel(in.Label) {
		return SourceSummary{}, ErrInvalidConfig
	}
	if _, _, _, err := validateSource(c); err != nil {
		return SourceSummary{}, ErrInvalidConfig
	}
	sealed := old.sealedSigner
	if in.SigningSeed != nil {
		if len(in.SigningSeed) != ed25519.SeedSize {
			return SourceSummary{}, ErrInvalidConfig
		}
		sealed, err = sealer.Seal(in.SigningSeed, signerAAD(id))
		if err != nil || len(sealed) == 0 {
			return SourceSummary{}, ErrUnavailable
		}
	}
	tag, err := db.Exec(ctx, `UPDATE shippin_acquisition_sources SET label=$2,origin=$3,dial_ip=$4,port=$5,ca_pem=$6,sealed_signer=$7,paused=$8,config_generation=config_generation+1,updated_at=now() WHERE id=$1 AND config_generation=$9`,
		id, in.Label, c.Origin, c.DialIP, c.Port, c.CAPEM, sealed, in.Paused, in.ExpectedConfigGeneration)
	if err != nil {
		return SourceSummary{}, ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return SourceSummary{}, ErrConflict
	}
	return scanSummary(db.QueryRow(ctx, `SELECT `+sourceSummaryCols+` FROM shippin_acquisition_sources s WHERE s.id=$1`, id))
}

func (PostgresRepository) Sync(ctx context.Context, db mcpl.DB, id string, fetcher Fetcher, sealer Sealer, now time.Time) (SyncResult, error) {
	if _, ok := db.(pgx.Tx); !ok || fetcher == nil || sealer == nil || now.IsZero() {
		return SyncResult{}, ErrUnavailable
	}
	old, err := loadSource(ctx, db, id, false)
	if err != nil {
		return SyncResult{}, err
	}
	if old.paused {
		return SyncResult{}, ErrConflict
	}
	seed, err := sealer.Open(old.sealedSigner, signerAAD(id))
	if err != nil || len(seed) != ed25519.SeedSize {
		clear(seed)
		return SyncResult{}, ErrUnavailable
	}
	page, err := fetcher.Fetch(ctx, old.config, seed, uint64(old.cursor))
	clear(seed)
	if err != nil {
		return SyncResult{}, ErrUnavailable
	}
	// Validate again at the storage boundary, including injected Fetchers.
	raw, err := json.Marshal(envelope{Version: 1, Items: page.Items, NextCursor: page.Cursor, RetentionDays: page.RetentionDays})
	if err != nil || len(raw) > maxResponseBytes {
		return SyncResult{}, ErrUnavailable
	}
	page, err = decodePage(raw, uint64(old.cursor))
	clear(raw)
	if err != nil || page.Cursor > math.MaxInt64 {
		return SyncResult{}, ErrUnavailable
	}
	seen := make(map[string]bool, len(page.Items))
	for _, lead := range page.Items {
		if seen[lead.ID] {
			return SyncResult{}, ErrUnavailable
		}
		seen[lead.ID] = true
	}
	current, err := loadSource(ctx, db, id, true)
	if err != nil {
		return SyncResult{}, err
	}
	if current.paused || current.generation != old.generation || current.cursor != old.cursor {
		return SyncResult{}, ErrConflict
	}
	applied := 0
	for _, lead := range page.Items {
		if !lead.Deleted {
			// PostgreSQL timestamptz stores microseconds. Canonicalize before
			// sealing and comparing, always rounding down so imported data
			// never outlives the source's exact expiry.
			lead.ExpiresAt = lead.ExpiresAt.UTC().Truncate(time.Microsecond)
		}
		if !lead.Deleted && !lead.ExpiresAt.After(now) {
			// A live source record may expire while its page is in flight.
			_, err = db.Exec(ctx, `DELETE FROM shippin_acquisition_leads WHERE source_id=$1 AND lead_id=$2 AND NOT deleted AND expires_at<=$3`, id, lead.ID, now)
			if err != nil {
				return SyncResult{}, ErrUnavailable
			}
			continue
		}
		var ciphertext []byte
		var expiry any
		if !lead.Deleted {
			lead.EmailVerified = false
			plain, e := json.Marshal(lead)
			if e != nil {
				return SyncResult{}, ErrUnavailable
			}
			ciphertext, e = sealer.Seal(plain, leadAAD(id, lead.ID, lead.Revision))
			clear(plain)
			if e != nil || len(ciphertext) == 0 {
				return SyncResult{}, ErrUnavailable
			}
			expiry = lead.ExpiresAt
		}
		tag, e := db.Exec(ctx, `INSERT INTO shippin_acquisition_leads(source_id,lead_id,revision,expires_at,deleted,sealed_payload)
 VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(source_id,lead_id) DO UPDATE SET revision=EXCLUDED.revision,expires_at=EXCLUDED.expires_at,deleted=EXCLUDED.deleted,sealed_payload=EXCLUDED.sealed_payload,imported_at=now()
 WHERE shippin_acquisition_leads.revision<EXCLUDED.revision AND (NOT shippin_acquisition_leads.deleted OR EXCLUDED.deleted)`,
			id, lead.ID, int64(lead.Revision), expiry, lead.Deleted, ciphertext)
		if e != nil {
			return SyncResult{}, ErrUnavailable
		}
		if tag.RowsAffected() == 1 {
			applied++
		}
	}
	tag, err := db.Exec(ctx, `UPDATE shippin_acquisition_sources SET cursor=$2,latest_success_at=now() WHERE id=$1 AND config_generation=$3 AND cursor=$4 AND NOT paused`, id, int64(page.Cursor), old.generation, old.cursor)
	if err != nil {
		return SyncResult{}, ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return SyncResult{}, ErrConflict
	}
	return SyncResult{Applied: applied, Cursor: page.Cursor, More: page.More}, nil
}

func (PostgresRepository) ListLeads(ctx context.Context, db mcpl.DB, id string, sealer Sealer) (LeadListResult, error) {
	out := LeadListResult{Items: []LeadPreview{}}
	if db == nil || sealer == nil {
		return out, ErrUnavailable
	}
	if _, err := loadSource(ctx, db, id, false); err != nil {
		return out, err
	}
	if db.QueryRow(ctx, `SELECT count(*) FROM shippin_acquisition_leads WHERE source_id=$1 AND NOT deleted AND expires_at>now()`, id).Scan(&out.Total) != nil {
		return LeadListResult{}, ErrUnavailable
	}
	rows, err := db.Query(ctx, `SELECT lead_id,revision,expires_at,sealed_payload FROM shippin_acquisition_leads WHERE source_id=$1 AND NOT deleted AND expires_at>now() ORDER BY revision DESC LIMIT 100`, id)
	if err != nil {
		return LeadListResult{}, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var leadID string
		var revision int64
		var expiry time.Time
		var sealed []byte
		if rows.Scan(&leadID, &revision, &expiry, &sealed) != nil || revision < 1 || !leadIDPattern.MatchString(leadID) {
			return LeadListResult{}, ErrUnavailable
		}
		plain, e := sealer.Open(sealed, leadAAD(id, leadID, uint64(revision)))
		if e != nil {
			return LeadListResult{}, ErrUnavailable
		}
		var l Lead
		e = json.Unmarshal(plain, &l)
		clear(plain)
		if e != nil || l.ID != leadID || l.Revision != uint64(revision) || !l.ExpiresAt.Equal(expiry) || l.Deleted || l.Email == "" || l.EmailVerified {
			return LeadListResult{}, ErrUnavailable
		}
		out.Items = append(out.Items, LeadPreview{ID: l.ID, Revision: l.Revision, Email: l.Email, EmailVerified: false, Status: l.Status,
			ConsentVersion: l.ConsentVersion, CreatedAt: l.CreatedAt, ConsentAt: l.ConsentAt, UpdatedAt: l.UpdatedAt, ExpiresAt: l.ExpiresAt})
	}
	if rows.Err() != nil {
		return LeadListResult{}, ErrUnavailable
	}
	out.Truncated = out.Total > int64(len(out.Items))
	return out, nil
}

func (PostgresRepository) PurgeExpired(ctx context.Context, pool *pgxpool.Pool, now time.Time, batch int) (bool, error) {
	if pool == nil || now.IsZero() || batch < 1 || batch > 10000 {
		return false, ErrUnavailable
	}
	tag, err := pool.Exec(ctx, `WITH expired AS (
 SELECT source_id,lead_id FROM shippin_acquisition_leads
 WHERE NOT deleted AND expires_at<=$1 ORDER BY expires_at,source_id,lead_id
 FOR UPDATE SKIP LOCKED LIMIT $2
 ) DELETE FROM shippin_acquisition_leads l USING expired e
 WHERE l.source_id=e.source_id AND l.lead_id=e.lead_id`, now, batch)
	if err != nil {
		return false, ErrUnavailable
	}
	return tag.RowsAffected() == int64(batch), nil
}
