-- Desired Shippin-to-network associations are independent from provider observations.
-- Missing inventory rows therefore do not erase an operator's intended association.
CREATE TABLE IF NOT EXISTS shippin_network_resource_attachments (
  connection_id uuid NOT NULL REFERENCES shippin_network_connections(id) ON DELETE CASCADE,
  external_id text NOT NULL CHECK (length(external_id) BETWEEN 1 AND 512),
  server_id uuid NOT NULL REFERENCES servers(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (connection_id, external_id)
);
CREATE INDEX IF NOT EXISTS shippin_network_resource_attachments_server
  ON shippin_network_resource_attachments(server_id);
