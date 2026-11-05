package servers

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Endpoint is the hypervisor API attached to one server. TokenEnv names the
// variable that holds the token secret. The secret is not a field.
type Endpoint struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Label    string `json:"label"`
	Site     string `json:"site"`
	Kind     string `json:"kind"`
	BaseURL  string `json:"base_url"`
	Node     string `json:"node"`
	TokenID  string `json:"token_id"`
	TokenEnv string `json:"token_env"`
	CAFile   string `json:"ca_file,omitempty"`
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
	FleetID  string `json:"fleet_id,omitempty"`
	// AllowTenantDeploymentSecrets is an operator acknowledgement that provider
	// administrators can read secrets projected into workload configuration.
	AllowTenantDeploymentSecrets bool `json:"allow_tenant_deployment_secrets"`
	// Revision fences provider observations against endpoint edits and detach.
	Revision int64 `json:"revision"`
	// TrustedCert is one certificate an operator accepted, or one a trust token vouched for.
	TrustedCert string `json:"trusted_cert,omitempty"`
	// HasSecret says a secret is stored. The secret itself never leaves the server.
	HasSecret bool `json:"has_secret"`
	// ClientCert is public. Its key, and any API secret, are sealed and never serialized.
	ClientCert   string `json:"-"`
	SealedKey    []byte `json:"-"`
	SealedSecret []byte `json:"-"`
	// Secret is a secret in memory for one call, such as a connection test. It is never stored.
	Secret string `json:"-"`
}

const epCols = `id, slug, label, site, coalesce(hv_kind,''), coalesce(hv_api_url,''), coalesce(hv_node,''), coalesce(hv_token_id,''), coalesce(hv_token_env,''), coalesce(hv_ca_file,''), coalesce(hv_cert_file,''), coalesce(hv_key_file,''), coalesce(fleet_id::text,''), coalesce(hv_allow_tenant_deployment_secrets,false), hv_revision, coalesce(hv_trusted_cert,''), coalesce(hv_client_cert,''), hv_client_key, hv_secret`

func scanEndpoint(row pgx.Row) (Endpoint, error) {
	var e Endpoint
	err := row.Scan(&e.ID, &e.Slug, &e.Label, &e.Site, &e.Kind, &e.BaseURL, &e.Node, &e.TokenID, &e.TokenEnv, &e.CAFile, &e.CertFile, &e.KeyFile, &e.FleetID, &e.AllowTenantDeploymentSecrets, &e.Revision, &e.TrustedCert, &e.ClientCert, &e.SealedKey, &e.SealedSecret)
	e.HasSecret = len(e.SealedKey) > 0 || len(e.SealedSecret) > 0
	return e, err
}

// Ref is a server the infra role can attach a hypervisor to. Kind is empty until attached.
// Token fields are not included.
type Ref struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Label string `json:"label"`
	Site  string `json:"site"`
	Kind  string `json:"kind,omitempty"`
}

