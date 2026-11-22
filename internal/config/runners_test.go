package config

import "testing"

func TestRunnersCheck(t *testing.T) {
	ok := Runners{Enabled: true, Fleet: "01a14149-0000-7000-8000-0000000000f1", Storage: "fast-1", JobLimitMinutes: 360}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	if err := (Runners{}).Check(); err != nil {
		t.Fatalf("off needs nothing: %v", err)
	}
	for name, r := range map[string]Runners{
		"no fleet":      {Enabled: true, JobLimitMinutes: 360},
		"bad fleet":     {Enabled: true, Fleet: "runners", JobLimitMinutes: 360},
		"bad pool":      {Enabled: true, Fleet: ok.Fleet, Storage: "a pool", JobLimitMinutes: 360},
		"no time limit": {Enabled: true, Fleet: ok.Fleet},
		"too long":      {Enabled: true, Fleet: ok.Fleet, JobLimitMinutes: 20000},
	} {
		if r.Check() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	c := &Config{Runners: ok}
	if err := c.Check(); err == nil {
		t.Error("runners without jobs accepted")
	}
}
