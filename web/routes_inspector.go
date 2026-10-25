package web

import (
	"net/http"

	"github.com/andreassag/morphic/internal/inspector"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerInspectorRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/inspector")
	{
		g.POST("/analyze", handleInspectorAnalyze)
	}
}

func handleInspectorAnalyze(c *gin.Context) {
	var req struct {
		Folder  string `json:"folder"`
		MaxTopN int    `json:"max_top_n"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	safeDir, err := shared.ValidateSafeDirPath(req.Folder)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_FOLDER", err.Error())
		return
	}

	res, err := inspector.Analyze(c.Request.Context(), safeDir, req.MaxTopN)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "ANALYSIS_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, res)
}
