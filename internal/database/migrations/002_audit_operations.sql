-- Migration 002: High-level bulk audit operations tracking

CREATE TABLE IF NOT EXISTS audit_operations (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    operation   TEXT NOT NULL, -- 'convert', 'delete', 'sort', 'rename', 'restore', 'purge'
    source      TEXT NOT NULL, -- 'converter', 'dupfinder', 'organizer', 'trash'
    summary     TEXT NOT NULL,
    item_count  INT NOT NULL DEFAULT 1,
    total_size  BIGINT NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'completed', -- 'completed', 'partial', 'failed'
    metadata    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_ops_created ON audit_operations (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_ops_operation ON audit_operations (operation);
