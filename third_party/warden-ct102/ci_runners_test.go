package main

import "testing"

func TestComposeRunnerHealth(t *testing.T) {
	cases := []struct {
		name                       string
		github, substrate, hostLoad HealthState
		want                       HealthState
	}{
		{"all green", HealthOK, HealthOK, HealthOK, HealthOK},
		{"substrate dead is red", HealthOK, HealthCritical, HealthOK, HealthCritical},
		{"github offline is red", HealthCritical, HealthOK, HealthOK, HealthCritical},
		{"host pressure is amber", HealthOK, HealthOK, HealthWarn, HealthWarn},
		{"agent missing is amber", HealthOK, HealthWarn, HealthOK, HealthWarn},
		{"no token still green when substrate fine", HealthUnknown, HealthOK, HealthOK, HealthOK},
		{"nothing probeable is unknown", HealthUnknown, HealthUnknown, HealthUnknown, HealthUnknown},
		{"red beats amber", HealthWarn, HealthCritical, HealthOK, HealthCritical},
	}
	for _, c := range cases {
		if got := composeRunnerHealth(c.github, c.substrate, c.hostLoad); got != c.want {
			t.Errorf("%s: composeRunnerHealth(%s,%s,%s) = %s, want %s",
				c.name, c.github, c.substrate, c.hostLoad, got, c.want)
		}
	}
}

func TestDefaultCIRunners(t *testing.T) {
	runners := defaultCIRunners()
	if len(runners) != 2 {
		t.Fatalf("expected 2 default runners, got %d", len(runners))
	}
	for _, r := range runners {
		if r.Kind != "qemu" && r.Kind != "lxc" {
			t.Errorf("runner %s has invalid kind %q", r.ID, r.Kind)
		}
		if r.VMID < 100 || len(r.Repos) == 0 || r.Host == "" {
			t.Errorf("runner %s incomplete: vmid=%d repos=%d host=%q", r.ID, r.VMID, len(r.Repos), r.Host)
		}
	}
}
