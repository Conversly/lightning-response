package response

import (
    "github.com/Conversly/db-ingestor/internal/config"
    "github.com/Conversly/db-ingestor/internal/loaders"
    "github.com/gin-gonic/gin"
)

// RegisterRoutes registers the /response endpoint at the root level
func RegisterRoutes(router *gin.Engine, db *loaders.PostgresClient, cfg *config.Config) {
    svc := NewService(db, cfg)
    ctrl := NewController(svc)
    router.POST("/response", ctrl.Respond)
}
