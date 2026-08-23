-- Demo schema for docker-compose.mcp-demo.yml (optional init).
-- Use a read-only Postgres role in production; this stack is for local trials only.

CREATE TABLE IF NOT EXISTS tickets (
  id         SERIAL PRIMARY KEY,
  title      TEXT        NOT NULL,
  status     TEXT        NOT NULL DEFAULT 'open',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO tickets (title, status) VALUES
  ('VPN outage',  'open'),
  ('Printer jam', 'closed');
