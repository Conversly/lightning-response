package feedback

import (
	"net/http"
	"time"

	"github.com/Conversly/lightning-response/internal/types"
	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Controller struct {
	svc *Service
}

func NewController(svc *Service) *Controller {
	return &Controller{svc: svc}
}

func (c *Controller) Submit(ctx *gin.Context) {
	var req Request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		utils.Zlog.Warn("invalid /feedback payload", zap.Error(err))
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":     "bad_request",
			"message":   err.Error(),
			"timestamp": time.Now().UTC(),
		})
		return
	}

	if err := c.svc.SubmitFeedback(ctx.Request.Context(), &req); err != nil {
		utils.Zlog.Warn("feedback update failed", zap.Error(err))
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":     "feedback_error",
			"message":   err.Error(),
			"timestamp": time.Now().UTC(),
		})
		return
	}

	res := Response{BaseResponse: types.BaseResponse{Success: true}}
	if idVal, exists := ctx.Get("request_id"); exists {
		if rid, ok := idVal.(string); ok {
			res.RequestID = rid
		}
	}
	ctx.JSON(http.StatusOK, res)
}
