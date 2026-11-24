// Package secret_store is the installation's secret store: its Vaultix account, connected in the
// panel with a machine identity (docs/settings.md). Keys staff enter in the panel are written there;
// the database keeps where they are. The identity's own client secret can't be kept in Vaultix, so it
// is sealed in shippin_secret_store with the panel's at-rest key and never returned.
package secret_store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/integrations/vaultix"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// aad binds the sealed client secret to this purpose, so it can't be opened as another sealed value.
const aad = "shippin-secret-store-client-secret"

// timeout bounds each call to Vaultix.
const timeout = 10 * time.Second

// Connection is the secret store as the panel shows it. The client secret is never part of it.
type Connection struct {
	Connected       bool       `json:"connected"`
	Kind            string     `json:"kind"`
	APIURL          string     `json:"api_url"`
	ProjectID       string     `json:"project_id"`
	Environment     string     `json:"environment"`
	Path            string     `json:"path"`
	ClientID        string     `json:"client_id"`
	HasClientSecret bool       `json:"has_client_secret"`
	UpdatedBy       string     `json:"updated_by,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
	CheckedAt       *time.Time `json:"checked_at,omitempty"`
	// CheckError is why the last check failed; "" when it passed.
	CheckError string `json:"check_error,omitempty"`
	// KeysHeld is how many settings sections keep keys in this store.
	KeysHeld int `json:"keys_held"`
}

// stored is the row, with the sealed client secret.
type stored struct {
	Connection
	sealed []byte
}

func read(ctx context.Context, q mcpl.DB, lock bool) (stored, error) {
	var s stored
	sql := `SELECT api_url, project_id, environment, path, client_id, client_secret, updated_by, updated_at, checked_at, check_error
	  FROM shippin_secret_store WHERE id`
	if lock {
		sql += " FOR UPDATE"
	}
	var at time.Time
	err := q.QueryRow(ctx, sql).Scan(&s.APIURL, &s.ProjectID, &s.Environment, &s.Path, &s.ClientID, &s.sealed, &s.UpdatedBy, &at, &s.CheckedAt, &s.CheckError)
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{Connection: Connection{Kind: "vaultix"}}, nil
	}
	if err != nil {
		return stored{}, err
	}
	s.Connected, s.Kind, s.HasClientSecret, s.UpdatedAt = true, "vaultix", len(s.sealed) > 0, &at
	err = q.QueryRow(ctx, `SELECT count(*) FROM shippin_settings WHERE keys <> '{}'::jsonb`).Scan(&s.KeysHeld)
	return s, err
}

// Fields are what staff enter to connect.
type Fields struct {
	// APIURL is the Vaultix address as the panel reaches it, such as https://vaultix.example.com.
	APIURL string `json:"api_url"`
	// ProjectID is the Vaultix project the panel's keys go in.
	ProjectID string `json:"project_id"`
	// Environment is the project's environment slug, such as prod.
	Environment string `json:"environment"`
	// Path is the folder keys go under; each section gets its own folder beneath it.
	Path string `json:"path"`
	// ClientID is the machine identity's Universal Auth client ID.
	ClientID string `json:"client_id"`
	// ClientSecret is the identity's client secret. Leave it out to keep the one saved.
	ClientSecret string `json:"client_secret,omitempty"`
}

var (
	envSlug   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	folder    = regexp.MustCompile(`^(/[A-Za-z0-9_-]{1,64}){1,8}$`)
	projectID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$`)
)

func (f *Fields) check() error {
	f.APIURL = strings.TrimRight(strings.TrimSpace(f.APIURL), "/")
	f.ProjectID, f.Environment = strings.TrimSpace(f.ProjectID), strings.TrimSpace(f.Environment)
	f.Path, f.ClientID, f.ClientSecret = strings.TrimSpace(f.Path), strings.TrimSpace(f.ClientID), strings.TrimSpace(f.ClientSecret)
	if f.Path == "" {
		f.Path = "/shippin"
	}
	u, err := url.Parse(f.APIURL)
	switch {
	case err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return mcpl.Invalid("The Vaultix address must be an https address, such as https://vaultix.example.com, without a password, query or fragment.")
	case u.Scheme == "http" && !local(u.Hostname()):
		return mcpl.Invalid("Use https for Vaultix. Plain http is allowed only for a Vaultix on this machine or a private network name.")
	case !projectID.MatchString(f.ProjectID):
		return mcpl.Invalid("Enter the Vaultix project ID, from the project's settings.")
	case !envSlug.MatchString(f.Environment):
		return mcpl.Invalid("Enter the environment's slug, such as prod.")
	case !folder.MatchString(f.Path):
		return mcpl.Invalid("The folder must look like /shippin: letters, digits, dashes and underscores, up to eight levels.")
	case f.ClientID == "" || len(f.ClientID) > 200:
		return mcpl.Invalid("Enter the machine identity's client ID.")
	case len(f.ClientSecret) > 1000:
		return mcpl.Invalid("That client secret is too long.")
	}
	return nil
}

