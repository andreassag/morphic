package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/exterex/morphic/internal/dupfinder"
	"github.com/exterex/morphic/internal/organizer"
	"github.com/gin-gonic/gin"
)

//go:embed templates static
var webFS embed.FS

// SetupRouter creates and configures the gin router with all routes and starts background cleanup tasks.
func SetupRouter(ctx context.Context) (*gin.Engine, error) {
	r := gin.Default()

	// Parse templates from embedded FS
	tmpl, err := template.ParseFS(webFS, "templates/*.html")
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

	// Health and readiness checks
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// Index route
	r.GET("/", func(c *gin.Context) {
		initialFolder := ""
		c.HTML(http.StatusOK, "index.html", gin.H{"initial_folder": initialFolder})
	})

	// Register API route groups
	registerSharedRoutes(r)
	registerOrganizerRoutes(r)
	registerConverterRoutes(r)
	registerDupfinderRoutes(r)

	// Start background job stores cleanup
	dupfinder.StartCleanup(ctx, 30*time.Minute)
	organizer.StartCleanup(ctx, 30*time.Minute)
	StartConverterCleanup(ctx, 30*time.Minute)

	return r, nil
}
