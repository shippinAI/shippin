package config

import "testing"

func TestMailCheckFailsClosed(t *testing.T) {
	ok := Mail{Enabled: true, Host: "203.0.113.60", Port: 465, TLS: "implicit", From: "notices@shippin.cloud",
		Username: "notices@shippin.cloud", PasswordEnv: "SHIPPIN_SMTP_PASSWORD", MaxAttempts: 8, RetryBaseSeconds: 60,
		RetryMaxSeconds: 3600, PollSeconds: 10, BatchSize: 20, TimeoutSeconds: 20}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	if err := (Mail{}).Check(); err != nil {
		t.Fatal("disabled mail needs no settings")
	}
	for name, bad := range map[string]func(*Mail){
		"plaintext": func(m *Mail) { m.TLS = "none" }, "no host": func(m *Mail) { m.Host = "" },
		"from with name": func(m *Mail) { m.From = "Shippin <notices@shippin.cloud>" }, "no password env": func(m *Mail) { m.PasswordEnv = "" },
		"retry cap below base": func(m *Mail) { m.RetryMaxSeconds = 1 }, "no attempts": func(m *Mail) { m.MaxAttempts = 0 },
	} {
		m := ok
		bad(&m)
		if m.Check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
