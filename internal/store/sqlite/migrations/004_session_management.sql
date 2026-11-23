ALTER TABLE users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0;

ALTER TABLE sessions ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN last_seen_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN ip_address TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';

UPDATE sessions
SET created_at = strftime('%s', 'now'),
    last_seen_at = strftime('%s', 'now')
WHERE created_at = 0;

CREATE INDEX IF NOT EXISTS sessions_last_seen_idx ON sessions(last_seen_at DESC);
