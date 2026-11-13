package runfleet

import "testing"

func TestRegistrationLevel(t *testing.T) {
	level, note, err := RegistrationLevel("user", "organization")
	if err != nil || level != "repository" || note == "" {
		t.Fatalf("level=%s note=%s err=%v", level, note, err)
	}
	level, _, err = RegistrationLevel("organization", "")
	if err != nil || level != "organization" {
		t.Fatalf("org default = %s %v", level, err)
	}
	level, _, err = RegistrationLevel("organization", "repository")
	if err != nil || level != "repository" {
		t.Fatalf("org per-repo = %s %v", level, err)
	}
	if _, _, err := RegistrationLevel("enterprise", ""); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApproval(t *testing.T) {
	auto, reason := Approval(RepoFact{Private: false, License: "MIT"}, true)
	if auto != "auto" || reason == "" {
		t.Fatalf("%s %s", auto, reason)
	}
	pending, _ := Approval(RepoFact{Private: true, License: "MIT"}, true)
	if pending != "pending" {
		t.Fatal(pending)
	}
	nolicense, _ := Approval(RepoFact{Private: false}, true)
	if nolicense != "pending" {
		t.Fatal(nolicense)
	}
	off, _ := Approval(RepoFact{License: "Apache-2.0"}, false)
	if off != "pending" {
		t.Fatal(off)
	}
}
