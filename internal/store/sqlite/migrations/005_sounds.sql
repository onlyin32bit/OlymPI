CREATE TABLE IF NOT EXISTS sounds (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    bus TEXT NOT NULL CHECK (bus IN ('music', 'effects')),
    volume REAL NOT NULL DEFAULT 1 CHECK (volume >= 0 AND volume <= 1),
    loop INTEGER NOT NULL DEFAULT 0,
    fade_in_ms INTEGER NOT NULL DEFAULT 0 CHECK (fade_in_ms >= 0),
    fade_out_ms INTEGER NOT NULL DEFAULT 120 CHECK (fade_out_ms >= 0),
    builtin_path TEXT,
    asset_id TEXT,
    builtin INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    archived_at INTEGER,
    FOREIGN KEY (asset_id) REFERENCES assets(id),
    CHECK ((builtin_path IS NOT NULL) <> (asset_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS sounds_category_order_idx
    ON sounds(category, sort_order, name);
