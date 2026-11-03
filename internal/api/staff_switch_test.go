package api

import (
	"testing"
	"time"
)

func TestFreshAuth(t *testing.T) {
	now := time.Now()
	w := 5 * time.Minute
	for _, c := range []struct {
		at   time.Time
		want bool
	}{{time.Time{}, false}, {now.Add(-time.Minute), true}, {now.Add(-6 * time.Minute), false}, {now.Add(2 * time.Minute), false}} {
		if got := freshAuth(c.at, now, w); got != c.want {
			t.Errorf("freshAuth(%v) = %v, want %v", c.at, got, c.want)
		}
	}
}
