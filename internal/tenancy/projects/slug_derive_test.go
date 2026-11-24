package projects

import (
	"context"
	"testing"
)

func TestSlugFromSource(t *testing.T) {
	for in, want := range map[string]string{
		"eonsofstupid/box-work":                      "box-work",
		"eonsofstupid/My_Repo.git":                   "my-repo",
		"/home/dev/code/My App/":                     "my-app",
		"/home/dev/2048":                             "p-2048",
		"/home/dev/core":                             "core-app",
		"/home/dev/x":                                "x-app",
		"/home/dev/a--b":                             "a-b",
		"org/a-very-long-repository-name-that-keeps": "a-very-long-repository-name-th",
		"/home/dev/___":                              "project",
	} {
		got := SlugFromSource(in)
		if got != want || ValidSlug(got) != nil {
			t.Errorf("SlugFromSource(%q) = %q (valid: %v), want %q", in, got, ValidSlug(got), want)
		}
	}
}

func TestUniqueSlug(t *testing.T) {
	used := map[string]bool{"box-work": true, "box-work-2": true}
	got, err := UniqueSlug(context.Background(), "box-work", func(_ context.Context, s string) (bool, error) { return used[s], nil })
	if err != nil || got != "box-work-3" {
		t.Fatalf("got %q, %v", got, err)
	}
	long := "x23456789012345678901234567890"
	got, _ = UniqueSlug(context.Background(), long, func(_ context.Context, s string) (bool, error) { return s == long, nil })
	if got != "x234567890123456789012345678-2" || ValidSlug(got) != nil {
		t.Fatalf("long: %q", got)
	}
}
