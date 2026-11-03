package db

import (
	"errors"
	"testing"
)

func TestNextPort(t *testing.T) {
	cases := []struct {
		used   []int
		lo, hi int
		want   int
		err    error
	}{
		{nil, 22000, 22999, 22000, nil},
		{[]int{22000, 22001}, 22000, 22999, 22002, nil},
		{[]int{22000, 22002}, 22000, 22999, 22001, nil}, // freed ports are reused
		{[]int{21999, 23000}, 22000, 22999, 22000, nil}, // out-of-range ports are ignored
		{[]int{22000, 22001}, 22000, 22001, 0, ErrNoPorts},
		{nil, 22001, 22000, 0, ErrNoPorts},
	}
	for i, c := range cases {
		got, err := NextPort(c.used, c.lo, c.hi)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("case %d: got %d, %v; want %d, %v", i, got, err, c.want, c.err)
		}
	}
}

func TestClassifyUniqueViolations(t *testing.T) {
	if clash, _ := classify(errors.New(`duplicate key value violates unique constraint "workspaces_live_ssh_port"`)); !clash {
		t.Error("port clash not detected")
	}
	if _, err := classify(errors.New(`duplicate key value violates unique constraint "workspaces_live_name"`)); !errors.Is(err, ErrNameTaken) {
		t.Error("name clash not mapped")
	}
	if clash, err := classify(nil); clash || err != nil {
		t.Error("nil must stay nil")
	}
}
