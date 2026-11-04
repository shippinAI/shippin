package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// AgentHubConnection contains installation-specific review endpoint material.
// TokenCiphertext must only hold ciphertext produced by the installation sealer.
type AgentHubConnection struct {
	BaseURL         string
	TokenCiphertext []byte
	HasCredential   bool
}

// AgentHubMappingCheck is the redacted result of validating one saved Shippin mapping.
type AgentHubMappingCheck struct {
	ID            string
	ServerID      string
	TenantID      string
	MachineID     string
	Outcome       string
	PolicyVersion *int64
	PolicySHA256  string
	Detail        string
	CheckedBy     string
	CheckedAt     time.Time
}

func (d *DB) AgentHubConnection(ctx context.Context) (AgentHubConnection, error) {
	var c AgentHubConnection
	err := d.Pool.QueryRow(ctx, `SELECT base_url, token_ciphertext FROM shippin_agenthub_connection WHERE id=1`).Scan(&c.BaseURL, &c.TokenCiphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.HasCredential = len(c.TokenCiphertext) > 0
	return c, nil
}

// SaveAgentHubConnection updates endpoint and encrypted token with its audit row atomically.
func (d *DB) SaveAgentHubConnection(ctx context.Context, baseURL string, tokenCiphertext []byte, keepCredential bool, by string, es ...AuditEntry) error {
	if tokenCiphertext == nil {
		tokenCiphertext = []byte{}
	}
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		var tag pgconn.CommandTag
		var err error
		if keepCredential {
			tag, err = tx.Exec(ctx, `INSERT INTO shippin_agenthub_connection(id,base_url,token_ciphertext,updated_by) VALUES(1,$1,$2,$3)
				ON CONFLICT (id) DO UPDATE SET base_url=$1,updated_by=$3,updated_at=now()`, baseURL, tokenCiphertext, by)
		} else {
			tag, err = tx.Exec(ctx, `INSERT INTO shippin_agenthub_connection(id,base_url,token_ciphertext,updated_by) VALUES(1,$1,$2,$3)
				ON CONFLICT (id) DO UPDATE SET base_url=$1,token_ciphertext=$2,updated_by=$3,updated_at=now()`, baseURL, tokenCiphertext, by)
		}
		return tag.RowsAffected(), err
	})
}

// RecordAgentHubMappingCheck appends a mapping observation and its audit entry atomically.
func (d *DB) RecordAgentHubMappingCheck(ctx context.Context, check AgentHubMappingCheck, audit AuditEntry) (AgentHubMappingCheck, error) {
	var policyHash any
	if check.PolicySHA256 != "" {
		policyHash = check.PolicySHA256
	}
	err := d.inTx(ctx, []AuditEntry{audit}, func(tx pgx.Tx) (int64, error) {
		err := tx.QueryRow(ctx, `INSERT INTO shippin_agenthub_mapping_checks
			(id,server_id,tenant_id,machine_id,outcome,policy_version,policy_sha256,detail,checked_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING checked_at`, check.ID, check.ServerID, check.TenantID,
			check.MachineID, check.Outcome, check.PolicyVersion, policyHash, check.Detail, check.CheckedBy).Scan(&check.CheckedAt)
		if err != nil {
			return 0, err
		}
		return 1, nil
	})
	return check, err
}
