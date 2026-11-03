package config

import "testing"

func TestRRFlowTenantsFromEnv(t *testing.T) {
	t.Setenv("SHIPPIN_RRFLOW_TENANTS", " 00000000-0000-4000-8000-000000000001, nope,,../x ")
	s := ServiceToggle{Enabled: true, Tenants: []string{"aaaaaaaa-0000-7000-8000-000000000001"}}
	s.loadEnv()
	if len(s.Tenants) != 2 || !s.Allows("00000000-0000-4000-8000-000000000001") || s.Allows("nope") {
		t.Fatalf("tenants %v", s.Tenants)
	}
}

func TestListedOnlyEmptyMeansNone(t *testing.T) {
	s := ServiceToggle{Enabled: true, ListedOnly: true}
	if s.Allows("00000000-0000-4000-8000-000000000001") {
		t.Fatal("empty listed_only rollout allowed a tenant")
	}
	t.Setenv("SHIPPIN_RRFLOW_TENANTS", "aaaaaaaa-0000-7000-8000-000000000001")
	s.loadEnv()
	if !s.Allows("aaaaaaaa-0000-7000-8000-000000000001") || s.Allows("00000000-0000-4000-8000-000000000001") {
		t.Fatal("env tenant not exclusive")
	}
}
