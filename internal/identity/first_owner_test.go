package identity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// Many people claiming at once: exactly one becomes the owner, the rest are told an owner exists.
func TestClaimInstallationFirstClaimWins(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make([]error, 6)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, results[i] = s.ClaimInstallation(ctx, FirstOwnerInput{Name: "Racer", Email: fmt.Sprintf("racer%d@example.invalid", i),
				Password: "a long test password"}, Claim{Via: "browser"})
		}(i)
	}
	wg.Wait()
	won := 0
	for _, err := range results {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrOwnerExists):
			t.Fatal("unexpected", err)
		}
	}
	var owners int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM identity_users WHERE role='owner'`).Scan(&owners); err != nil || won != 1 || owners != 1 {
		t.Fatalf("%d claims won and %d owners exist, want one of each (%v)", won, owners, err)
	}
}

// A claim with no username gets one from its email address, numbered when an existing account has it; a
// claim nobody watches issues no recovery codes, and one with no organization names it Operations.
func TestClaimInstallationNamesTheOwner(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	if _, err := s.db.Exec(ctx, `INSERT INTO identity_users (id, email, email_verified, name, role, username) VALUES (gen_random_uuid(), 'dev@example.invalid', true, 'Dev', 'member', 'ada.lovelace-x')`); err != nil {
		t.Fatal(err)
	}
	u, codes, err := s.ClaimInstallation(ctx, FirstOwnerInput{Name: "Ada", Email: "Ada.Lovelace+X@Example.invalid", Password: "a long test password"},
		Claim{Via: "environment"})
	if err != nil || u.Username != "ada.lovelace-x2" || codes != nil {
		t.Fatal(err, u.Username, codes)
	}
	var org string
	if err := s.db.QueryRow(ctx, `SELECT t.name FROM tenants t JOIN orgs o ON o.tenant_id=t.id JOIN identity_members m ON m.org_id=o.identity_org_id WHERE m.user_id=$1`, u.ID).Scan(&org); err != nil || org != DefaultOrganization {
		t.Fatal("organization", org, err)
	}
	if _, err := s.AuthenticatePassword(ctx, "ada.lovelace-x2", "a long test password", "test"); err != nil {
		t.Fatal("the owner cannot sign in", err)
	}
}

func TestUsernameFor(t *testing.T) {
	for in, want := range map[string]string{"owner@example.invalid": "owner", "Jo@x.invalid": "owner", "a.b_c-d@x.invalid": "a.b_c-d", "--x--y@x.invalid": "x--y"} {
		if got := usernameFor(in); got != want {
			t.Errorf("usernameFor(%q) = %q, want %q", in, got, want)
		}
	}
}
