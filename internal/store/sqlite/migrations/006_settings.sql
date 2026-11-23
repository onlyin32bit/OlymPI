CREATE TABLE IF NOT EXISTS site_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    values_json TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
