// Package placement is which hosts an organization may use (docs/editions.md, the placement policy).
// The panel asks the registered module's policy (ext.Module.Placement) if one is set, and Assigned
// otherwise. It imports nothing from the panel's features, so the stores that place work can use it.
package placement

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Querier is the caller's database handle, usually its transaction.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Policy says which hosts an organization may see and place work on.
type Policy interface {
	// Hosts are the IDs of the servers org may use besides the ones it owns. When q is a transaction,
	// what the policy read must not change until that transaction ends, so a host cannot be taken
	// away while work is being placed on it.
	Hosts(ctx context.Context, q Querier, org string) ([]string, error)
}

// Assigned is today's policy, until customer server assignments move to the enterprise module
// (roadmap S3): the servers assigned to the organization in server_assignments.
type Assigned struct{}

func (Assigned) Hosts(ctx context.Context, q Querier, org string) ([]string, error) {
	sql := `SELECT server_id::text FROM server_assignments WHERE tenant_id=$1`
	if _, ok := q.(pgx.Tx); ok {
		sql += ` FOR SHARE`
	}
	rows, err := q.Query(ctx, sql, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
