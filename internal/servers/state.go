// Package servers is the CP-P1 server registry: servers an org adds, single-use enrollment tokens,
// host credentials and the heartbeat that decides whether a server is active or stale.
package servers

import (
	"errors"
	"time"
)

// State is the stored lifecycle state. Stale is never stored: it is derived from last_seen_at.
type State string

const (
	Planned State = "planned" // estate row (remote site); nothing contacts it, no token can be issued
	Pending State = "pending" // added, waiting for hostd to redeem a token
	Active  State = "active"  // enrolled; beating (or stale when beats stop)
	Stale   State = "stale"   // derived: active with no beat for missed*interval
	Revoked State = "revoked" // terminal; credential and tokens are dead
)

var (
	ErrNotFound  = errors.New("server not found")
	ErrState     = errors.New("not allowed in this state")
	ErrBadToken  = errors.New("enrollment token invalid, used or expired")
	ErrBadCred   = errors.New("host credential invalid or expired")
	ErrSlugTaken = errors.New("slug taken")
	ErrReadOnly  = errors.New("estate server is read-only")
)

// Clock decides staleness: a beat is missed after Interval; Missed misses make the server stale.
type Clock struct {
	Interval time.Duration
	Missed   int
}

// Effective is what a user sees: active turns stale after Missed silent intervals.
func (c Clock) Effective(st State, lastSeen *time.Time, now time.Time) State {
	if st != Active {
		return st
	}
	if lastSeen == nil || now.Sub(*lastSeen) > time.Duration(c.Missed)*c.Interval {
		return Stale
	}
	return Active
}

// CanIssue: a token may be issued to a pending server (first enroll) or an enrolled one
// (re-enroll, e.g. after its credential expired). Never to planned or revoked rows.
func CanIssue(st State) error {
	switch st {
	case Pending, Active:
		return nil
	}
	return ErrState
}

// CanEnroll mirrors CanIssue at redemption time (the server may have been revoked since).
func CanEnroll(st State) error { return CanIssue(st) }

// CanBeat: only an enrolled server may beat; a revoked server's credential is dead.
func CanBeat(st State) error {
	if st == Active {
		return nil
	}
	return ErrState
}

// CanRevoke: anything an org added may be revoked once; planned estate rows are not the org's.
func CanRevoke(st State) error {
	switch st {
	case Pending, Active:
		return nil
	}
	return ErrState
}
