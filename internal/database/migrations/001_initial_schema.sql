-- Morphic Initial Schema Migration

-- Track executed schema migrations
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INT PRIMARY KEY,
    name        TEXT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Perceptual hash cache for incremental DupFinder scans
CREATE TABLE IF NOT EXISTS media_hashes (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    path        TEXT NOT NULL,
    file_size   BIGINT NOT NULL,
    mod_time    TIMESTAMPTZ NOT NULL,
    phash       BIGINT,
    ahash       BIGINT,
    dhash       BIGINT,
    width       INT,
    height      INT,
    duration    DOUBLE PRECISION,
    format      TEXT,
    fps         DOUBLE PRECISION,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_media_hashes UNIQUE (path, file_size, mod_time)
);

CREATE INDEX IF NOT EXISTS idx_media_hashes_path ON media_hashes (path);
CREATE INDEX IF NOT EXISTS idx_media_hashes_phash ON media_hashes (phash);
CREATE INDEX IF NOT EXISTS idx_media_hashes_lookup ON media_hashes (path, file_size, mod_time);

-- Persistent background jobs (converter, dupfinder, organizer)
CREATE TABLE IF NOT EXISTS jobs (
    id          UUID PRIMARY KEY,
    type        TEXT NOT NULL, -- 'conversion', 'dupfinder', 'organizer'
    status      TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'running', 'done', 'failed', 'cancelled'
    progress    DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    message     TEXT,
    error       TEXT,
    payload     JSONB NOT NULL DEFAULT '{}',
    result      JSONB NOT NULL DEFAULT '{}',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    done_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_jobs_type ON jobs (type);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_updated ON jobs (updated_at DESC);

-- Non-destructive audit log & safe-trash record for 1-click undo
CREATE TABLE IF NOT EXISTS audit_log (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    operation        TEXT NOT NULL, -- 'delete', 'convert', 'rename', 'sort'
    original_path    TEXT NOT NULL,
    destination_path TEXT,
    trash_path       TEXT,
    file_size        BIGINT,
    metadata         JSONB NOT NULL DEFAULT '{}',
    reversible       BOOLEAN NOT NULL DEFAULT TRUE,
    reversed_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_operation ON audit_log (operation);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_reversible ON audit_log (reversible) WHERE reversed_at IS NULL;

-- Persistent thumbnail and frame cache
CREATE TABLE IF NOT EXISTS thumbnail_cache (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    path        TEXT NOT NULL,
    file_size   BIGINT NOT NULL,
    mod_time    TIMESTAMPTZ NOT NULL,
    thumb_size  INT NOT NULL DEFAULT 200,
    thumb_data  BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_thumbnail_cache UNIQUE (path, file_size, mod_time, thumb_size)
);

CREATE INDEX IF NOT EXISTS idx_thumbnail_cache_lookup ON thumbnail_cache (path, file_size, mod_time, thumb_size);

-- Automated watch folder configurations
CREATE TABLE IF NOT EXISTS watch_folders (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    path        TEXT NOT NULL UNIQUE,
    action      TEXT NOT NULL DEFAULT 'organize', -- 'organize', 'convert'
    config      JSONB NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    last_scan   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Conversion and organization user presets
CREATE TABLE IF NOT EXISTS presets (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL, -- 'converter', 'organizer'
    config      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
