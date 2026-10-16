package web

import (
	"fmt"
	"net/http"

	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/trimmer"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerTrimmerRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/trimmer")
	{
		g.POST("/preview", handleTrimmerPreview)
		g.POST("/export", func(c *gin.Context) { handleTrimmerExport(c, pool) })
	}
}

func handleTrimmerPreview(c *gin.Context) {
	var opts trimmer.ImageTransformOpts
	if err := c.ShouldBindJSON(&opts); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	data, err := trimmer.PreviewImageTransform(c.Request.Context(), opts)
	if err != nil {
		respondError(c, http.StatusBadRequest, "PREVIEW_FAILED", err.Error())
		return
	}

	c.Data(http.StatusOK, "image/jpeg", data)
}

func handleTrimmerExport(c *gin.Context, pool *pgxpool.Pool) {
	var opts trimmer.ExportOpts
	if err := c.ShouldBindJSON(&opts); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	ctx := c.Request.Context()
	outPath, err := trimmer.ExportMedia(ctx, opts)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
		return
	}

	if pool != nil {
		_, _ = database.LogOperation(ctx, pool, database.AuditOperation{
			Operation: "trim_media",
			Source:    "trimmer",
			Summary:   fmt.Sprintf("Trimmed/transformed %s to %s", opts.SourcePath, outPath),
			ItemCount: 1,
			Status:    "completed",
		})
	}

	c.JSON(http.StatusOK, gin.H{"output_path": outPath})
}
