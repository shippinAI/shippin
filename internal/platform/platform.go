// Package platform owns what an installation runs for itself: its built-in PostgreSQL, ClickHouse and
// Dragonfly, the acknowledgements an owner gives about them, and disaster recovery. It follows the
// context shape in docs/architecture.md: routes, rules and storage live here, and cmd/console mounts them.
package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/clickhouse"
)

// Permissions these routes are gated by.
const (
	PermRead  = "platform.settings.read"
	PermWrite = "platform.settings.write"
)

// Deps is everything this context needs from the rest of the process.
type Deps struct {
	Pool          *pgxpool.Pool
	DatabaseURL   string
	CacheURL      string
	CachePing     func(context.Context) error
	ClickHouse    func() *clickhouse.Client
	UseClickHouse func(*clickhouse.Client)
	// ClickHouseURL is the address the install was set up with, used until an owner changes it.
	ClickHouseURL string
	StateDir      string
	SessionKey    string
	Seal          func(plain []byte) ([]byte, error)
	Unseal        func(box []byte) ([]byte, error)
	// Audit records an action before it runs. It answers the request itself when it cannot.
	Audit func(w http.ResponseWriter, r *http.Request, action, detail string) bool
}

// Handler serves the platform context's API.
type Handler struct{ Deps }

// Route is mounted by the caller, which gates it by Perm.
type Route struct {
	Pattern string
	Perm    string
	H       func(w http.ResponseWriter, r *http.Request, actor string)
}

// Routes lists every endpoint with the permission that guards it.
func (h *Handler) Routes() []Route {
	return []Route{
		{"GET /v1/operator/built-in-services", PermRead, h.listServices},
		{"PUT /v1/operator/built-in-services/clickhouse", PermWrite, h.setClickHouse},
		{"GET /v1/operator/setup-steps", PermRead, h.listSteps},
		{"PUT /v1/operator/setup-steps/{step}", PermWrite, h.acknowledgeStep},
		{"GET /v1/operator/backups", PermRead, h.listBackups},
		{"POST /v1/operator/backups", PermWrite, h.createBackup},
		{"GET /v1/operator/backups/{name}", PermWrite, h.downloadBackup},
		{"DELETE /v1/operator/backups/{name}", PermWrite, h.deleteBackup},
		{"POST /v1/operator/recovery-key", PermWrite, h.revealRecoveryKey},
	}
}

const (
	reasonMin = 5
	reasonMax = 300
)

// confirmation is what every dangerous change here must carry: a reason for the audit log and an
// explicit "I understand", which the panel sends from its checkbox.
type confirmation struct {
	Reason     string `json:"reason"`
	Understood bool   `json:"understood"`
}

func (c confirmation) check(w http.ResponseWriter) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(c.Reason))
	if n < reasonMin || n > reasonMax {
		auth.Problem(w, http.StatusBadRequest, "Say why you are making this change, in 5 to 300 characters.")
		return false
	}
	if !c.Understood {
		auth.Problem(w, http.StatusBadRequest, "Confirm that you understand what this change does.")
		return false
	}
	return true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		auth.Problem(w, http.StatusBadRequest, "The request could not be read.")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// endpointOf names where a connection URL points, without its credentials.
func endpointOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host + strings.TrimRight(u.Path, "/")
}

// modeOf says whether a connection is one the install runs itself or one somebody pointed elsewhere.
func modeOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ModeOff
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); (ip != nil && ip.IsLoopback()) || host == "localhost" || !strings.Contains(host, ".") {
		return ModeBuiltIn
	}
	return ModeExternal
}

var errNoDatabase = errors.New("platform: no database")
