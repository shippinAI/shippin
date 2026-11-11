package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/clickhouse"
)

// How a service is connected.
const (
	ModeBuiltIn  = "built-in"
	ModeExternal = "external"
	ModeOff      = "off"
)

// What a live check found.
const (
	StateUp       = "up"
	StateDown     = "down"
	StateDisabled = "disabled"
)

const clickHouseSetting = "platform.clickhouse"

// Fact is one labelled reading about a service.
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Service is the live state of one service this installation runs on.
type Service struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	State    string `json:"state"`
	Endpoint string `json:"endpoint,omitempty"`
	Version  string `json:"version,omitempty"`
	Facts    []Fact `json:"facts"`
	Problem  string `json:"problem,omitempty"`
	// Changeable says the panel can change this connection itself. Otherwise it is set at install.
	Changeable bool `json:"changeable"`
}

func (h *Handler) listServices(w http.ResponseWriter, r *http.Request, _ string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	writeJSON(w, map[string]any{"items": []Service{h.postgres(ctx), h.clickHouse(ctx), h.dragonfly(ctx)}})
}

func (h *Handler) postgres(ctx context.Context) Service {
	s := Service{ID: "postgres", Name: "PostgreSQL", Mode: modeOf(h.DatabaseURL), State: StateDown, Endpoint: endpointOf(h.DatabaseURL), Facts: []Fact{}}
	var version, role, super string
	var size, tables int64
	err := h.Pool.QueryRow(ctx, `SELECT current_setting('server_version'), current_user, current_setting('is_superuser'),
	  pg_database_size(current_database()), (SELECT count(*) FROM pg_tables WHERE schemaname='public')`).Scan(&version, &role, &super, &size, &tables)
	if err != nil {
		s.Problem = "The database did not answer."
		slog.Error("platform: postgres did not answer", "error", err)
		return s
	}
	s.State, s.Version = StateUp, strings.Fields(version)[0]
	access := "Limited role"
	if super == "on" {
		access = "Superuser. This install should be given its own role."
	}
	s.Facts = append(s.Facts, Fact{"Size", humanBytes(size)}, Fact{"Tables", fmt.Sprint(tables)}, Fact{"Panel connects as", role}, Fact{"Access", access})
	var ok bool
	var rows int64
	switch err := h.Pool.QueryRow(ctx, `SELECT ok, n_rows FROM shippin_audit_verify()`).Scan(&ok, &rows); {
	case err != nil:
		s.Facts = append(s.Facts, Fact{"Audit log", "Not protected"})
	case ok:
		s.Facts = append(s.Facts, Fact{"Audit log", fmt.Sprintf("Protected, %d entries intact", rows)})
	default:
		s.Facts = append(s.Facts, Fact{"Audit log", "Failed its integrity check"})
		s.Problem = "The audit log failed its integrity check."
	}
	return s
}

func (h *Handler) clickHouse(ctx context.Context) Service {
	s := Service{ID: "clickhouse", Name: "ClickHouse", State: StateDisabled, Mode: ModeOff, Facts: []Fact{}, Changeable: true}
	c := h.ClickHouse()
	if c == nil {
		return s
	}
	s.State, s.Endpoint = StateDown, c.Endpoint()
	s.Mode = modeOf(c.URL())
	var info []struct {
		Version string `json:"v"`
		Tables  uint64 `json:"tables"`
		Rows    uint64 `json:"rows"`
		Bytes   uint64 `json:"bytes"`
	}
	if err := c.Query(ctx, `SELECT version() AS v, count() AS tables, sum(total_rows) AS rows, sum(total_bytes) AS bytes
	  FROM system.tables WHERE database = currentDatabase()`, nil, &info); err != nil || len(info) != 1 {
		s.Problem = "ClickHouse did not answer."
		slog.Error("platform: clickhouse did not answer", "error", err)
		return s
	}
	s.State, s.Version = StateUp, info[0].Version
	s.Facts = append(s.Facts, Fact{"Size", humanBytes(int64(info[0].Bytes))}, Fact{"Tables", fmt.Sprint(info[0].Tables)}, Fact{"Rows", fmt.Sprint(info[0].Rows)})
	return s
}

