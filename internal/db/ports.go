package db

import (
	"context"
	"errors"
	"strings"
)

// ErrNoPorts: every SSH port in the configured range is held by a live workspace.
var ErrNoPorts = errors.New("no free SSH port in the configured range")

// ErrNameTaken: a live workspace in the tenant already uses the name.
var ErrNameTaken = errors.New("workspace name already in use")

// NextPort returns the lowest port in [lo, hi] not in used.
func NextPort(used []int, lo, hi int) (int, error) {
	taken := make(map[int]bool, len(used))
	for _, p := range used {
		taken[p] = true
	}
	for p := lo; p <= hi; p++ {
		if !taken[p] {
			return p, nil
		}
	}
	return 0, ErrNoPorts
}

func (d *DB) livePorts(ctx context.Context) ([]int, error) {
	rows, err := d.Pool.Query(ctx, `SELECT ssh_port FROM workspaces WHERE state<>'deleted'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// classify maps a unique violation to which constraint was hit.
func classify(err error) (portClash bool, mapped error) {
	if err == nil {
		return false, nil
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "workspaces_live_ssh_port"):
		return true, err
	case strings.Contains(s, "workspaces_live_name"):
		return false, ErrNameTaken
	}
	return false, err
}
