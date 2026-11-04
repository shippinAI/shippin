CREATE TABLE shippin_agenthub_connection (
  id smallint PRIMARY KEY CHECK (id = 1),
  base_url text NOT NULL CHECK (length(base_url) BETWEEN 1 AND 2048),
  token_ciphertext bytea NOT NULL DEFAULT ''::bytea,
  updated_by text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
