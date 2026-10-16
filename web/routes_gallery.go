package web

import (
	"net/http"

	"github.com/andreassag/morphic/internal/gallery"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerGalleryRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/gallery")
	{
		g.GET("/items", handleGalleryItems)
		g.POST("/list", handleGalleryItems)
	}
}

func handleGalleryItems(c *gin.Context) {
	var query gallery.GalleryQuery
	if c.Request.Method == http.MethodPost {
		if err := c.ShouldBindJSON(&query); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
	} else {
		if err := c.ShouldBindQuery(&query); err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
	}

	if query.Folder == "" {
		respondError(c, http.StatusBadRequest, "EMPTY_FOLDER", "Folder parameter is required")
		return
	}

	res, err := gallery.ListMedia(c.Request.Context(), query)
	if err != nil {
		respondError(c, http.StatusBadRequest, "GALLERY_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, res)
}
