package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"strconv"
	"time"

	"shippin.cloud/console/internal/config"
)

// Transport delivers one raw message to one recipient.
type Transport interface {
	Send(ctx context.Context, from, to string, raw []byte) error
}

// SMTP submits over implicit TLS or STARTTLS with AUTH PLAIN; plaintext is never used.
type SMTP struct {
	cfg config.Mail
	tls *tls.Config
}

// NewSMTP builds the transport; CAFile pins the server certificate's issuer when set.
func NewSMTP(cfg config.Mail) (*SMTP, error) {
	tc := &tls.Config{ServerName: cfg.ServerName, MinVersion: tls.VersionTLS12}
	if tc.ServerName == "" {
		tc.ServerName = cfg.Host
	}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		tc.RootCAs = x509.NewCertPool()
		if !tc.RootCAs.AppendCertsFromPEM(pem) {
			return nil, errors.New("mail: no certificates in ca_file")
		}
	}
	return &SMTP{cfg: cfg, tls: tc}, nil
}

// Permanent reports whether err is an SMTP 5xx (retrying won't help).
func Permanent(err error) bool {
	var te *textproto.Error
	return errors.As(err, &te) && te.Code >= 500
}

func (s *SMTP) Send(ctx context.Context, from, to string, raw []byte) error {
	timeout := time.Duration(s.cfg.TimeoutSeconds) * time.Second
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	d := &net.Dialer{Timeout: timeout}
	var conn net.Conn
	var err error
	if s.cfg.TLS == "implicit" {
		conn, err = (&tls.Dialer{NetDialer: d, Config: s.tls}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	c, err := smtp.NewClient(conn, s.tls.ServerName)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if s.cfg.TLS == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("mail: server does not offer STARTTLS")
		}
		if err := c.StartTLS(s.tls); err != nil {
			return err
		}
	}
	if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.tls.ServerName)); err != nil {
		return fmt.Errorf("mail auth: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
