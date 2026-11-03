package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// AuditChain is the result of re-walking the audit hash chain (audit_chain.sql).
type AuditChain struct {
	Enabled  bool   `json:"enabled"`             // false until the superuser path installed the chain
	Rows     int64  `json:"rows"`                // entries walked
	OK       bool   `json:"ok"`                  // every link and hash matched
	FirstBad int64  `json:"first_bad,omitempty"` // first broken chain_seq (negative = an unlinked row id)
	Head     string `json:"head,omitempty"`      // hex sha256 of the newest entry, for anchoring elsewhere
}

// VerifyAuditChain runs shippin_audit_verify(). A database without the chain reports Enabled=false.
func (d *DB) VerifyAuditChain(ctx context.Context) (AuditChain, error) {
	var c AuditChain
	var bad *int64
	var head *string
	err := d.Pool.QueryRow(ctx, `SELECT n_rows, ok, first_bad, head FROM shippin_audit_verify()`).
		Scan(&c.Rows, &c.OK, &bad, &head)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "42883" { // undefined_function: chain not installed
		return AuditChain{}, nil
	}
	if err != nil {
		return AuditChain{}, err
	}
	c.Enabled = true
	if bad != nil {
		c.FirstBad = *bad
	}
	if head != nil {
		c.Head = *head
	}
	return c, nil
}
