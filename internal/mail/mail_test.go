package mail

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

func cfgFor(f *fakeSMTP) config.Mail {
	host, port, _ := net.SplitHostPort(f.addr)
	p, _ := strconv.Atoi(port)
	tlsMode := "implicit"
	if f.startTLS {
		tlsMode = "starttls"
	}
	return config.Mail{Enabled: true, Host: host, Port: p, TLS: tlsMode, ServerName: "mail.test", CAFile: f.caFile,
		From: "notices@test", FromName: "Shippin", Username: f.user, Password: f.pass, MaxAttempts: 3,
		RetryBaseSeconds: 60, RetryMaxSeconds: 600, PollSeconds: 1, BatchSize: 10, TimeoutSeconds: 5}
}

// memQueue is an in-memory outbox with the same semantics as the Postgres one.
type memQueue struct {
	rows   map[int64]*db.OutMail
	status map[int64]string
	next   int64
}

func newMemQueue() *memQueue {
	return &memQueue{rows: map[int64]*db.OutMail{}, status: map[int64]string{}}
}

func (q *memQueue) EnqueueMail(_ context.Context, m db.OutMail) (bool, error) {
	for _, r := range q.rows {
		if r.DedupeKey == m.DedupeKey {
			return false, nil
		}
	}
	q.next++
	m.ID = q.next
	q.rows[m.ID], q.status[m.ID] = &m, "pending"
	return true, nil
}
func (q *memQueue) ClaimMail(_ context.Context, n int, _ time.Duration) ([]db.OutMail, error) {
	var out []db.OutMail
	for id, r := range q.rows {
		if q.status[id] == "pending" && !r.SendAfter.After(time.Now()) && len(out) < n {
			out = append(out, *r)
		}
	}
	return out, nil
}
func (q *memQueue) MailSent(_ context.Context, id int64) error {
	q.status[id] = "sent"
	q.rows[id].Attempts++
	return nil
}
func (q *memQueue) MailFailed(_ context.Context, id int64, _ string, at time.Time, final bool) error {
	q.rows[id].Attempts++
	q.rows[id].SendAfter = at
	if final {
		q.status[id] = "failed"
	}
	return nil
}

func TestSMTPSubmitsOverTLSOnly(t *testing.T) {
	for _, startTLS := range []bool{false, true} {
		f := newFakeSMTP(t, startTLS)
		s, err := NewSMTP(cfgFor(f))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Send(context.Background(), "notices@test", "owner@example.com", []byte("Subject: x\r\n\r\nhello\r\n")); err != nil {
			t.Fatalf("startTLS=%v: %v", startTLS, err)
		}
		if m := f.messages(); len(m) != 1 || !strings.Contains(m[0], "hello") || f.authOK != 1 {
			t.Fatalf("startTLS=%v: got %d messages, %d auths", startTLS, len(m), f.authOK)
		}
	}
}

func TestSMTPRefusesUntrustedServer(t *testing.T) {
	f, other := newFakeSMTP(t, false), newFakeSMTP(t, false)
	c := cfgFor(f)
	c.CAFile = other.caFile
	s, _ := NewSMTP(c)
	if err := s.Send(context.Background(), "notices@test", "a@example.com", []byte("x")); err == nil {
		t.Fatal("a server signed by another CA must be refused")
	}
	if len(f.messages()) != 0 {
		t.Fatal("nothing may be sent to an untrusted server")
	}
}

func TestWorkerRetriesThenSends(t *testing.T) {
	f := newFakeSMTP(t, false)
	c := cfgFor(f)
	s, _ := NewSMTP(c)
	q := newMemQueue()
	w := NewWorker(q, s, c)
	nt := &Notifier{R: mustRenderer(t), Q: q, Brand: "Shippin", Footer: "footer"}
	notice := Notice{Kind: "workspace_ready", Vars: map[string]string{"Workspace": "api", "Tenant": "Acme"}, URL: "https://c.test/w"}
	if n, _ := nt.Queue(context.Background(), notice, []string{"dev@example.com", "dev@example.com"}, "ws1", time.Time{}); n != 1 {
		t.Fatalf("queued %d, want 1 (duplicates collapse)", n)
	}
	if n, _ := nt.Queue(context.Background(), notice, []string{"dev@example.com"}, "ws1", time.Time{}); n != 0 {
		t.Fatal("the same notice must not be queued twice")
	}
	f.rcptCode = "451 try later"
	if sent, failed := w.Process(context.Background()); sent != 0 || failed != 1 {
		t.Fatalf("4xx: sent %d failed %d", sent, failed)
	}
	if q.status[1] != "pending" || time.Until(q.rows[1].SendAfter) < 50*time.Second {
		t.Fatalf("4xx must reschedule with backoff: %s %v", q.status[1], q.rows[1].SendAfter)
	}
	q.rows[1].SendAfter = time.Now()
	f.rcptCode = "250 OK"
	if sent, _ := w.Process(context.Background()); sent != 1 || q.status[1] != "sent" {
		t.Fatal("retry must send")
	}
	raw := f.messages()[0]
	for _, want := range []string{"Subject: Your workspace api is ready", "From: \"Shippin\" <notices@test>", "To: dev@example.com",
		"Auto-Submitted: auto-generated", "multipart/alternative", "text/html"} {
		if !strings.Contains(raw, want) {
			t.Errorf("message lacks %q", want)
		}
	}
}

func TestWorkerGivesUp(t *testing.T) {
	f := newFakeSMTP(t, false)
	c := cfgFor(f)
	s, _ := NewSMTP(c)
	q := newMemQueue()
	w := NewWorker(q, s, c)
	q.EnqueueMail(context.Background(), db.OutMail{Kind: "k", DedupeKey: "a", To: "a@example.com", Subject: "s"})
	f.rcptCode = "550 no such user"
	w.Process(context.Background())
	if q.status[1] != "failed" {
		t.Fatal("5xx must fail at once")
	}
	q.EnqueueMail(context.Background(), db.OutMail{Kind: "k", DedupeKey: "b", To: "b@example.com", Subject: "s"})
	f.rcptCode = "451 later"
	for i := 0; i < c.MaxAttempts; i++ {
		q.rows[2].SendAfter = time.Now()
		w.Process(context.Background())
	}
	if q.status[2] != "failed" || q.rows[2].Attempts != c.MaxAttempts {
		t.Fatalf("max attempts: %s after %d", q.status[2], q.rows[2].Attempts)
	}
	if b := w.Backoff(10); b != 600*time.Second {
		t.Fatalf("backoff cap: %v", b)
	}
}
