package runfleet

import "testing"

func TestCompose(t *testing.T) {
	cases := []struct {
		name                        string
		github, substrate, hostLoad HealthState
		want                        HealthState
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
		t.Run(c.name, func(t *testing.T) {
			if got := Compose(c.github, c.substrate, c.hostLoad); got != c.want {
				t.Fatalf("Compose() = %s, want %s", got, c.want)
			}
		})
	}
}
