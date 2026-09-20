package web

import (
	"net/http"
	"strconv"

	"github.com/exterex/morphic/internal/database"
	"github.com/exterex/morphic/internal/events"
	"github.com/exterex/morphic/internal/shared"
	"github.com/exterex/morphic/internal/trash"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerTrashRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api")
	{
		g.GET("/history", func(c *gin.Context) {
			op := c.Query("operation")
			limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
			offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

			var entries []database.AuditEntry
			var total int64
			var err error

			if pool != nil {
				entries, total, err = database.ListAuditHistory(c.Request.Context(), pool, op, limit, offset)
			} else {
				entries, total, err = trash.ListStandaloneTrash(op, limit, offset)
			}
			if err != nil {
				respondError(c, http.StatusInternalServerError, "HISTORY_FAILED", err.Error())
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"entries": entries,
				"total":   total,
				"limit":   limit,
				"offset":  offset,
			})
		})

		g.POST("/history/:id/undo", func(c *gin.Context) {
			idStr := c.Param("id")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				respondError(c, http.StatusBadRequest, "INVALID_ID", "Invalid audit log ID")
				return
			}

			if err := trash.Restore(c.Request.Context(), pool, id); err != nil {
				respondError(c, http.StatusInternalServerError, "RESTORE_FAILED", err.Error())
				return
			}

			events.DefaultBus.Publish("trash", "restored", gin.H{"id": id})
			c.JSON(http.StatusOK, gin.H{"status": "restored", "id": id})
		})

		g.POST("/trash/purge", func(c *gin.Context) {
			days, _ := strconv.Atoi(c.DefaultQuery("days", "0"))
			purged, freed, err := trash.PurgeExpired(c.Request.Context(), pool, days)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "PURGE_FAILED", err.Error())
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"purged_count":    purged,
				"freed_bytes":     freed,
				"freed_formatted": shared.FormatFileSize(freed),
			})
		})
	}
}
