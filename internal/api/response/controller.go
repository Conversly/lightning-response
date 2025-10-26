package response

import (
	"net/http"
	"time"

	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Controller struct {
	graphService *GraphService
}

func NewController(gs *GraphService) *Controller {
	return &Controller{graphService: gs}
}

func (c *Controller) Respond(ctx *gin.Context) {
	var req Request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Zlog.Warn("invalid /response payload", zap.Error(err))
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":     "bad_request",
			"message":   err.Error(),
			"timestamp": time.Now().UTC(),
		})
		return
	}

	var result *Response
	var err error

	result, err = c.graphService.BuildAndRunGraph(ctx.Request.Context(), &req)
	if err != nil {
		utils.Zlog.Error("graph execution failed", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":     "internal_error",
			"message":   err.Error(),
			"timestamp": time.Now().UTC(),
		})
		return
	}

	// Attach request id if available
	if idVal, exists := ctx.Get("request_id"); exists {
		if rid, ok := idVal.(string); ok {
			result.RequestID = rid
		}
	}

	ctx.JSON(http.StatusOK, result)
}
