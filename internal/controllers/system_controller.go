package controllers

import (
	"net/http"
	"time"

	"github.com/Conversly/db-ingestor/internal/config"
	"github.com/gin-gonic/gin"
)

type SystemController struct {
	cfg *config.Config
}

func NewSystemController(cfg *config.Config) *SystemController {
	return &SystemController{cfg: cfg}
}

// Status godoc
// @Summary Get system status
// @Description Get current system status information
// @Tags system
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/status [get]
func (s *SystemController) Status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":     s.cfg.ServiceName,
		"version":     "1.0.0",
		"environment": s.cfg.Environment,
		"hostname":    s.cfg.Hostname,
		"timestamp":   time.Now().UTC(),
	})
}

// Info godoc
// @Summary Get system information
// @Description Get detailed system information
// @Tags system
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/info [get]
func (s *SystemController) Info(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":     s.cfg.ServiceName,
		"version":     "1.0.0",
		"environment": s.cfg.Environment,
		"hostname":    s.cfg.Hostname,
		"debug":       s.cfg.Debug,
		"log_level":   s.cfg.LogLevel,
		"timestamp":   time.Now().UTC(),
	})
}
