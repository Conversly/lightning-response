package widget

import (
	"net/http"
	"time"

	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Controller handles widget HTTP requests
type Controller struct {
	graphService *GraphService
}

// NewController creates a new widget controller
func NewController(gs *GraphService) *Controller {
	return &Controller{graphService: gs}
}

// Respond handles the /response endpoint
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

	result, err := c.graphService.BuildAndRunGraph(ctx.Request.Context(), &req)
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

// PlaygroundResponse handles the /playground/response endpoint
func (c *Controller) PlaygroundResponse(ctx *gin.Context) {
	var req PlaygroundRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Zlog.Warn("invalid /playground/response payload", zap.Error(err))
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":     "bad_request",
			"message":   err.Error(),
			"timestamp": time.Now().UTC(),
		})
		return
	}

	result, err := c.graphService.BuildAndRunPlaygroundGraph(ctx.Request.Context(), &req)
	if err != nil {
		utils.Zlog.Error("playground graph execution failed", zap.Error(err))
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

