-- Outgoing notice queue: rows are claimed with SKIP LOCKED, retried with backoff, never lost.
CREATE TABLE IF NOT EXISTS mail_outbox (
  id          bigserial PRIMARY KEY,
  kind        text NOT NULL,
  dedupe_key  text NOT NULL UNIQUE,
  to_addr     text NOT NULL,
  subject     text NOT NULL,
  body_text   text NOT NULL,
  body_html   text NOT NULL,
  status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
  attempts    int  NOT NULL DEFAULT 0,
  last_error  text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now(),
  send_after  timestamptz NOT NULL DEFAULT now(),
  locked_until timestamptz,
  sent_at     timestamptz
);
CREATE INDEX IF NOT EXISTS mail_outbox_due ON mail_outbox(send_after) WHERE status = 'pending';