// Registry lists non-revoked servers so an operator can attach an API without already knowing the id.
func (s *Store) Registry(ctx context.Context) ([]Ref, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, slug, label, site, coalesce(hv_kind,'') FROM servers WHERE state <> 'revoked' ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ref{}
	for rows.Next() {
		var r Ref
		if err := rows.Scan(&r.ID, &r.Slug, &r.Label, &r.Site, &r.Kind); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Endpoints lists servers that have a hypervisor kind. Rows with a null kind are not included.
func (s *Store) Endpoints(ctx context.Context) ([]Endpoint, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+epCols+` FROM servers WHERE hv_kind IS NOT NULL ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Endpoint{}
	for rows.Next() {
		e, err := scanEndpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Endpoint returns one server's hypervisor fields. A server with no kind is not found.
func (s *Store) Endpoint(ctx context.Context, id string) (Endpoint, error) {
	if uuid.Validate(id) != nil {
		return Endpoint{}, ErrNotFound
	}
	e, err := scanEndpoint(s.Pool.QueryRow(ctx, `SELECT `+epCols+` FROM servers WHERE id=$1 AND hv_kind IS NOT NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Endpoint{}, ErrNotFound
	}
	return e, err
}

// VisibleEndpoint returns the hypervisor on a server this tenant owns or is assigned.
// An estate row with no assignment is not visible. The token secret is not selected.
func (s *Store) VisibleEndpoint(ctx context.Context, tenant, id string) (Endpoint, error) {
	if uuid.Validate(id) != nil || uuid.Validate(tenant) != nil {
		return Endpoint{}, ErrNotFound
	}
	e, err := scanEndpoint(s.Pool.QueryRow(ctx, `SELECT `+epCols+` FROM servers s
		WHERE s.id=$1 AND s.hv_kind IS NOT NULL AND (
			s.tenant_id=$2 OR EXISTS (SELECT 1 FROM server_assignments a WHERE a.server_id=s.id AND a.tenant_id=$2)
		)`, id, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return Endpoint{}, ErrNotFound
	}
	return e, err
}

// SetEndpoint writes hypervisor fields on an existing server. It does not create a row
// and does not change enrollment state. kind empty clears the attachment.
func (s *Store) SetEndpoint(ctx context.Context, e Endpoint) error {
	if uuid.Validate(e.ID) != nil {
		return ErrNotFound
	}
	var kind any
	if e.Kind != "" {
		kind = e.Kind
	}
	allowSecrets := e.AllowTenantDeploymentSecrets && e.Kind == "incus"
	tag, err := s.Pool.Exec(ctx, `UPDATE servers SET hv_kind=$2, hv_api_url=$3, hv_node=$4, hv_token_id=$5, hv_token_env=$6, hv_ca_file=$7, hv_cert_file=$8, hv_key_file=$9, hv_allow_tenant_deployment_secrets=$10, hv_trusted_cert=$11, hv_client_cert=$12, hv_client_key=$13, hv_secret=$14, fleet_id=CASE WHEN $2::text IS NULL THEN NULL ELSE fleet_id END, hv_revision=hv_revision+1 WHERE id=$1`,
		e.ID, kind, e.BaseURL, e.Node, e.TokenID, e.TokenEnv, e.CAFile, e.CertFile, e.KeyFile, allowSecrets, e.TrustedCert, e.ClientCert, e.SealedKey, e.SealedSecret)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateEndpoint adds a platform host, owned by the system tenant and reached through its hypervisor API. It stays planned:
// no agent enrolls on it and no enrollment token can be issued. The operator page owns these rows.
func (s *Store) CreateEndpoint(ctx context.Context, e Endpoint, by string) (Endpoint, error) {
	if uuid.Validate(e.ID) != nil {
		return Endpoint{}, errors.New("servers: a new host needs its id set first, because its secrets are sealed to it")
	}
	allowSecrets := e.AllowTenantDeploymentSecrets && e.Kind == "incus"
	_, err := s.Pool.Exec(ctx, `INSERT INTO servers(id, tenant_id, slug, label, site, state, created_by, hv_kind, hv_api_url, hv_node, hv_token_id, hv_token_env, hv_ca_file, hv_cert_file, hv_key_file, hv_allow_tenant_deployment_secrets, hv_trusted_cert, hv_client_cert, hv_client_key, hv_secret)
		VALUES($1,'`+SystemTenantID+`',$2,$3,$4,'planned',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		e.ID, e.Slug, e.Label, e.Site, by, e.Kind, e.BaseURL, e.Node, e.TokenID, e.TokenEnv, e.CAFile, e.CertFile, e.KeyFile, allowSecrets, e.TrustedCert, e.ClientCert, e.SealedKey, e.SealedSecret)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return Endpoint{}, ErrSlugTaken
	}
	if err != nil {
		return Endpoint{}, err
	}
	return s.Endpoint(ctx, e.ID)
}
