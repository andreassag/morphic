package web

import (
	"github.com/gin-gonic/gin"
)

// APIErrorDetail holds structured error details.
type APIErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIErrorResponse standardises the error envelope across all endpoints.
type APIErrorResponse struct {
	Error APIErrorDetail `json:"error"`
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": message, // Backwards compatibility for frontend
		"error_detail": APIErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}
