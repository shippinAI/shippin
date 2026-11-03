package mail

import (
	"context"
	"time"

	"shippin.cloud/console/internal/db"
)

// Enqueuer is the slice of the database the notifier needs.
type Enqueuer interface {
	EnqueueMail(ctx context.Context, m db.OutMail) (bool, error)
}

// Notifier renders a notice once per recipient and queues it. A nil Notifier (mail disabled)
// queues nothing. Dedupe keys make repeated triggers (retries, sweeps) send at most once.
type Notifier struct {
	R      *Renderer
	Q      Enqueuer
	Brand  string
	Footer string
}

// Queue renders n for every recipient; sendAfter delays delivery (legal-hold notices).
func (nt *Notifier) Queue(ctx context.Context, n Notice, to []string, key string, sendAfter time.Time) (int, error) {
	if nt == nil {
		return 0, nil
	}
	if n.Vars == nil {
		n.Vars = map[string]string{}
	}
	n.Vars["Brand"] = nt.Brand
	if n.Footer == "" {
		n.Footer = nt.Footer
	}
	r, err := nt.R.Render(n)
	if err != nil {
		return 0, err
	}
	queued := 0
	seen := map[string]bool{}
	for _, addr := range to {
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		ok, err := nt.Q.EnqueueMail(ctx, db.OutMail{Kind: n.Kind, DedupeKey: n.Kind + "|" + key + "|" + addr, To: addr,
			Subject: r.Subject, Text: r.Text, HTML: r.HTML, SendAfter: sendAfter})
		if err != nil {
			return queued, err
		}
		if ok {
			queued++
		}
	}
	return queued, nil
}
