// Package mail renders console notices and delivers them through SMTP submission with a
// Postgres-backed retry queue (see Worker).
package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
)

// Rendered is a notice ready to send.
type Rendered struct {
	Subject string
	Text    string
	HTML    string
}

// Build makes an RFC 5322 multipart/alternative message (text first, then HTML).
func Build(from mail.Address, to string, r Rendered, now time.Time) ([]byte, error) {
	if _, err := mail.ParseAddress(to); err != nil || strings.ContainsAny(to, "\r\n") {
		return nil, fmt.Errorf("invalid recipient")
	}
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	boundary := "shippin-" + hex.EncodeToString(id)
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]
	var b bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	h("From", from.String())
	h("To", to)
	h("Subject", mime.QEncoding.Encode("utf-8", strings.ReplaceAll(r.Subject, "\n", " ")))
	h("Date", now.UTC().Format(time.RFC1123Z))
	h("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	h("MIME-Version", "1.0")
	h("Auto-Submitted", "auto-generated")
	h("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	for _, p := range []struct{ typ, body string }{{"text/plain", r.Text}, {"text/html", r.HTML}} {
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", boundary, p.typ)
		w := quotedprintable.NewWriter(&b)
		if _, err := w.Write([]byte(strings.ReplaceAll(p.body, "\r\n", "\n"))); err != nil {
			return nil, err
		}
		_ = w.Close()
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes(), nil
}
