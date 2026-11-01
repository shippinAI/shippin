package main

// Supabase Postgres pool — userdata source of truth per ADR 0007.
//
// Connection via SUPABASE_DB_URL (transaction pooler, port 6543). The
// pool is process-wide and initialized once during startup. In pre-release
// Warden can run in degraded mode without this pool unless
// WARDEN_SUPABASE_REQUIRED is set.
//
// Required env:
//   SUPABASE_DB_URL — full postgres URL (postgresql://… ?sslmode=require)
//
// Optional env (with defaults):
//   SUPABASE_DB_POOL_MAX_CONNS         (default 8)
//   SUPABASE_DB_POOL_MIN_CONNS         (default 0)
//   SUPABASE_DB_POOL_MAX_CONN_LIFETIME (default 30m, time.Duration string)

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	supabasePool   *pgxpool.Pool
	supabasePoolMu sync.RWMutex
)

// initSupabase builds the pool from env, validates with `select 1`, and
// installs it as the process-wide pool. Caller decides whether failure is
// fatal based on the deployment profile.
func initSupabase(ctx context.Context) error {
	url := os.Getenv("SUPABASE_DB_URL")
	if url == "" {
		return fmt.Errorf("SUPABASE_DB_URL not set; warden requires Supabase connection per ADR 0007")
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return fmt.Errorf("Supabase DB URL parse failed: %w", err)
	}

	// Supabase transaction pooler (port 6543) reuses backend connections
	// across clients, so pgx's prepared-statement cache collides with
	// SQLSTATE 42P05. QueryExecModeExec disables the cache.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	cfg.MaxConns = envInt32("SUPABASE_DB_POOL_MAX_CONNS", 8)
	cfg.MinConns = envInt32("SUPABASE_DB_POOL_MIN_CONNS", 0)
	cfg.MaxConnLifetime = envDuration("SUPABASE_DB_POOL_MAX_CONN_LIFETIME", 30*time.Minute)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("Supabase pool create failed: %w", err)
	}

	var one int
	if err := pool.QueryRow(ctx, "select 1").Scan(&one); err != nil {
		pool.Close()
		return fmt.Errorf("Supabase select 1 failed: %w", err)
	}

	supabasePoolMu.Lock()
	supabasePool = pool
	supabasePoolMu.Unlock()

	log.Printf("supabase pool ready (host=%q max_conns=%d min_conns=%d max_lifetime=%s)",
		cfg.ConnConfig.Host, cfg.MaxConns, cfg.MinConns, cfg.MaxConnLifetime)
	return nil
}

func supabaseRequired() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("WARDEN_SUPABASE_REQUIRED")))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "required"
}

// getSupabasePool returns the live pool. Returns nil if init never ran.
func getSupabasePool() *pgxpool.Pool {
	supabasePoolMu.RLock()
	p := supabasePool
	supabasePoolMu.RUnlock()
	return p
}

// closeSupabase tears down the pool. Idempotent.
func closeSupabase() {
	supabasePoolMu.Lock()
	p := supabasePool
	supabasePool = nil
	supabasePoolMu.Unlock()

	if p == nil {
		return
	}
	p.Close()
	log.Printf("supabase pool closed")
}

// envInt32 reads an int env var with a default. Invalid values fall back
// to the default and emit a warning.
func envInt32(key string, def int32) int32 {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		log.Printf("Supabase env %s=%q invalid: %v; using default %d", key, raw, err, def)
		return def
	}
	return int32(n)
}

// envDuration reads a Go duration env var with a default. Invalid values
// fall back to the default and emit a warning.
func envDuration(key string, def time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("Supabase env %s=%q invalid: %v; using default %s", key, raw, err, def)
		return def
	}
	return d
}
