package config

import "testing"

func TestServersCheck(t *testing.T) {
	ok := Servers{Enabled: true, TokenTTLSeconds: 900, HeartbeatSeconds: 15, MissedBeats: 3, CredentialTTLSeconds: 300, MaxPerTenant: 10}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	if (Servers{}).Check() != nil {
		t.Fatal("disabled must pass")
	}
	for name, mut := range map[string]func(*Servers){
		"short token":   func(s *Servers) { s.TokenTTLSeconds = 5 },
		"no beat":       func(s *Servers) { s.HeartbeatSeconds = 0 },
		"no cap":        func(s *Servers) { s.MaxPerTenant = 0 },
		"cred <= stale": func(s *Servers) { s.CredentialTTLSeconds = 45 },
	} {
		s := ok
		mut(&s)
		if s.Check() == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
