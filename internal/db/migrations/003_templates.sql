CREATE TABLE IF NOT EXISTS templates (
  id          uuid PRIMARY KEY,
  slug        text NOT NULL UNIQUE,
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  icon        text NOT NULL DEFAULT 'box',
  image       text NOT NULL,
  toolchain   text[] NOT NULL DEFAULT '{}',
  cpu         int  NOT NULL DEFAULT 2,
  memory_gib  int  NOT NULL DEFAULT 4,
  disk_gib    int  NOT NULL DEFAULT 20,
  repo        text NOT NULL DEFAULT '',
  startup     text NOT NULL DEFAULT '',
  ports       int[] NOT NULL DEFAULT '{}',
  enabled     boolean NOT NULL DEFAULT true,
  sort        int NOT NULL DEFAULT 100,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS template_id uuid REFERENCES templates(id) ON DELETE SET NULL;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS repo text NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS size jsonb NOT NULL DEFAULT '{}';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS progress text NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS previews jsonb NOT NULL DEFAULT '[]';
