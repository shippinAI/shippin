// Package catalog_service is the service catalog of the services domain: what Shippin can turn on for an
// organization, with the config schema and defaults its settings start from and, for a deployable
// service, its pinned source and runtime contract. Staff with catalog.plans.write change it; plans,
// effective config and deployments read it through internal/services/catalog, which owns the shared types and
// the merge and validation rules. Each action is its own file (list.go, get.go, create.go, update.go,
// delete.go), found by cmd/mcplgen.
package catalog_service

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/services/catalog"
)

// Object is a JSON object whose values are open-ended: a config schema is a JSON Schema document and
// defaults hold whatever that schema describes, so only the top level is typed. Values keep their exact
// JSON. A null, an array or a scalar is refused.
type Object map[string]json.RawMessage

var errNotObject = errors.New("config_schema and default_config must each be a JSON object")

func (o *Object) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		return errNotObject
	}
	*o = m
	return nil
}

func (o Object) raw() json.RawMessage {
	b, _ := json.Marshal(o)
	return b
}

// Service is a catalog service as staff read it. A deployment source pins an artifact and a runtime
// contract declares provider compatibility; neither means anything has been deployed.
type Service struct {
	ID               string                    `json:"id"`
	Label            string                    `json:"label"`
	Description      string                    `json:"description"`
	Kind             string                    `json:"kind"`
	DocsURL          string                    `json:"docs_url"`
	ConfigSchema     Object                    `json:"config_schema"`
	DefaultConfig    Object                    `json:"default_config"`
	DeploymentSource *catalog.DeploymentSource `json:"deployment_source"`
	RuntimeContract  *catalog.RuntimeContract  `json:"runtime_contract"`
	// Required services are locked on for every plan.
	Required bool `json:"required"`
	// Inactive services are left out of effective config and deployments.
	Active    bool      `json:"active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Fields are what create and update take; update replaces every one, so a source or contract left out
// is removed. An empty kind is "app", a missing config schema or defaults is {}, and Active defaults to true.
type Fields struct {
	Label            string                    `json:"label"`
	Description      string                    `json:"description"`
	Kind             string                    `json:"kind"`
	DocsURL          string                    `json:"docs_url"`
	ConfigSchema     Object                    `json:"config_schema"`
	DefaultConfig    Object                    `json:"default_config"`
	DeploymentSource *catalog.DeploymentSource `json:"deployment_source"`
	RuntimeContract  *catalog.RuntimeContract  `json:"runtime_contract"`
	Required         bool                      `json:"required"`
	Active           *bool                     `json:"active,omitempty"`
}

// ByID names one catalog service in the route.
type ByID struct {
	ID string `path:"catalogServiceId" json:"-"`
}

var slug = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,39}$`)

// normalize fills the defaults, then checks the definition the way the deployment code will read it: a
// pinned source, a config schema and defaults that pass the deployable-config policy, and a runtime
// contract that matches the source and binds only declared config fields.
func (f *Fields) normalize(id string) error {
	if f.Kind == "" {
		f.Kind = "app"
	}
	if f.ConfigSchema == nil {
		f.ConfigSchema = Object{}
	}
	if f.DefaultConfig == nil {
		f.DefaultConfig = Object{}
	}
	if !slug.MatchString(id) || f.Label == "" {
		return mcpl.Invalid("A service needs a lowercase id, a label, and JSON objects for config schema and defaults.")
	}
	if f.DeploymentSource == nil {
		if f.RuntimeContract != nil {
			return mcpl.Invalid("A runtime contract requires a pinned deployment source.")
		}
		return nil
	}
	if err := f.DeploymentSource.Validate(); err != nil {
		return mcpl.Invalid("The deployment source is invalid: " + err.Error() + ".")
	}
	schema, defaults := f.ConfigSchema.raw(), f.DefaultConfig.raw()
	if err := catalog.ValidateDeploymentConfig(schema, defaults); err != nil {
		return mcpl.Invalid("The deployable service configuration is invalid: " + err.Error() + ".")
	}
	if f.RuntimeContract != nil {
		if err := f.RuntimeContract.Validate(*f.DeploymentSource); err != nil {
			return mcpl.Invalid("The runtime contract is invalid: " + err.Error() + ".")
		}
		if err := catalog.ValidateRuntimeConfigBindings(*f.RuntimeContract, schema, defaults); err != nil {
			return mcpl.Invalid("The runtime environment bindings are invalid: " + err.Error() + ".")
		}
	}
	return nil
}

// jsonb is v as a jsonb parameter: NULL for a missing source or contract.
func jsonb[T any](v *T) any {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

func boolOr(b *bool) bool { return b == nil || *b }

const cols = `id, label, description, kind, docs_url, config_schema, default_config, deployment_source, runtime_contract, required, active, updated_at`

func scan(row pgx.Row) (Service, error) {
	var s Service
	var schema, defaults, source, runtime []byte
	if err := row.Scan(&s.ID, &s.Label, &s.Description, &s.Kind, &s.DocsURL, &schema, &defaults, &source, &runtime, &s.Required, &s.Active, &s.UpdatedAt); err != nil {
		return s, err
	}
	if err := json.Unmarshal(schema, &s.ConfigSchema); err != nil {
		return s, err
	}
	if err := json.Unmarshal(defaults, &s.DefaultConfig); err != nil {
		return s, err
	}
	if source != nil {
		if err := json.Unmarshal(source, &s.DeploymentSource); err != nil {
			return s, err
		}
	}
	if runtime != nil {
		if err := json.Unmarshal(runtime, &s.RuntimeContract); err != nil {
			return s, err
		}
	}
	return s, nil
}

func one(c *mcpl.Call, id string) (Service, error) {
	s, err := scan(c.DB.QueryRow(c.Ctx, `SELECT `+cols+` FROM catalog_services WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return s, mcpl.NotFound("No catalog service " + id + ".")
	}
	return s, err
}

// duplicate names the id a create collided with, instead of a bare "already exists".
func duplicate(err error, id string) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return mcpl.Conflict("The catalog already has a service " + id + ".")
	}
	return err
}
