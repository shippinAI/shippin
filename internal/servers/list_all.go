package servers

import "context"

// ListAll is the Operations (staff) view: every server across tenants, the platform's own last.
func (s *Store) ListAll(ctx context.Context) ([]Server, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+cols+` FROM servers ORDER BY tenant_id=$1, slug`, SystemTenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Server{}
	for rows.Next() {
		v, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
