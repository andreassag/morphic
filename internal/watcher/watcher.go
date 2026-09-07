package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/exterex/morphic/internal/converter"
	"github.com/exterex/morphic/internal/organizer"
	"github.com/exterex/morphic/internal/shared"
	"github.com/fsnotify/fsnotify"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WatchFolder represents a configured directory watcher in PostgreSQL.
type WatchFolder struct {
	ID        int64           `json:"id"`
	Path      string          `json:"path"`
	Action    string          `json:"action"` // "organize" | "convert"
	Config    json.RawMessage `json:"config"`
	Enabled   bool            `json:"enabled"`
	LastScan  *time.Time      `json:"last_scan,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Daemon coordinates filesystem event monitoring for active watch folders.
type Daemon struct {
	pool       *pgxpool.Pool
	watcher    *fsnotify.Watcher
	mu         sync.Mutex
	watched    map[string]WatchFolder
	debounceMu sync.Mutex
	debounce   map[string]*time.Timer
}

var globalDaemon *Daemon

// GetPollInterval returns the fallback polling interval in seconds.
func GetPollInterval() time.Duration {
	if s := os.Getenv("WATCH_POLL_INTERVAL_SECS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 30 * time.Second
}

// StartDaemon initializes and starts the watch folder background daemon.
func StartDaemon(ctx context.Context, pool *pgxpool.Pool) (*Daemon, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	d := &Daemon{
		pool:     pool,
		watcher:  fw,
		watched:  make(map[string]WatchFolder),
		debounce: make(map[string]*time.Timer),
	}
	globalDaemon = d

	// Load initial watch folders from database
	if err := d.reload(ctx); err != nil {
		slog.Warn("watcher: failed to load initial watch folders", "err", err)
	}

	go d.eventLoop(ctx)
	go d.pollLoop(ctx)

	return d, nil
}

func (d *Daemon) reload(ctx context.Context) error {
	if d.pool == nil {
		return nil
	}

	folders, err := ListWatchFolders(ctx, d.pool)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	activePaths := make(map[string]bool)
	for _, f := range folders {
		if !f.Enabled {
			continue
		}
		activePaths[f.Path] = true
		d.watched[f.Path] = f
		_ = d.watcher.Add(f.Path)
	}

	// Remove unmanaged paths
	for p := range d.watched {
		if !activePaths[p] {
			_ = d.watcher.Remove(p)
			delete(d.watched, p)
		}
	}

	return nil
}

func (d *Daemon) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			_ = d.watcher.Close()
			return
		case event, ok := <-d.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
				d.handlePathEvent(ctx, event.Name)
			}
		case err, ok := <-d.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("watcher error", "err", err)
		}
	}
}

func (d *Daemon) handlePathEvent(ctx context.Context, filePath string) {
	ext := shared.NormaliseExt(filepath.Ext(filePath))
	if !shared.IsImage(filePath) && !shared.IsVideo(filePath) {
		return
	}

	d.debounceMu.Lock()
	if timer, ok := d.debounce[filePath]; ok {
		timer.Stop()
	}

	d.debounce[filePath] = time.AfterFunc(2*time.Second, func() {
		d.debounceMu.Lock()
		delete(d.debounce, filePath)
		d.debounceMu.Unlock()

		d.processFile(context.Background(), filePath, ext)
	})
	d.debounceMu.Unlock()
}

func (d *Daemon) processFile(ctx context.Context, filePath string, ext string) {
	dir := filepath.Dir(filePath)

	d.mu.Lock()
	wf, exists := d.watched[dir]
	d.mu.Unlock()

	if !exists || !wf.Enabled {
		return
	}

	// Verify file is settled and readable
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return
	}

	slog.Info("watcher processing new file", "path", filePath, "action", wf.Action)

	switch wf.Action {
	case "organize":
		template := "{year}/{month}"
		var cfg struct {
			Template string `json:"template"`
		}
		if len(wf.Config) > 0 {
			_ = json.Unmarshal(wf.Config, &cfg)
			if cfg.Template != "" {
				template = cfg.Template
			}
		}

		plan := organizer.PlanSort([]string{filePath}, template, dir)
		if len(plan) > 0 && plan[0].Status != "conflict" {
			organizer.ExecuteSort(ctx, plan, "move")
			slog.Info("watcher auto-organized file", "from", filePath, "to", plan[0].Destination)
		}

	case "convert":
		var cfg struct {
			TargetExt string `json:"target_ext"`
			Codec     string `json:"codec"`
		}
		if len(wf.Config) > 0 {
			_ = json.Unmarshal(wf.Config, &cfg)
		}
		if cfg.TargetExt == "" {
			if shared.IsImage(filePath) {
				cfg.TargetExt = ".webp"
			} else {
				cfg.TargetExt = ".mp4"
				cfg.Codec = "h264"
			}
		}

		dest, err := converter.ConvertFile(ctx, filePath, cfg.TargetExt, cfg.Codec, "", "", 0)
		if err == nil {
			slog.Info("watcher auto-converted file", "from", filePath, "to", dest)
		}
	}
}

func (d *Daemon) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(GetPollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = d.reload(ctx)
		}
	}
}

// ListWatchFolders queries all configured watch folders from PostgreSQL.
func ListWatchFolders(ctx context.Context, pool *pgxpool.Pool) ([]WatchFolder, error) {
	if pool == nil {
		return nil, nil
	}

	query := `
		SELECT id, path, action, config, enabled, last_scan, created_at, updated_at
		FROM watch_folders
		ORDER BY created_at DESC;
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying watch folders: %w", err)
	}
	defer rows.Close()

	var folders []WatchFolder
	for rows.Next() {
		var wf WatchFolder
		if err := rows.Scan(
			&wf.ID,
			&wf.Path,
			&wf.Action,
			&wf.Config,
			&wf.Enabled,
			&wf.LastScan,
			&wf.CreatedAt,
			&wf.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning watch folder: %w", err)
		}
		folders = append(folders, wf)
	}
	return folders, nil
}

// AddWatchFolder adds a new directory to the watch_folders table.
func AddWatchFolder(ctx context.Context, pool *pgxpool.Pool, path, action string, config json.RawMessage) (int64, error) {
	if pool == nil {
		return 0, nil
	}

	path = filepath.Clean(strings.TrimSpace(path))
	if len(config) == 0 {
		config = []byte("{}")
	}

	query := `
		INSERT INTO watch_folders (path, action, config, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, TRUE, NOW(), NOW())
		ON CONFLICT (path) DO UPDATE SET
			action = EXCLUDED.action,
			config = EXCLUDED.config,
			enabled = TRUE,
			updated_at = NOW()
		RETURNING id;
	`

	var id int64
	err := pool.QueryRow(ctx, query, path, action, config).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("adding watch folder: %w", err)
	}

	if globalDaemon != nil {
		_ = globalDaemon.reload(ctx)
	}

	return id, nil
}

// DeleteWatchFolder removes a directory from watch_folders.
func DeleteWatchFolder(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	if pool == nil {
		return nil
	}

	query := `DELETE FROM watch_folders WHERE id = $1;`
	_, err := pool.Exec(ctx, query, id)
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("deleting watch folder %d: %w", id, err)
	}

	if globalDaemon != nil {
		_ = globalDaemon.reload(ctx)
	}

	return nil
}