// local is a host plain http is acceptable for: this machine, or a name only a private network resolves.
func local(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "host.docker.internal" ||
		!strings.Contains(host, ".") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local")
}

// Check signs in as the identity and writes, reads back and deletes a check value in the folder, so
// a connection that is saved is known to work. It names the step that failed.
func Check(ctx context.Context, f Fields, secret string) error {
	c := vaultix.New(f.APIURL, f.ClientID, secret, timeout)
	scope := vaultix.Scope{Project: f.ProjectID, Env: f.Environment, Path: f.Path}
	if _, err := c.List(ctx, vaultix.Scope{Project: f.ProjectID, Env: f.Environment, Path: "/"}); err != nil {
		if strings.Contains(err.Error(), "login") {
			return fmt.Errorf("signing in as the machine identity failed; check the client ID and secret (%v)", err)
		}
		return fmt.Errorf("the identity signed in but can't read project %s, environment %s; give it access to the project (%v)", f.ProjectID, f.Environment, err)
	}
	if err := c.EnsureFolder(ctx, f.ProjectID, f.Environment, f.Path); err != nil {
		return fmt.Errorf("creating the folder %s failed; the identity needs write access (%v)", f.Path, err)
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	name, want := "SHIPPIN_CONNECTION_CHECK_"+strings.ToUpper(hex.EncodeToString(b)), hex.EncodeToString(b)
	if _, err := c.Set(ctx, scope, name, want, true); err != nil {
		return fmt.Errorf("writing a check value in %s failed; the identity needs write access (%v)", f.Path, err)
	}
	_, got, err := c.Reveal(ctx, scope, name)
	delErr := c.Delete(ctx, scope, name)
	switch {
	case err != nil:
		return fmt.Errorf("reading the check value back failed (%v)", err)
	case got != want:
		return errors.New("the check value read back differs from the one written")
	case delErr != nil:
		return fmt.Errorf("deleting the check value %s failed; delete it in Vaultix and give the identity delete access (%v)", name, delErr)
	}
	return nil
}

// Vaultix is ports.Secrets on the connected store. It reads the connection each time it is used, so
// a change in the panel takes effect at once, and keeps one client per connection for its token.
type Vaultix struct {
	Pool   *pgxpool.Pool
	Sealer ports.Sealer

	mu     sync.Mutex
	at     time.Time
	client *vaultix.Client
}

func (v *Vaultix) connect(ctx context.Context) (*vaultix.Client, stored, error) {
	s, err := read(ctx, v.Pool, false)
	if err != nil {
		return nil, s, err
	}
	if !s.Connected {
		return nil, s, ports.ErrNoSecretStore
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.client != nil && v.at.Equal(*s.UpdatedAt) {
		return v.client, s, nil
	}
	plain, err := v.Sealer.Open(s.sealed, aad)
	if err != nil {
		return nil, s, errors.New("the secret store's client secret can't be opened with this panel's key; enter it again in Settings > Secret store")
	}
	v.client, v.at = vaultix.New(s.APIURL, s.ClientID, string(plain), timeout), *s.UpdatedAt
	return v.client, s, nil
}

func (v *Vaultix) Where(ctx context.Context) (ports.SecretRef, error) {
	_, s, err := v.connect(ctx)
	if err != nil {
		return ports.SecretRef{}, err
	}
	return ports.SecretRef{Project: s.ProjectID, Environment: s.Environment, Path: s.Path}, nil
}

func scopeOf(r ports.SecretRef) vaultix.Scope {
	return vaultix.Scope{Project: r.Project, Env: r.Environment, Path: r.Path}
}

func (v *Vaultix) Write(ctx context.Context, r ports.SecretRef, value string) error {
	c, _, err := v.connect(ctx)
	if err != nil {
		return err
	}
	if err := c.EnsureFolder(ctx, r.Project, r.Environment, r.Path); err != nil {
		return err
	}
	_, err = c.Set(ctx, scopeOf(r), r.Name, value, true)
	return err
}

func (v *Vaultix) Read(ctx context.Context, r ports.SecretRef) (string, error) {
	c, _, err := v.connect(ctx)
	if err != nil {
		return "", err
	}
	_, value, err := c.Reveal(ctx, scopeOf(r), r.Name)
	return value, err
}

func (v *Vaultix) Delete(ctx context.Context, r ports.SecretRef) error {
	c, _, err := v.connect(ctx)
	if err != nil {
		return err
	}
	if err := c.Delete(ctx, scopeOf(r), r.Name); err != nil && !errors.Is(err, vaultix.ErrNotFound) {
		return err
	}
	return nil
}
