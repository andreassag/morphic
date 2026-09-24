package web

import (
	"net/http"

	"github.com/andreassag/morphic/internal/compare"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
)

func registerCompareRoutes(r *gin.Engine) {
	g := r.Group("/api/media")
	{
		g.GET("/compare", handleMediaCompare)
		g.GET("/diff", handleMediaDiff)
	}
}

func handleMediaCompare(c *gin.Context) {
	left := c.Query("left")
	right := c.Query("right")

	if left == "" || right == "" {
		respondError(c, http.StatusBadRequest, "MISSING_PATHS", "Both 'left' and 'right' paths are required")
		return
	}
	safeLeft, err := shared.ValidateMediaFilePath(left)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PATH", "Invalid left path: "+err.Error())
		return
	}
	safeRight, err := shared.ValidateMediaFilePath(right)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PATH", "Invalid right path: "+err.Error())
		return
	}

	result, err := compare.CompareMetadata(c.Request.Context(), safeLeft, safeRight)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "COMPARE_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, result)
}

func handleMediaDiff(c *gin.Context) {
	left := c.Query("left")
	right := c.Query("right")

	if left == "" || right == "" {
		respondError(c, http.StatusBadRequest, "MISSING_PATHS", "Both 'left' and 'right' paths are required")
		return
	}
	safeLeft, err := shared.ValidateMediaFilePath(left)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PATH", "Invalid left path: "+err.Error())
		return
	}
	safeRight, err := shared.ValidateMediaFilePath(right)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PATH", "Invalid right path: "+err.Error())
		return
	}

	data, err := compare.GenerateDiffImage(c.Request.Context(), safeLeft, safeRight)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "DIFF_FAILED", err.Error())
		return
	}

	c.Data(http.StatusOK, "image/jpeg", data)
}
