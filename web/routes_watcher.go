package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/andreassag/morphic/internal/watcher"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerWatcherRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/watch")
	{
		g.GET("", func(c *gin.Context) {
			folders, err := watcher.ListWatchFolders(c.Request.Context(), pool)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "WATCH_LIST_FAILED", err.Error())
				return
			}
			c.JSON(http.StatusOK, folders)
		})

		g.POST("", func(c *gin.Context) {
			var req struct {
				Path   string          `json:"path"`
				Action string          `json:"action"` // "organize", "convert"
				Config json.RawMessage `json:"config"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
				return
			}
			if req.Path == "" || !isAbsPath(req.Path) || !isDir(req.Path) {
				respondError(c, http.StatusBadRequest, "INVALID_FOLDER", "Valid directory path required")
				return
			}
			if req.Action != "organize" && req.Action != "convert" {
				req.Action = "organize"
			}

			id, err := watcher.AddWatchFolder(c.Request.Context(), pool, req.Path, req.Action, req.Config)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "WATCH_ADD_FAILED", err.Error())
				return
			}

			c.JSON(http.StatusOK, gin.H{"id": id, "status": "active"})
		})

		g.DELETE("/:id", func(c *gin.Context) {
			idStr := c.Param("id")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				respondError(c, http.StatusBadRequest, "INVALID_ID", "Invalid watch folder ID")
				return
			}

			if err := watcher.DeleteWatchFolder(c.Request.Context(), pool, id); err != nil {
				respondError(c, http.StatusInternalServerError, "WATCH_DELETE_FAILED", err.Error())
				return
			}

			c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": id})
		})
	}
}
