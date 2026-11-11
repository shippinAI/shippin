-- What an owner has acknowledged about the built-in services during setup.
CREATE TABLE IF NOT EXISTS shippin_setup_steps (
    step            text PRIMARY KEY,
    acknowledged_by text NOT NULL,
    acknowledged_at timestamptz NOT NULL DEFAULT now()
);
