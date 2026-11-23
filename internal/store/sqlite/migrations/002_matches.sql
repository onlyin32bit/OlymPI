ALTER TABLE users ADD COLUMN fixed_slot INTEGER;
ALTER TABLE users ADD COLUMN password_change_required INTEGER NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS users_fixed_slot_idx ON users(fixed_slot) WHERE fixed_slot IS NOT NULL;

CREATE TABLE IF NOT EXISTS question_sets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT '',
    current_revision_id TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    archived_at INTEGER
);

CREATE TABLE IF NOT EXISTS question_set_revisions (
    id TEXT PRIMARY KEY,
    question_set_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    data_json TEXT NOT NULL,
    created_by INTEGER,
    created_at INTEGER NOT NULL,
    UNIQUE(question_set_id, revision),
    FOREIGN KEY (question_set_id) REFERENCES question_sets(id),
    FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS question_set_revisions_set_idx ON question_set_revisions(question_set_id, revision DESC);

CREATE TABLE IF NOT EXISTS matches (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    season INTEGER NOT NULL,
    stage TEXT NOT NULL,
    match_number INTEGER NOT NULL,
    status TEXT NOT NULL,
    question_set_id TEXT,
    question_set_revision_id TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    result_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    started_at INTEGER,
    completed_at INTEGER,
    archived_at INTEGER,
    FOREIGN KEY (question_set_id) REFERENCES question_sets(id),
    FOREIGN KEY (question_set_revision_id) REFERENCES question_set_revisions(id)
);

CREATE INDEX IF NOT EXISTS matches_status_idx ON matches(status, updated_at DESC);
CREATE INDEX IF NOT EXISTS matches_question_set_idx ON matches(question_set_id);

CREATE TABLE IF NOT EXISTS match_contestants (
    match_id TEXT NOT NULL,
    slot INTEGER NOT NULL CHECK(slot BETWEEN 1 AND 4),
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    class TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(match_id, slot),
    FOREIGN KEY (match_id) REFERENCES matches(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS app_state (
    singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
    selected_match_id TEXT,
    epoch INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (selected_match_id) REFERENCES matches(id)
);
INSERT OR IGNORE INTO app_state(singleton, selected_match_id, epoch) VALUES (1, NULL, 0);

CREATE TABLE IF NOT EXISTS match_events (
    match_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    actor_user_id INTEGER,
    client_id TEXT NOT NULL DEFAULT '',
    command_id TEXT,
    created_at INTEGER NOT NULL,
    PRIMARY KEY(match_id, sequence),
    UNIQUE(match_id, command_id),
    FOREIGN KEY (match_id) REFERENCES matches(id),
    FOREIGN KEY (actor_user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS match_snapshots (
    match_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    state_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY(match_id, sequence),
    FOREIGN KEY (match_id) REFERENCES matches(id)
);

CREATE TABLE IF NOT EXISTS result_amendments (
    id TEXT PRIMARY KEY,
    match_id TEXT NOT NULL,
    result_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    actor_user_id INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (match_id) REFERENCES matches(id),
    FOREIGN KEY (actor_user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id INTEGER,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    match_id TEXT,
    before_json TEXT,
    after_json TEXT,
    reason TEXT NOT NULL DEFAULT '',
    client_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (actor_user_id) REFERENCES users(id),
    FOREIGN KEY (match_id) REFERENCES matches(id)
);

CREATE INDEX IF NOT EXISTS audit_logs_created_idx ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_entity_idx ON audit_logs(entity_type, entity_id, created_at DESC);

CREATE TABLE IF NOT EXISTS assets (
    id TEXT PRIMARY KEY,
    sha256 TEXT NOT NULL UNIQUE,
    original_name TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    created_by INTEGER,
    created_at INTEGER NOT NULL,
    archived_at INTEGER,
    FOREIGN KEY (created_by) REFERENCES users(id)
);
