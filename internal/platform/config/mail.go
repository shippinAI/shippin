package config

import (
	"errors"
	"net/mail"
	"os"
)

// Mail configures outgoing notices: SMTP submission to Shippin Mail plus the retry queue.
// Disabled means notices are not queued at all.
type Mail struct {
	Enabled    bool   `json:"enabled"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	TLS        string `json:"tls"` // "implicit" or "starttls"; plaintext is never offered
	ServerName string `json:"server_name"`
	CAFile     string `json:"ca_file"`
	From       string `json:"from"`
	FromName   string `json:"from_name"`
	Username   string `json:"username"`
	// PasswordEnv names the environment variable holding the SMTP password.
	PasswordEnv      string `json:"password_env"`
	MaxAttempts      int    `json:"max_attempts"`
	RetryBaseSeconds int    `json:"retry_base_seconds"`
	RetryMaxSeconds  int    `json:"retry_max_seconds"`
	PollSeconds      int    `json:"poll_seconds"`
	BatchSize        int    `json:"batch_size"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	Password         string `json:"-"`
}

func (m *Mail) load() {
	if m.PasswordEnv != "" {
		m.Password = os.Getenv(m.PasswordEnv)
	}
}

// Check fails closed when mail is enabled but incomplete.
func (m Mail) Check() error {
	if !m.Enabled {
		return nil
	}
	switch {
	case m.Host == "" || m.Port < 1 || m.Port > 65535:
		return errors.New("mail: host and port are required")
	case m.TLS != "implicit" && m.TLS != "starttls":
		return errors.New("mail: tls must be implicit or starttls")
	case m.Username == "" || m.PasswordEnv == "":
		return errors.New("mail: username and password_env are required")
	case m.MaxAttempts < 1 || m.RetryBaseSeconds < 1 || m.RetryMaxSeconds < m.RetryBaseSeconds:
		return errors.New("mail: max_attempts and retry_base_seconds <= retry_max_seconds are required")
	case m.PollSeconds < 1 || m.BatchSize < 1 || m.TimeoutSeconds < 1:
		return errors.New("mail: poll_seconds, batch_size and timeout_seconds are required")
	}
	if a, err := mail.ParseAddress(m.From); err != nil || a.Address != m.From {
		return errors.New("mail: from must be a bare address")
	}
	return nil
}
