package projects

import (
	"errors"
	"sort"
	"testing"
	"time"
)

func TestValidSlug(t *testing.T) {
	cases := map[string]error{
		"box-work": nil, "abc": nil, "a1b": nil, "shippin-console-2": nil, "x23456789012345678901234567890": nil,
		"ab": ErrSlugShape, "x234567890123456789012345678901": ErrSlugShape, "Box-work": ErrSlugShape,
		"1box": ErrSlugShape, "box-": ErrSlugShape, "-box": ErrSlugShape, "box_work": ErrSlugShape, "box.work": ErrSlugShape,
		"box--work": ErrSlugDashes, "core": ErrSlugReserved, "console": ErrSlugReserved, "admin": ErrSlugReserved,
	}
	for in, want := range cases {
		if got := ValidSlug(in); !errors.Is(got, want) {
			t.Errorf("ValidSlug(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPublicIDShapeAndOrder(t *testing.T) {
	a := "prj_" + ulidAt(time.UnixMilli(1_700_000_000_000))
	b := "prj_" + ulidAt(time.UnixMilli(1_700_000_000_001))
	for _, id := range []string{a, b, NewPublicID()} {
		if !ValidPublicID(id) {
			t.Fatalf("bad shape %q", id)
		}
	}
	if s := []string{b, a}; sort.StringsAreSorted(s) {
		t.Fatalf("later id sorted first: %v", s)
	}
	if a[4:14] != "01HF7YAT00" {
		t.Errorf("timestamp prefix = %s, want 01HF7YAT00 (1.7e12 ms)", a[4:14])
	}
	if NewPublicID() == NewPublicID() {
		t.Error("two ids collided")
	}
	for _, bad := range []string{"prj_", "prj_01HF7YAT00ILOU0000000000000", "proj_01HF7YAT000000000000000000", "prj_01hf7yat000000000000000000"} {
		if ValidPublicID(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
}
