package api

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/db"
)

func localStaffFor(t *testing.T, d *db.DB) *localStaff {
	t.Helper()
	return &localStaff{db: d}
}

func addLocalUser(t *testing.T, d *db.DB, role string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := d.Pool.Exec(context.Background(), `INSERT INTO identity_users (id, email, email_verified, name, role) VALUES ($1, $2, true, 'Someone', $3)`, id, id+"@example.invalid", role); err != nil {
		t.Fatal(err)
	}
	return localSubPrefix + id
}

func TestLocalStaffKeepsRolesAndOnlyTheOwnerIsAutomatic(t *testing.T) {
	_, d, _, _ := serverTestAPI(t, "org_owner")
	ctx := context.Background()
	s := localStaffFor(t, d)
	owner := addLocalUser(t, d, "owner")
	member := addLocalUser(t, d, "member")
	if g, err := s.Grant(ctx, owner); err != nil || !containsString(g.Roles, ownerRoleKey) {
		t.Fatalf("the owner holds the Owner role: %+v %v", g, err)
	}
	if g, err := s.Grant(ctx, member); err != nil || len(g.Roles) != 0 {
		t.Fatalf("nobody else holds anything until given it: %+v %v", g, err)
	}
	if err := s.SetRoles(ctx, member, staffGrant{}, []string{"staff.infra"}); err != nil {
		t.Fatal(err)
	}
	if g, _ := s.Grant(ctx, member); len(g.Roles) != 1 || g.Roles[0] != "staff.infra" {
		t.Fatalf("%+v", g)
	}
	all, err := s.AllGrants(ctx)
	if err != nil || len(all[member].Roles) != 1 || !containsString(all[owner].Roles, ownerRoleKey) {
		t.Fatalf("%+v %v", all, err)
	}
	if g, err := s.Grant(ctx, "someone-at-an-idp"); err != nil || len(g.Roles) != 0 {
		t.Fatalf("a subject from elsewhere has nothing here: %+v %v", g, err)
	}
	if err := s.SetActive(ctx, member, false); err != nil {
		t.Fatal(err)
	}
	if g, _ := s.Grant(ctx, member); len(g.Roles) != 0 {
		t.Fatalf("a deactivated account holds nothing: %+v", g)
	}
	if err := s.SetActive(ctx, owner, false); err != nil {
		t.Fatal(err)
	}
	if g, _ := s.Grant(ctx, owner); !containsString(g.Roles, ownerRoleKey) {
		t.Fatal("the owner cannot be switched off")
	}
}

func TestLocalStaffSecondFactor(t *testing.T) {
	_, d, _, _ := serverTestAPI(t, "org_owner")
	ctx := context.Background()
	optional := false
	s := &localStaff{db: d, noSecondFactor: func(context.Context) bool { return optional }}
	owner, member, linked, keyed := addLocalUser(t, d, "owner"), addLocalUser(t, d, "member"), addLocalUser(t, d, "member"), addLocalUser(t, d, "member")
	linkedID, _ := localID(linked)
	keyedID, _ := localID(keyed)
	if _, err := d.Pool.Exec(ctx, `INSERT INTO identity_links (issuer, sub, user_id) VALUES ('https://id.invalid', 's', $1)`, linkedID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO identity_passkeys (id, user_id, credential_id, public_key) VALUES ($1, $2, '\x01', '\x02')`, uuid.NewString(), keyedID); err != nil {
		t.Fatal(err)
	}
	for who, want := range map[string]bool{owner: true, member: false, linked: true, keyed: true, "someone-at-an-idp": false} {
		if got, err := s.MFA(ctx, who); err != nil || got != want {
			t.Errorf("%s: %v %v", who, got, err)
		}
	}
	optional = true
	if got, _ := s.MFA(ctx, member); !got {
		t.Error("the team can turn this off")
	}
}

func TestLocalStaffInviteMakesAnAccountOnTheOwnersBoard(t *testing.T) {
	_, d, _, _ := serverTestAPI(t, "org_owner")
	ctx := context.Background()
	s := localStaffFor(t, d)
	owner := addLocalUser(t, d, "owner")
	ownerID, _ := localID(owner)
	org := uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO identity_orgs (id, name) VALUES ($1, 'Owner org')`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO identity_members (org_id, user_id, role) VALUES ($1, $2, 'owner')`, org, ownerID); err != nil {
		t.Fatal(err)
	}
	sub, err := s.Invite(ctx, "Grace@Example.invalid", "Grace", "Hopper")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.User(ctx, sub)
	if err != nil || u == nil || u.Email != "grace@example.invalid" || u.Name != "Grace Hopper" {
		t.Fatalf("%+v %v", u, err)
	}
	if claimed, _ := s.Claimed(ctx, sub); claimed {
		t.Fatal("nobody has signed in to it yet")
	}
	id, _ := localID(sub)
	var got string
	if err := d.Pool.QueryRow(ctx, `SELECT org_id::text FROM identity_members WHERE user_id = $1`, id).Scan(&got); err != nil || got != org {
		t.Fatalf("what they ask for must reach the owner's board: %q %v", got, err)
	}
	if _, err := s.Invite(ctx, "grace@example.invalid", "Other", "Person"); err == nil {
		t.Fatal("the same email twice must be refused")
	}
}