func (h *Handler) dragonfly(ctx context.Context) Service {
	s := Service{ID: "dragonfly", Name: "Dragonfly", State: StateDisabled, Mode: ModeOff, Facts: []Fact{}}
	if h.CachePing == nil {
		return s
	}
	s.State, s.Mode, s.Endpoint = StateUp, modeOf(h.CacheURL), endpointOf(h.CacheURL)
	if err := h.CachePing(ctx); err != nil {
		s.State, s.Problem = StateDown, "Dragonfly did not answer. Limited sign-in attempts are refused until it returns."
	}
	return s
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// clickHouseChoice is stored once an owner changes the connection. URL is sealed: it holds a password.
type clickHouseChoice struct {
	Mode string `json:"mode"`
	URL  []byte `json:"url,omitempty"`
}

func (h *Handler) setClickHouse(w http.ResponseWriter, r *http.Request, actor string) {
	var in struct {
		confirmation
		Mode string `json:"mode"`
		URL  string `json:"url"`
	}
	if !decode(w, r, &in) || !in.check(w) {
		return
	}
	var next *clickhouse.Client
	stored := clickHouseChoice{Mode: in.Mode}
	switch in.Mode {
	case ModeBuiltIn:
		if h.ClickHouseURL == "" {
			auth.Problem(w, http.StatusBadRequest, "This install was not set up with a built-in ClickHouse.")
			return
		}
		c, err := connect(r.Context(), h.ClickHouseURL)
		if err != nil {
			auth.Problem(w, http.StatusBadGateway, err.Error())
			return
		}
		next = c
	case ModeExternal:
		c, err := connect(r.Context(), in.URL)
		if err != nil {
			auth.Problem(w, http.StatusBadGateway, err.Error())
			return
		}
		box, err := h.Seal([]byte(in.URL))
		if err != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "The address could not be protected, so it was not saved.")
			return
		}
		next, stored.URL = c, box
	case ModeOff:
	default:
		auth.Problem(w, http.StatusBadRequest, "Choose built-in, external or off.")
		return
	}
	if !h.Audit(w, r, "platform.clickhouse", "mode "+in.Mode+" · "+strings.TrimSpace(in.Reason)) {
		return
	}
	var err error
	if in.Mode == ModeBuiltIn {
		_, err = h.Pool.Exec(r.Context(), `DELETE FROM platform_settings WHERE key=$1`, clickHouseSetting)
	} else {
		raw, _ := json.Marshal(stored)
		_, err = h.Pool.Exec(r.Context(), `INSERT INTO platform_settings(key,value,updated_by) VALUES($1,$2,$3)
		  ON CONFLICT (key) DO UPDATE SET value=$2, updated_by=$3, updated_at=now()`, clickHouseSetting, raw, actor)
	}
	if err != nil {
		slog.Error("platform: clickhouse choice not saved", "error", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The change could not be saved, so it was not made.")
		return
	}
	h.UseClickHouse(next)
	writeJSON(w, map[string]string{"mode": in.Mode})
}

// connect proves a ClickHouse is usable before the panel depends on it.
func connect(ctx context.Context, raw string) (*clickhouse.Client, error) {
	c, err := clickhouse.Open(raw)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := c.Migrate(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// LoadClickHouse applies a choice an owner saved earlier. It reports what it did; the panel runs either way.
func (h *Handler) LoadClickHouse(ctx context.Context) error {
	var raw []byte
	err := h.Pool.QueryRow(ctx, `SELECT value FROM platform_settings WHERE key=$1`, clickHouseSetting).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var choice clickHouseChoice
	if err := json.Unmarshal(raw, &choice); err != nil {
		return err
	}
	if choice.Mode == ModeOff {
		h.UseClickHouse(nil)
		return nil
	}
	plain, err := h.Unseal(choice.URL)
	if err != nil {
		return fmt.Errorf("the saved ClickHouse address cannot be read: %w", err)
	}
	c, err := connect(ctx, string(plain))
	if err != nil {
		return err
	}
	h.UseClickHouse(c)
	return nil
}
