package acquisition

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/mcpl"
)

// Sealer is the platform's purpose-bound at-rest encryption adapter. Callers
// pass distinct AAD for each source signer and source/lead payload.
type Sealer interface {
	Seal([]byte, string) ([]byte, error)
	Open([]byte, string) ([]byte, error)
}

type SourceSummary struct {
	ID               string     `json:"id"`
	Label            string     `json:"label"`
	Paused           bool       `json:"paused"`
	ConfigGeneration int64      `json:"config_generation"`
	Cursor           uint64     `json:"cursor"`
	LatestSuccessAt  *time.Time `json:"latest_success_at,omitempty"`
	ActiveLeadCount  int64      `json:"active_lead_count"`
	SignerConfigured bool       `json:"signer_configured"`
	CAConfigured     bool       `json:"ca_configured"`
}

type SourceDetail struct {
	Summary       SourceSummary `json:"summary"`
	Origin        string        `json:"origin"`
	DialIP        string        `json:"dial_ip"`
	Port          int           `json:"port"`
	SourceTenant  string        `json:"source_tenant"`
	SourceProject string        `json:"source_project"`
	Issuer        string        `json:"issuer"`
	Audience      string        `json:"audience"`
	CAPEM         string        `json:"ca_pem"`
}

// CreateCommand contains the write-only signer seed; it is never an output.
type CreateCommand struct {
	Label, Origin, DialIP, SourceTenant, SourceProject, Issuer, Audience, CAPEM string
	Port                                                                        int
	SigningSeed                                                                 []byte
}

// UpdateCommand carries a generation CAS and explicit pause choice. A nil
// SigningSeed means keep the current key; a nonnil seed rotates it.
type UpdateCommand struct {
	ExpectedConfigGeneration     int64
	Label, Origin, DialIP, CAPEM string
	Port                         int
	SigningSeed                  []byte
	Paused                       bool
}

type SyncResult struct {
	Applied int    `json:"applied"`
	Cursor  uint64 `json:"cursor"`
	More    bool   `json:"more"`
}

type LeadPreview struct {
	ID             string    `json:"id"`
	Revision       uint64    `json:"revision"`
	Email          string    `json:"email"`
	EmailVerified  bool      `json:"email_verified"`
	Status         string    `json:"status"`
	ConsentVersion string    `json:"consent_version"`
	CreatedAt      time.Time `json:"created_at"`
	ConsentAt      time.Time `json:"consent_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type LeadListResult struct {
	Items     []LeadPreview `json:"items"`
	Total     int64         `json:"total"`
	Truncated bool          `json:"truncated"`
}

// Repository performs all source and lead operations on the caller's MCPL DB
// handle, which is the audited transaction for every write. Implementations
// must not substitute a pool for that transaction.
type Repository interface {
	List(context.Context, mcpl.DB) ([]SourceSummary, error)
	Get(context.Context, mcpl.DB, string) (SourceDetail, error)
	Create(context.Context, mcpl.DB, CreateCommand, Sealer) (SourceSummary, error)
	Update(context.Context, mcpl.DB, string, UpdateCommand, Sealer) (SourceSummary, error)
	Sync(context.Context, mcpl.DB, string, Fetcher, Sealer, time.Time) (SyncResult, error)
	ListLeads(context.Context, mcpl.DB, string, Sealer) (LeadListResult, error)
}

// PurgeExpired removes a bounded batch of expired sealed lead payloads and
// returns whether more work remains. Tombstone revision floors are permanent.
type RetentionRepository interface {
	PurgeExpired(context.Context, *pgxpool.Pool, time.Time, int) (bool, error)
}
