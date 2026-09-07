package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LookupThumbnail retrieves cached thumbnail bytes for path, size, and mod_time from PostgreSQL.
func LookupThumbnail(ctx context.Context, pool *pgxpool.Pool, path string, fileSize int64, modTime time.Time, thumbSize int) ([]byte, error) {
	if pool == nil {
		return nil, nil
	}

	query := `
		SELECT thumb_data
		FROM thumbnail_cache
		WHERE path = $1 AND file_size = $2 AND mod_time = $3 AND thumb_size = $4
		LIMIT 1;
	`

	var data []byte
	err := pool.QueryRow(ctx, query, path, fileSize, modTime, thumbSize).Scan(&data)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("looking up thumbnail for %s: %w", path, err)
	}
	return data, nil
}

// StoreThumbnail stores thumbnail bytes in PostgreSQL.
func StoreThumbnail(ctx context.Context, pool *pgxpool.Pool, path string, fileSize int64, modTime time.Time, thumbSize int, data []byte) error {
	if pool == nil || len(data) == 0 {
		return nil
	}

	query := `
		INSERT INTO thumbnail_cache (path, file_size, mod_time, thumb_size, thumb_data, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (path, file_size, mod_time, thumb_size) DO UPDATE SET
			thumb_data = EXCLUDED.thumb_data;
	`

	_, err := pool.Exec(ctx, query, path, fileSize, modTime, thumbSize, data)
	if err != nil {
		return fmt.Errorf("storing thumbnail for %s: %w", path, err)
	}
	return nil
}
