package customer

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"shippin.cloud/console/internal/mcpl"
)

const notDeletable = "Only unlinked draft customers without workspaces can be deleted here; use Tenants for live ones."

// Delete is shippin.tenancy.customer.delete: DELETE /v1/operator/tenancy/customers/{customerId}.
// A live organization is removed through the audited operator delete under Tenants, not here.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Remove a draft customer that was never provisioned and has no workspaces.", Access: mcpl.Staff, Perm: "customers.write"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		ok, err := Exists(c, in.ID)
		if err != nil {
			return mcpl.None{}, err
		}
		if !ok {
			return mcpl.None{}, NotFound()
		}
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM tenants t WHERE t.kind='customer' AND t.id::text=$1 AND t.status='draft' AND starts_with(t.idp_org_id, $2)
		  AND NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.tenant_id=t.id)`, in.ID, unlinkedOrg)
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" || err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.Conflict(notDeletable)
		}
		return mcpl.None{}, err
	},
}
