package feedback

import (
	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, db *loaders.PostgresClient, _ *config.Config) {
	svc := NewService(db)
	ctrl := NewController(svc)
	router.POST("/feedback", ctrl.Submit)
}
