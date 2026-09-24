package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HashRow represents a cached perceptual hash entry in PostgreSQL.
type HashRow struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"`
	FileSize  int64     `json:"file_size"`
	ModTime   time.Time `json:"mod_time"`
	PHash     uint64    `json:"phash"`
	AHash     uint64    `json:"ahash"`
	DHash     uint64    `json:"dhash"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Duration  float64   `json:"duration"`
	Format    string    `json:"format"`
	FPS       float64   `json:"fps"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LookupHash queries the database for an existing cached perceptual hash matching path, size, and mod_time.
func LookupHash(ctx context.Context, pool *pgxpool.Pool, path string, fileSize int64, modTime time.Time) (*HashRow, error) {
	if pool == nil {
		return nil, nil
	}

	query := `
		SELECT id, path, file_size, mod_time, phash, ahash, dhash, width, height, duration, format, fps, created_at, updated_at
		FROM media_hashes
		WHERE path = $1 AND file_size = $2 AND mod_time = $3
		LIMIT 1;
	`

	var row HashRow
	var phash, ahash, dhash *int64

	err := pool.QueryRow(ctx, query, path, fileSize, modTime).Scan(
		&row.ID,
		&row.Path,
		&row.FileSize,
		&row.ModTime,
		&phash,
		&ahash,
		&dhash,
		&row.Width,
		&row.Height,
		&row.Duration,
		&row.Format,
		&row.FPS,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("looking up media hash for %s: %w", path, err)
	}

	if phash != nil {
		row.PHash = uint64(*phash)
	}
	if ahash != nil {
		row.AHash = uint64(*ahash)
	}
	if dhash != nil {
		row.DHash = uint64(*dhash)
	}

	return &row, nil
}

// StoreHash upserts a computed media hash into the media_hashes table.
func StoreHash(ctx context.Context, pool *pgxpool.Pool, row HashRow) error {
	if pool == nil {
		return nil
	}

	query := `
		INSERT INTO media_hashes (path, file_size, mod_time, phash, ahash, dhash, width, height, duration, format, fps, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (path, file_size, mod_time) DO UPDATE SET
			phash = EXCLUDED.phash,
			ahash = EXCLUDED.ahash,
			dhash = EXCLUDED.dhash,
			width = EXCLUDED.width,
			height = EXCLUDED.height,
			duration = EXCLUDED.duration,
			format = EXCLUDED.format,
			fps = EXCLUDED.fps,
			updated_at = NOW();
	`

	_, err := pool.Exec(ctx, query,
		row.Path,
		row.FileSize,
		row.ModTime,
		int64(row.PHash),
		int64(row.AHash),
		int64(row.DHash),
		row.Width,
		row.Height,
		row.Duration,
		row.Format,
		row.FPS,
	)
	if err != nil {
		return fmt.Errorf("storing hash for %s: %w", row.Path, err)
	}
	return nil
}

// StoreHashesBatch inserts multiple hash rows in a single batch.
func StoreHashesBatch(ctx context.Context, pool *pgxpool.Pool, rows []HashRow) error {
	if pool == nil || len(rows) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO media_hashes (path, file_size, mod_time, phash, ahash, dhash, width, height, duration, format, fps, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (path, file_size, mod_time) DO UPDATE SET
			phash = EXCLUDED.phash,
			ahash = EXCLUDED.ahash,
			dhash = EXCLUDED.dhash,
			width = EXCLUDED.width,
			height = EXCLUDED.height,
			duration = EXCLUDED.duration,
			format = EXCLUDED.format,
			fps = EXCLUDED.fps,
			updated_at = NOW();
	`

	for _, r := range rows {
		batch.Queue(query,
			r.Path,
			r.FileSize,
			r.ModTime,
			int64(r.PHash),
			int64(r.AHash),
			int64(r.DHash),
			r.Width,
			r.Height,
			r.Duration,
			r.Format,
			r.FPS,
		)
	}

	br := pool.SendBatch(ctx, batch)
	defer br.Close()

	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("executing batch hash store: %w", err)
		}
	}
	return nil
}
