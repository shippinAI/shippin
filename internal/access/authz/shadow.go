package authz

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// Shadow runs checks next to the existing role logic without changing any outcome, and
// counts agreement so drift is visible before enforce mode. Checks run off the request path.
type Shadow struct {
	A       Authorizer
	Timeout time.Duration
	agree   atomic.Int64
	differ  atomic.Int64
	failed  atomic.Int64
	last    atomic.Pointer[string]
}

// Stats is what the operator panel shows. LastDiffer names the check, never secret data.
type Stats struct {
	Mode       string `json:"mode"`
	Agree      int64  `json:"agree"`
	Differ     int64  `json:"differ"`
	Errors     int64  `json:"errors"`
	LastDiffer string `json:"last_differ,omitempty"`
}

// Compare checks c in the background and records whether OpenFGA agrees with legacy.
func (s *Shadow) Compare(ctx context.Context, c Check, legacy bool) {
	if s == nil || s.A == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(ctx, s.Timeout)
		defer cancel()
		got, err := s.A.Authorize(ctx, c)
		switch {
		case err != nil:
			s.failed.Add(1)
			log.Printf("authz shadow: %s#%s: %v", c.Object, c.Relation, err)
		case got == legacy:
			s.agree.Add(1)
		default:
			s.differ.Add(1)
			d := c.Relation + " on " + c.Object
			s.last.Store(&d)
			log.Printf("authz shadow differs: %s#%s for %s: openfga=%v legacy=%v", c.Object, c.Relation, c.User, got, legacy)
		}
	}()
}

// Snapshot returns the counters.
func (s *Shadow) Snapshot() Stats {
	if s == nil {
		return Stats{Mode: "off"}
	}
	st := Stats{Mode: "shadow", Agree: s.agree.Load(), Differ: s.differ.Load(), Errors: s.failed.Load()}
	if p := s.last.Load(); p != nil {
		st.LastDiffer = *p
	}
	return st
}
