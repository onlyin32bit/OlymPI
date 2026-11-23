-- Sound buses: add the single-slot "timer" bus and reclassify built-in sounds
-- (legacy loudness: effects .5 : timer .4 : music .4 : question audio .6,
-- normalised so question audio = 1.0). Keep in sync with sound.Builtins().
CREATE TABLE sounds_new (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    bus TEXT NOT NULL CHECK (bus IN ('music', 'effects', 'timer')),
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

INSERT INTO sounds_new(id, key, name, category, sort_order, bus, volume, loop, fade_in_ms, fade_out_ms,
    builtin_path, asset_id, builtin, enabled, created_at, updated_at, archived_at)
SELECT id, key, name, category, sort_order, bus, volume, loop, fade_in_ms, fade_out_ms,
    builtin_path, asset_id, builtin, enabled, created_at, updated_at, archived_at
FROM sounds;

DROP TABLE sounds;
ALTER TABLE sounds_new RENAME TO sounds;

CREATE INDEX IF NOT EXISTS sounds_category_order_idx
    ON sounds(category, sort_order, name);

UPDATE sounds SET bus = 'effects', volume = 5.0 / 6.0, loop = 0
WHERE builtin = 1;

UPDATE sounds SET bus = 'timer', volume = 4.0 / 6.0, loop = 0
WHERE builtin = 1 AND builtin_path IN (
    'kd/3Seconds.m4a', 'tt/10Seconds.mp3', 'tt/20Seconds.mp3', 'tt/30Seconds.mp3', 'tt/40Seconds.mp3',
    'vcnv/15Seconds.mp3', 'bonus/Thinking.mp3', 'vd/5Seconds.mp3', 'vd/15Seconds.mp3', 'vd/20Seconds.mp3',
    'vd/Practice20Seconds.mp3', 'vd/Practice30Seconds.mp3', 'vd/Practice40Seconds.mp3', 'vd/Practice60Seconds.mp3'
);

UPDATE sounds SET bus = 'music', volume = 4.0 / 6.0, loop = 1
WHERE builtin = 1 AND builtin_path = 'kd/MainTime.mp3';
