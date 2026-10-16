package web

import (
	"fmt"
	"net/http"

	"github.com/andreassag/morphic/internal/reporter"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerReporterRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/reporter")
	{
		g.POST("/generate", func(c *gin.Context) { handleReporterGenerate(c, pool) })
		g.GET("/download/:id", handleReporterDownload)
	}
}

func handleReporterGenerate(c *gin.Context, pool *pgxpool.Pool) {
	var req reporter.ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	report, err := reporter.GenerateReport(c.Request.Context(), pool, req)
	if err != nil {
		respondError(c, http.StatusBadRequest, "REPORT_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, report)
}

func handleReporterDownload(c *gin.Context) {
	id := c.Param("id")
	report, ok := reporter.GetReport(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Report not found")
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="morphic_report_%s.html"`, id))
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(report.HTMLContent))
}
