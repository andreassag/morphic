package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/andreassag/morphic/internal/dupfinder"
	"github.com/andreassag/morphic/internal/organizer"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/andreassag/morphic/internal/trash"
	"github.com/andreassag/morphic/internal/watcher"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed templates static
var webFS embed.FS

// SetupRouter creates and configures the gin router with all routes and starts background cleanup tasks.
func SetupRouter(ctx context.Context, pool *pgxpool.Pool) (*gin.Engine, error) {
	r := gin.Default()

	// Parse templates (including partials) from embedded FS
	tmpl, err := template.ParseFS(webFS, "templates/*.html", "templates/partials/*.html", "templates/partials/*/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	r.SetHTMLTemplate(tmpl)

	// Serve static files from embedded FS
	staticFS, err := fs.Sub(webFS, "static")
	if err != nil {
		return nil, fmt.Errorf("loading static assets: %w", err)
	}
	r.StaticFS("/static", http.FS(staticFS))

	// No-cache middleware
	r.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	})

	// Set pool for thumbnail L2 cache, dupfinder hash cache, and organizer
	shared.SetThumbnailDBPool(pool)
	dupfinder.SetDBPool(pool)
	organizer.SetDBPool(pool)

	// Health and readiness checks
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		if pool != nil {
			if err := pool.Ping(c.Request.Context()); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unavailable", "error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// Index route
	r.GET("/", func(c *gin.Context) {
		initialFolder := ""
		c.HTML(http.StatusOK, "index.html", gin.H{"initial_folder": initialFolder})
	})

	// Register API route groups
	registerSharedRoutes(r)
	registerOrganizerRoutes(r, pool)
	registerConverterRoutes(r, pool)
	registerDupfinderRoutes(r, pool)
	registerTrashRoutes(r, pool)
	registerCompareRoutes(r)
	registerWatcherRoutes(r, pool)
	registerSSERoutes(r)

	// Start background cleanup tasks & daemons
	dupfinder.StartCleanup(ctx, 30*time.Minute)
	organizer.StartCleanup(ctx, 30*time.Minute)
	StartConverterCleanup(ctx, 30*time.Minute)

	if pool != nil {
		trash.StartAutoPurge(ctx, pool)
		watcher.StartDaemon(ctx, pool)
	}

	return r, nil
}
