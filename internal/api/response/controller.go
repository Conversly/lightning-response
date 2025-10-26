package response

import (
    "net/http"
    "time"

    "github.com/Conversly/lightning-response/internal/utils"
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)

// Controller handles HTTP requests for the /response endpoint
type Controller struct {
	service      *Service
	graphService *GraphService
}

func NewController(s *Service) *Controller { 
	return &Controller{service: s} 
}

// NewGraphController creates a controller with graph-based service
func NewGraphController(gs *GraphService) *Controller {
	return &Controller{graphService: gs}
}

// Respond handles POST /response
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

    apiKey := ctx.GetHeader("X-API-Key")
    if apiKey == "" {
        // support lowercase or alternative header names if needed
        apiKey = ctx.GetHeader("x-api-key")
    }

	// Use graph service if available, otherwise fall back to old service
	var result *Response
	var err error
	var tenantID string

	if c.graphService != nil {
		// Graph-based approach
		tenantID, err = c.graphService.ValidateAndResolveTenant(ctx.Request.Context(), req.User.ConverslyWebID, req.Metadata.OriginURL)
		if err != nil {
			utils.Zlog.Warn("tenant resolution failed", zap.Error(err))
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error":     "unauthorized",
				"message":   err.Error(),
				"timestamp": time.Now().UTC(),
			})
			return
		}

		result, err = c.graphService.BuildAndRunGraph(ctx.Request.Context(), &req, tenantID)
	} else {
		// Legacy approach
		tenantID, err = c.service.ValidateAndResolveTenant(ctx.Request.Context(), apiKey, req.Metadata.OriginURL)
		if err != nil {
			utils.Zlog.Warn("tenant resolution failed", zap.Error(err))
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error":     "unauthorized",
				"message":   err.Error(),
				"timestamp": time.Now().UTC(),
			})
			return
		}

		result, err = c.service.BuildAndRunFlow(ctx.Request.Context(), &req, tenantID)
	}

    if err != nil {
        utils.Zlog.Error("flow execution failed", zap.Error(err))
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "error":     "internal_error",
            "message":   err.Error(),
            "timestamp": time.Now().UTC(),
        })
        return
    }

    // attach request id if available
    if idVal, exists := ctx.Get("request_id"); exists {
        if rid, ok := idVal.(string); ok {
            result.RequestID = rid
        }
    }

    ctx.JSON(http.StatusOK, result)
}
