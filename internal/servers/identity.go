package servers

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SystemIdentity describes one stable Shippin server UUID and explicit external identities.
// Hostname is observed; aliases, tags, and AgentHub identifiers are operator-maintained.
type SystemIdentity struct {
	ServerID          string                `json:"server_id"`
	Label             string                `json:"label"`
	Hostname          string                `json:"hostname,omitempty"`
	HostnameSource    string                `json:"hostname_source"`
	HostnameObserved  *time.Time            `json:"hostname_observed_at,omitempty"`
	Aliases           []string              `json:"aliases"`
	Tags              []string              `json:"tags"`
	AgentHubTenantID  string                `json:"agenthub_tenant_id,omitempty"`
	AgentHubMachineID string                `json:"agenthub_machine_id,omitempty"`
	UpdatedBy         string                `json:"identity_updated_by,omitempty"`
	UpdatedAt         *time.Time            `json:"identity_updated_at,omitempty"`
	AgentHubCheck     *AgentHubMappingCheck `json:"agenthub_mapping_check,omitempty"`
}

type AgentHubMappingCheck struct {
	Outcome        string    `json:"outcome"`
	TenantID       string    `json:"tenant_id"`
	MachineID      string    `json:"machine_id"`
	PolicyVersion  *int64    `json:"policy_version,omitempty"`
	PolicySHA256   string    `json:"policy_sha256,omitempty"`
	Detail         string    `json:"detail"`
	CheckedBy      string    `json:"checked_by"`
	CheckedAt      time.Time `json:"checked_at"`
	MatchesCurrent bool      `json:"matches_current"`
}

var ErrIdentityInvalid = errors.New("invalid system identity")
var ErrIdentityConflict = errors.New("AgentHub machine is already mapped")

// Identity returns identity facts for an existing Shippin server record.
func (s *Store) Identity(ctx context.Context, id string) (SystemIdentity, error) {
	if uuid.Validate(id) != nil {
		return SystemIdentity{}, ErrNotFound
	}
	var v SystemIdentity
	err := s.Pool.QueryRow(ctx, `SELECT id::text, label, hostname, hostname_source, hostname_observed_at,
		aliases, tags, coalesce(agenthub_tenant_id,''), coalesce(agenthub_machine_id,''), identity_updated_by, identity_updated_at
		FROM servers WHERE id=$1`, id).Scan(&v.ServerID, &v.Label, &v.Hostname, &v.HostnameSource, &v.HostnameObserved,
		&v.Aliases, &v.Tags, &v.AgentHubTenantID, &v.AgentHubMachineID, &v.UpdatedBy, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SystemIdentity{}, ErrNotFound
	}
	if v.Aliases == nil {
		v.Aliases = []string{}
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	var check AgentHubMappingCheck
	err = s.Pool.QueryRow(ctx, `SELECT outcome,tenant_id,machine_id,policy_version,coalesce(policy_sha256,''),detail,checked_by,checked_at
		FROM shippin_agenthub_mapping_checks WHERE server_id=$1 ORDER BY checked_at DESC,id DESC LIMIT 1`, id).Scan(
		&check.Outcome, &check.TenantID, &check.MachineID, &check.PolicyVersion, &check.PolicySHA256, &check.Detail, &check.CheckedBy, &check.CheckedAt)
	if err == nil {
		check.MatchesCurrent = check.TenantID == v.AgentHubTenantID && check.MachineID == v.AgentHubMachineID
		v.AgentHubCheck = &check
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return SystemIdentity{}, err
	} else {
		err = nil
	}
	return v, err
}

// SetIdentity updates descriptive metadata and an explicit AgentHub mapping. It never derives
// identity from hostname, label, slug, provider ID, or tenant defaults.
func (s *Store) SetIdentity(ctx context.Context, v SystemIdentity, by string) (SystemIdentity, error) {
	v.AgentHubTenantID = strings.TrimSpace(v.AgentHubTenantID)
	v.AgentHubMachineID = strings.TrimSpace(v.AgentHubMachineID)
	if uuid.Validate(v.ServerID) != nil || strings.TrimSpace(by) == "" || !validIdentityValues(v) {
		return SystemIdentity{}, ErrIdentityInvalid
	}
	if v.Aliases == nil {
		v.Aliases = []string{}
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	var tenant, machine any
	if v.AgentHubTenantID != "" {
		tenant, machine = v.AgentHubTenantID, v.AgentHubMachineID
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE servers SET label=$2, aliases=$3, tags=$4,
		agenthub_tenant_id=$5, agenthub_machine_id=$6, identity_updated_by=$7, identity_updated_at=now()
		WHERE id=$1 AND state<>'revoked'`, v.ServerID, strings.TrimSpace(v.Label), v.Aliases, v.Tags, tenant, machine, by)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return SystemIdentity{}, ErrIdentityConflict
		}
		return SystemIdentity{}, err
	}
	if tag.RowsAffected() == 0 {
		return SystemIdentity{}, ErrNotFound
	}
	return s.Identity(ctx, v.ServerID)
}

func validIdentityValues(v SystemIdentity) bool {
	label := strings.TrimSpace(v.Label)
	if utf8.RuneCountInString(label) == 0 || utf8.RuneCountInString(label) > 120 || hasControl(label) ||
		(v.AgentHubTenantID == "") != (v.AgentHubMachineID == "") ||
		!validExternalID(v.AgentHubTenantID) || !validExternalID(v.AgentHubMachineID) ||
		len(v.Aliases) > 20 || len(v.Tags) > 40 {
		return false
	}
	return validLabels(v.Aliases, 120) && validLabels(v.Tags, 80)
}

func validExternalID(s string) bool {
	return utf8.RuneCountInString(s) <= 255 && !hasControl(s)
}

func validLabels(items []string, max int) bool {
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
		n := utf8.RuneCountInString(items[i])
		if n == 0 || n > max || hasControl(items[i]) {
			return false
		}
		key := strings.ToLower(items[i])
		if _, ok := seen[key]; ok {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func hasControl(s string) bool { return strings.IndexFunc(s, unicode.IsControl) >= 0 }
