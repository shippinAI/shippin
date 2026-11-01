package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Team is a tenant team with its members (024_teams.sql).
type Team struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	CreatedAt   time.Time    `json:"created_at"`
	Members     []TeamMember `json:"members"`
}

type TeamMember struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// Teams lists tenant's teams, oldest first.
func (d *DB) Teams(ctx context.Context, tenant string) ([]Team, error) {
	rows, err := d.Pool.Query(ctx, `SELECT t.id::text, t.name, t.description, t.created_at,
	  (SELECT coalesce(json_agg(json_build_object('user_id',m.user_id,'role',m.role) ORDER BY m.added_at),'[]')
	     FROM team_members m WHERE m.team_id=t.id)
	  FROM tenant_teams t WHERE t.tenant_id=$1 ORDER BY t.created_at`, tenant)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Team, error) {
		var t Team
		var b []byte
		if err := r.Scan(&t.ID, &t.Name, &t.Description, &t.CreatedAt, &b); err != nil {
			return t, err
		}
		return t, json.Unmarshal(b, &t.Members)
	})
}

// CreateTeam adds a team (audited in the same transaction) and returns its id.
func (d *DB) CreateTeam(ctx context.Context, tenant, name, desc, by string, e AuditEntry) (string, error) {
	id, _ := uuid.NewV7()
	return id.String(), d.inTx(ctx, []AuditEntry{e}, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `INSERT INTO tenant_teams(id,tenant_id,name,description,created_by) VALUES ($1,$2,$3,$4,$5)`,
			id, tenant, name, desc, by)
		return tag.RowsAffected(), err
	})
}

// DeleteTeam removes a team of tenant and its memberships; ErrNotFound if it isn't there.
func (d *DB) DeleteTeam(ctx context.Context, tenant, team string, e AuditEntry) error {
	return d.inTx(ctx, []AuditEntry{e}, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `DELETE FROM tenant_teams WHERE tenant_id=$1 AND id::text=$2`, tenant, team)
		return tag.RowsAffected(), err
	})
}

// SetTeamMember adds or updates a member of a tenant's team.
func (d *DB) SetTeamMember(ctx context.Context, tenant, team, user, role, by string, e AuditEntry) error {
	return d.inTx(ctx, []AuditEntry{e}, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `INSERT INTO team_members(team_id,user_id,role,added_by)
		  SELECT t.id,$3,$4,$5 FROM tenant_teams t WHERE t.tenant_id=$1 AND t.id::text=$2
		  ON CONFLICT (team_id,user_id) DO UPDATE SET role=EXCLUDED.role`, tenant, team, user, role, by)
		return tag.RowsAffected(), err
	})
}

// RemoveTeamMember removes user from one team of tenant.
func (d *DB) RemoveTeamMember(ctx context.Context, tenant, team, user string, e AuditEntry) error {
	return d.inTx(ctx, []AuditEntry{e}, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `DELETE FROM team_members m USING tenant_teams t
		  WHERE m.team_id=t.id AND t.tenant_id=$1 AND t.id::text=$2 AND m.user_id=$3`, tenant, team, user)
		return tag.RowsAffected(), err
	})
}

// RemoveFromAllTeams drops user from every team of tenant (after leaving the org).
func (d *DB) RemoveFromAllTeams(ctx context.Context, tenant, user string) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM team_members m USING tenant_teams t
	  WHERE m.team_id=t.id AND t.tenant_id=$1 AND m.user_id=$2`, tenant, user)
	return err
}
