-- Console chat: server-backed sessions shared by the overlay and /chat. One owner (IdP sub) per
-- session, scoped to a tenant, optional project, and the workspace whose AI CLI answers.
CREATE TABLE IF NOT EXISTS chat_sessions (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id   uuid,
  workspace_id uuid NOT NULL,
  owner_sub    text NOT NULL,
  provider     text NOT NULL,
  title        text NOT NULL DEFAULT 'New chat' CHECK (length(title) <= 120),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS chat_sessions_owner ON chat_sessions(owner_sub, updated_at DESC);
CREATE TABLE IF NOT EXISTS chat_messages (
  id          uuid PRIMARY KEY,
  session_id  uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  role        text NOT NULL CHECK (role IN ('user','assistant','error')),
  content     text NOT NULL,
  ui          jsonb NOT NULL DEFAULT '[]',
  created_at  timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS chat_messages_session ON chat_messages(session_id, created_at);
