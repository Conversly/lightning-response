package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Setup404Handler configures the 404 handler
func Setup404Handler(router *gin.Engine) {
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Not Found",
			"message": "The requested resource was not found",
			"path":    c.Request.URL.Path,
		})
	})
}
