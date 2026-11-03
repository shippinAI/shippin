package mail

import (
	"context"
	"log"
	"net/mail"
	"time"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

// Queue is the slice of the database the worker needs.
type Queue interface {
	ClaimMail(ctx context.Context, n int, lease time.Duration) ([]db.OutMail, error)
	MailSent(ctx context.Context, id int64) error
	MailFailed(ctx context.Context, id int64, msg string, retryAt time.Time, final bool) error
}

// Worker drains the outbox: due mails are claimed (leased), submitted, and either marked sent
// or rescheduled with exponential backoff; 5xx answers or max_attempts end in "failed".
type Worker struct {
	Q    Queue
	T    Transport
	Cfg  config.Mail
	Now  func() time.Time
	from mail.Address
}

func NewWorker(q Queue, t Transport, cfg config.Mail) *Worker {
	return &Worker{Q: q, T: t, Cfg: cfg, Now: time.Now, from: mail.Address{Name: cfg.FromName, Address: cfg.From}}
}

// Backoff is the delay before retry n (1-based): base * 2^(n-1), capped at max.
func (w *Worker) Backoff(n int) time.Duration {
	d := time.Duration(w.Cfg.RetryBaseSeconds) * time.Second
	for i := 1; i < n && d < time.Duration(w.Cfg.RetryMaxSeconds)*time.Second; i++ {
		d *= 2
	}
	return min(d, time.Duration(w.Cfg.RetryMaxSeconds)*time.Second)
}

func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(time.Duration(w.Cfg.PollSeconds) * time.Second)
	defer t.Stop()
	for {
		w.Process(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Process sends one batch; returns how many were sent and how many attempts failed.
func (w *Worker) Process(ctx context.Context) (sent, failed int) {
	lease := time.Duration(w.Cfg.TimeoutSeconds*3) * time.Second
	ms, err := w.Q.ClaimMail(ctx, w.Cfg.BatchSize, lease)
	if err != nil {
		log.Printf("mail: claim: %v", err)
		return 0, 0
	}
	for _, m := range ms {
		raw, err := Build(w.from, m.To, Rendered{Subject: m.Subject, Text: m.Text, HTML: m.HTML}, w.Now())
		if err == nil {
			err = w.T.Send(ctx, w.Cfg.From, m.To, raw)
		}
		if err == nil {
			if err := w.Q.MailSent(ctx, m.ID); err != nil {
				log.Printf("mail %d: sent but not marked: %v", m.ID, err)
			}
			sent++
			continue
		}
		failed++
		n := m.Attempts + 1
		final := n >= w.Cfg.MaxAttempts || Permanent(err)
		log.Printf("mail %d (%s) attempt %d failed (final=%v): %v", m.ID, m.Kind, n, final, err)
		if err := w.Q.MailFailed(ctx, m.ID, err.Error(), w.Now().Add(w.Backoff(n)), final); err != nil {
			log.Printf("mail %d: reschedule: %v", m.ID, err)
		}
	}
	return sent, failed
}
