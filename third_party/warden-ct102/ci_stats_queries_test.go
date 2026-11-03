package main

import "testing"

func TestComputeStreaks(t *testing.T) {
	cases := []struct {
		name          string
		conclusions   []string // newest first
		current, best int
	}{
		{"empty", nil, 0, 0},
		{"all green", []string{"success", "success", "success"}, 3, 3},
		{"broken today", []string{"failure", "success", "success"}, 0, 2},
		{"current shorter than best", []string{"success", "failure", "success", "success", "success"}, 1, 3},
		{"single failure", []string{"failure"}, 0, 0},
		{"recovering", []string{"success", "success", "failure", "success"}, 2, 2},
	}
	for _, c := range cases {
		cur, best := computeStreaks(c.conclusions)
		if cur != c.current || best != c.best {
			t.Errorf("%s: computeStreaks = (%d,%d), want (%d,%d)", c.name, cur, best, c.current, c.best)
		}
	}
}
