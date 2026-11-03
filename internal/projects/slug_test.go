package projects

import (
	"context"
	"testing"

	"shippin.cloud/console/internal/config"
)

func TestSlugFromPrompt(t *testing.T) {
	for in, want := range map[string]string{
		"Build me a todo app with Vite + React":        "todo-app-vite-react",
		"a CRM for my dog-walking business, please!!!": "crm-dog-walking-business",
		"`; rm -rf ~ $(id)`":                           "rm-rf-id",
		"123 go":                                       "go",
		"日本語":                                          "project",
		"an extremely long description of a web app that keeps going": "extremely-long-description-web",
	} {
		if got := SlugFromPrompt(in); got != want || !nameShape.MatchString(got) {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestUniqueName(t *testing.T) {
	used := map[string]bool{"todo-app": true, "todo-app-2": true}
	n, err := UniqueName(context.Background(), "todo-app", func(_ context.Context, s string) (bool, error) { return used[s], nil })
	if err != nil || n != "todo-app-3" {
		t.Fatalf("%q %v", n, err)
	}
	long := "abcdefghij-abcdefghij-abcdefghij-abcdefghi"[:40]
	n, _ = UniqueName(context.Background(), long, func(_ context.Context, s string) (bool, error) { return s == long, nil })
	if len(n) > 40 || !nameShape.MatchString(n) {
		t.Fatalf("suffix overflow: %q", n)
	}
}

func TestGuessStack(t *testing.T) {
	stacks := []config.Stack{
		{Template: "node-vite", Label: "React", Keywords: []string{"react", "vite", "next.js", "web app"}},
		{Template: "python", Label: "Python", Keywords: []string{"python", "fastapi", "flask", "django"}},
		{Template: "go", Label: "Go", Keywords: []string{"go", "golang"}},
	}
	for in, want := range map[string]string{
		"a todo app with Vite + React":  "node-vite",
		"REST API in FastAPI":           "python",
		"a CLI in Golang":               "go",
		"let's go build a web app":      "node-vite",
		"a cargo tracker":               "blank",
		"I'm going to make a todo list": "blank",
	} {
		if got := GuessStack(in, stacks, "blank"); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
