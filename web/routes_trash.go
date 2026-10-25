package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/events"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/andreassag/morphic/internal/trash"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerTrashRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api")
	{
		// Safe Trash endpoint: Returns recoverable deleted media files
		g.GET("/trash", func(c *gin.Context) {
			limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
			offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

			entries, total, err := trash.ListSafeTrash(c.Request.Context(), pool, limit, offset)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "TRASH_FAILED", err.Error())
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"entries": entries,
				"total":   total,
				"limit":   limit,
				"offset":  offset,
			})
		})

		// Audit History endpoint: Returns high-level bulk operations
		g.GET("/history", func(c *gin.Context) {
			op := c.Query("operation")
			limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
			offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

			var operations []database.AuditOperation
			var total int64
			var err error

			if pool != nil {
				operations, total, err = database.ListAuditOperations(c.Request.Context(), pool, op, limit, offset)
			} else {
				operations, total, err = trash.ListStandaloneOperations(op, limit, offset)
			}
			if err != nil {
				respondError(c, http.StatusInternalServerError, "HISTORY_FAILED", err.Error())
				return
			}

			// Fallback for existing legacy entries when audit_operations is empty
			if total == 0 {
				var legacyEntries []database.AuditEntry
				if pool != nil {
					legacyEntries, total, err = database.ListAuditHistory(c.Request.Context(), pool, op, limit, offset)
				} else {
					legacyEntries, total, err = trash.ListStandaloneTrash(op, limit, offset)
				}
				if err == nil && len(legacyEntries) > 0 {
					for _, le := range legacyEntries {
						summary := fmt.Sprintf("%s: %s", le.Operation, le.OriginalPath)
						if le.Operation == "delete" {
							summary = fmt.Sprintf("Safe-trashed file: %s", le.OriginalPath)
						} else if le.Operation == "convert" {
							summary = fmt.Sprintf("Converted %s to %s", le.OriginalPath, le.DestinationPath)
						}
						operations = append(operations, database.AuditOperation{
							ID:        le.ID,
							Operation: le.Operation,
							Source:    le.Source,
							Summary:   summary,
							ItemCount: 1,
							TotalSize: le.FileSize,
							Status:    "completed",
							CreatedAt: le.CreatedAt,
						})
					}
				}
			}

			c.JSON(http.StatusOK, gin.H{
				"entries": operations,
				"total":   total,
				"limit":   limit,
				"offset":  offset,
			})
		})

		// Restore trashed item by ID
		restoreHandler := func(c *gin.Context) {
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

			// Log high-level operation for media restore
			op := database.AuditOperation{
				Operation: "restore",
				Source:    "trash",
				Summary:   fmt.Sprintf("Restored media item #%d to original location", id),
				ItemCount: 1,
				Status:    "completed",
				CreatedAt: time.Now(),
			}
			if pool != nil {
				_, _ = database.LogOperation(c.Request.Context(), pool, op)
			} else {
				_ = trash.LogStandaloneOperation(op)
			}

			events.DefaultBus.Publish("trash", "restored", gin.H{"id": id})
			events.DefaultBus.Publish("history", "operation_logged", op)
			c.JSON(http.StatusOK, gin.H{"status": "restored", "id": id})
		}

		g.POST("/history/:id/undo", restoreHandler)
		g.POST("/trash/:id/restore", restoreHandler)

		// Permanently delete a single trashed item
		deleteSingleHandler := func(c *gin.Context) {
			idStr := c.Param("id")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				respondError(c, http.StatusBadRequest, "INVALID_ID", "Invalid audit log ID")
				return
			}

			freedBytes, err := trash.DeleteItem(c.Request.Context(), pool, id)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
				return
			}

			op := database.AuditOperation{
				Operation: "purge",
				Source:    "trash",
				Summary:   fmt.Sprintf("Permanently deleted item #%d from safe trash (freed %s)", id, shared.FormatFileSize(freedBytes)),
				ItemCount: 1,
				TotalSize: freedBytes,
				Status:    "completed",
				CreatedAt: time.Now(),
			}
			if pool != nil {
				_, _ = database.LogOperation(c.Request.Context(), pool, op)
			} else {
				_ = trash.LogStandaloneOperation(op)
			}

			events.DefaultBus.Publish("trash", "deleted", gin.H{"id": id, "freed_bytes": freedBytes})
			events.DefaultBus.Publish("history", "operation_logged", op)
			c.JSON(http.StatusOK, gin.H{
				"status":          "deleted",
				"id":              id,
				"freed_bytes":     freedBytes,
				"freed_formatted": shared.FormatFileSize(freedBytes),
			})
		}

		g.DELETE("/trash/:id", deleteSingleHandler)

		// Empty all items from safe trash
		emptyTrashHandler := func(c *gin.Context) {
			count, freed, err := trash.EmptyTrash(c.Request.Context(), pool)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "EMPTY_FAILED", err.Error())
				return
			}

			if count > 0 {
				op := database.AuditOperation{
					Operation: "purge",
					Source:    "trash",
					Summary:   fmt.Sprintf("Emptied safe trash (%d item(s), freed %s)", count, shared.FormatFileSize(freed)),
					ItemCount: count,
					TotalSize: freed,
					Status:    "completed",
					CreatedAt: time.Now(),
				}
				if pool != nil {
					_, _ = database.LogOperation(c.Request.Context(), pool, op)
				} else {
					_ = trash.LogStandaloneOperation(op)
				}
				events.DefaultBus.Publish("trash", "emptied", gin.H{"count": count, "freed_bytes": freed})
				events.DefaultBus.Publish("history", "operation_logged", op)
			}

			c.JSON(http.StatusOK, gin.H{
				"status":          "emptied",
				"deleted_count":   count,
				"freed_bytes":     freed,
				"freed_formatted": shared.FormatFileSize(freed),
			})
		}

		g.DELETE("/trash", emptyTrashHandler)
		g.POST("/trash/empty", emptyTrashHandler)

		// Purge expired files from safe trash
		g.POST("/trash/purge", func(c *gin.Context) {
			daysStr := c.DefaultQuery("days", "-1")
			days, err := strconv.Atoi(daysStr)
			if err != nil {
				days = -1
			}

			purged, freed, err := trash.PurgeExpired(c.Request.Context(), pool, days)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "PURGE_FAILED", err.Error())
				return
			}

			if purged > 0 {
				meta, _ := json.Marshal(map[string]interface{}{
					"purged_count": purged,
					"freed_bytes":  freed,
				})
				op := database.AuditOperation{
					Operation: "purge",
					Source:    "trash",
					Summary:   fmt.Sprintf("Purged %d expired files from safe trash (freed %s)", purged, shared.FormatFileSize(freed)),
					ItemCount: purged,
					TotalSize: freed,
					Status:    "completed",
					Metadata:  meta,
					CreatedAt: time.Now(),
				}
				if pool != nil {
					_, _ = database.LogOperation(c.Request.Context(), pool, op)
				} else {
					_ = trash.LogStandaloneOperation(op)
				}
				events.DefaultBus.Publish("trash", "purged", gin.H{"purged_count": purged, "freed_bytes": freed})
				events.DefaultBus.Publish("history", "operation_logged", op)
			}

			c.JSON(http.StatusOK, gin.H{
				"purged_count":    purged,
				"freed_bytes":     freed,
				"freed_formatted": shared.FormatFileSize(freed),
			})
		})
	}
}
