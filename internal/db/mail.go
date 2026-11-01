package db

import (
	"context"
	"time"
)

// OutMail is one queued notice to one recipient.
type OutMail struct {
	ID        int64
	Kind      string
	DedupeKey string
	To        string
	Subject   string
	Text      string
	HTML      string
	Attempts  int
	SendAfter time.Time
}

// EnqueueMail queues m unless a mail with the same dedupe key exists; true when queued.
func (d *DB) EnqueueMail(ctx context.Context, m OutMail) (bool, error) {
	if m.SendAfter.IsZero() {
		m.SendAfter = time.Now()
	}
	tag, err := d.Pool.Exec(ctx, `INSERT INTO mail_outbox(kind, dedupe_key, to_addr, subject, body_text, body_html, send_after)
		VALUES($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (dedupe_key) DO NOTHING`,
		m.Kind, m.DedupeKey, m.To, m.Subject, m.Text, m.HTML, m.SendAfter)
	return err == nil && tag.RowsAffected() == 1, err
}

// ClaimMail locks up to n due mails for lease (so a crashed sender's rows come back later).
func (d *DB) ClaimMail(ctx context.Context, n int, lease time.Duration) ([]OutMail, error) {
	rows, err := d.Pool.Query(ctx, `UPDATE mail_outbox SET locked_until = now() + $2::int * interval '1 second'
		WHERE id IN (SELECT id FROM mail_outbox WHERE status = 'pending' AND send_after <= now()
			AND (locked_until IS NULL OR locked_until < now()) ORDER BY send_after, id LIMIT $1 FOR UPDATE SKIP LOCKED)
		RETURNING id, kind, dedupe_key, to_addr, subject, body_text, body_html, attempts, send_after`, n, int(lease.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutMail
	for rows.Next() {
		var m OutMail
		if err := rows.Scan(&m.ID, &m.Kind, &m.DedupeKey, &m.To, &m.Subject, &m.Text, &m.HTML, &m.Attempts, &m.SendAfter); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MailSent marks a mail delivered to the submission server.
func (d *DB) MailSent(ctx context.Context, id int64) error {
	_, err := d.Pool.Exec(ctx, `UPDATE mail_outbox SET status='sent', sent_at=now(), attempts=attempts+1,
		last_error='', locked_until=NULL WHERE id=$1`, id)
	return err
}

// MailFailed records a failed attempt: retried at retryAt, or given up for good when final.
func (d *DB) MailFailed(ctx context.Context, id int64, msg string, retryAt time.Time, final bool) error {
	status := "pending"
	if final {
		status = "failed"
	}
	_, err := d.Pool.Exec(ctx, `UPDATE mail_outbox SET status=$2, attempts=attempts+1, last_error=left($3, 500),
		send_after=$4, locked_until=NULL WHERE id=$1`, id, status, msg, retryAt)
	return err
}

// MailCounts reports the queue by status (for health/operator views and tests).
func (d *DB) MailCounts(ctx context.Context) (map[string]int, error) {
	rows, err := d.Pool.Query(ctx, `SELECT status, count(*) FROM mail_outbox GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{"pending": 0, "sent": 0, "failed": 0}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}
