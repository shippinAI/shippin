package config

import (
	"encoding/json"
	"os"
	"strconv"
	"time"
)

// TenantDB is how the console reaches the shared Postgres that holds starter
// databases. Passwords are not part of this config; hostd keeps them in sops.
type TenantDB struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	SSLMode            string `json:"sslmode"`
	StatementTimeoutMS int    `json:"statement_timeout_ms"`
	MaxRows            int    `json:"max_rows"`
}

func (t TenantDB) Timeout() time.Duration {
	ms := t.StatementTimeoutMS
	if ms <= 0 {
		ms = 8000
	}
	if ms > 30000 {
		ms = 30000
	}
	return time.Duration(ms) * time.Millisecond
}

func (t TenantDB) Rows() int {
	if t.MaxRows <= 0 {
		return 200
	}
	if t.MaxRows > 1000 {
		return 1000
	}
	return t.MaxRows
}

func (t TenantDB) PortOrDefault() int {
	if t.Port == 0 {
		return 5432
	}
	return t.Port
}

func (t TenantDB) SSL() string {
	if t.SSLMode == "" {
		return "require"
	}
	return t.SSLMode
}

// loadTenantDB applies env overrides. SHIPPIN_TENANT_DB_SECRETS is a JSON file
// of {"tn_…": "password"} for development. It is never logged.
func (c *Config) loadTenantDB() error {
	if v := os.Getenv("SHIPPIN_TENANT_DB_HOST"); v != "" {
		c.TenantDB.Host = v
	}
	if v := os.Getenv("SHIPPIN_TENANT_DB_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		c.TenantDB.Port = n
	}
	if v := os.Getenv("SHIPPIN_TENANT_DB_SSLMODE"); v != "" {
		c.TenantDB.SSLMode = v
	}
	path := os.Getenv("SHIPPIN_TENANT_DB_SECRETS")
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	secrets := map[string]string{}
	if err := json.Unmarshal(raw, &secrets); err != nil {
		return err
	}
	c.TenantDBSecrets = secrets
	return nil
}
