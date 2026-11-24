package catalog

import "testing"

func TestServiceItemsPlanBoundsTenant(t *testing.T) {
	svcs := []Service{
		{ID: "vaultix", Label: "Vaultix", Required: true, Active: true, DefaultConfig: raw(`{"projects":1}`)},
		{ID: "chatmaton", Label: "Chatmaton", Active: true, DefaultConfig: raw(`{"seats":5}`)},
		{ID: "dns", Label: "DNS", Active: true, DefaultConfig: raw(`{}`)},
	}
	cells := []PlanService{
		{PlanID: "pro", ServiceID: "vaultix", Enabled: true, Lock: LockBounded, Limits: raw(`{"projects":5}`)},
		{PlanID: "pro", ServiceID: "chatmaton", Enabled: true, Lock: LockBounded, Limits: raw(`{"seats":10}`)},
		{PlanID: "team", ServiceID: "dns", Enabled: true, Lock: LockBounded, Limits: raw(`{}`)},
	}
	no, yes := false, true
	ts := []TenantService{
		{TenantID: "t1", ServiceID: "vaultix", Enabled: &no, Overrides: raw(`{"projects":50}`)},
		{TenantID: "t1", ServiceID: "chatmaton", Overrides: raw(`{"seats":7}`)},
		{TenantID: "t1", ServiceID: "dns", Enabled: &yes},
	}
	es := Merge(ServiceItems("pro", svcs, cells, ts))
	cases := map[string]string{
		"svc.vaultix.enabled":   "true", // required: locked at system
		"svc.vaultix.projects":  "5",    // clamped to plan max
		"svc.chatmaton.seats":   "7",    // inside bounds
		"svc.chatmaton.enabled": "true",
		"svc.dns.enabled":       "false", // not in plan pro: locked off
	}
	for k, want := range cases {
		if got := string(get(t, es, k).Value); got != want {
			t.Errorf("%s = %s, want %s", k, got, want)
		}
	}
	if get(t, es, "svc.dns.enabled").Source != "plan" {
		t.Error("dns should be decided by the plan")
	}
}
