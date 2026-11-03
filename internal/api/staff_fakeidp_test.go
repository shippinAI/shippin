package api

import (
	"context"
	"slices"
	"sync"
)

// fakeIdP is an in-memory staffIdP: users of the operator org, grants, MFA and project roles.
// calls records every write, so tests can prove "audit before write" and "no write on refusal".
type fakeIdP struct {
	mu     sync.Mutex
	users  map[string]*staffUser
	grants map[string]staffGrant
	mfa    map[string]bool
	keys   []string
	calls  []string
	fail   bool
}

func newFakeIdP() *fakeIdP {
	return &fakeIdP{users: map[string]*staffUser{}, grants: map[string]staffGrant{}, mfa: map[string]bool{}}
}

// add puts an active human with MFA and the given roles into the operator org.
func (f *fakeIdP) add(id string, roles ...string) *fakeIdP {
	f.users[id] = &staffUser{ID: id, Name: "User " + id, Email: id + "@example.test", State: "active", Kind: "human"}
	f.mfa[id] = true
	if len(roles) > 0 {
		f.grants[id] = staffGrant{ID: "g-" + id, Roles: roles}
	}
	return f
}

func (f *fakeIdP) Grant(_ context.Context, u string) (staffGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.grants[u], nil
}
func (f *fakeIdP) AllGrants(context.Context) (map[string]staffGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := map[string]staffGrant{}
	for k, v := range f.grants {
		m[k] = v
	}
	return m, nil
}
func (f *fakeIdP) MFA(_ context.Context, u string) (bool, error) { return f.mfa[u], nil }
func (f *fakeIdP) Users(context.Context) ([]staffUser, error) {
	out := []staffUser{}
	for _, u := range f.users {
		out = append(out, *u)
	}
	return out, nil
}
func (f *fakeIdP) User(_ context.Context, id string) (*staffUser, error) {
	if u, ok := f.users[id]; ok {
		v := *u
		return &v, nil
	}
	return nil, nil
}
func (f *fakeIdP) write(op string) error {
	f.calls = append(f.calls, op)
	if f.fail {
		return errFake
	}
	return nil
}
func (f *fakeIdP) Invite(_ context.Context, email, given, family string) (string, error) {
	if err := f.write("invite " + email); err != nil {
		return "", err
	}
	id := "new-" + given
	f.users[id] = &staffUser{ID: id, Name: given + " " + family, Email: email, State: "initial", Kind: "human"}
	return id, nil
}
func (f *fakeIdP) SetRoles(_ context.Context, u string, _ staffGrant, roles []string) error {
	if err := f.write("roles " + u); err != nil {
		return err
	}
	f.mu.Lock()
	f.grants[u] = staffGrant{ID: "g-" + u, Roles: slices.Clone(roles)}
	f.mu.Unlock()
	return nil
}
func (f *fakeIdP) SetActive(_ context.Context, u string, active bool) error {
	if err := f.write("active " + u); err != nil {
		return err
	}
	f.users[u].State = map[bool]string{true: "active", false: "inactive"}[active]
	return nil
}
func (f *fakeIdP) RoleKeys(context.Context) ([]string, error) { return f.keys, nil }
func (f *fakeIdP) PutRole(_ context.Context, key, _ string, _ bool) error {
	return f.write("role " + key)
}
func (f *fakeIdP) DeleteRole(_ context.Context, key string) error { return f.write("delrole " + key) }
