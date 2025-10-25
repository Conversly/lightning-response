package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/Conversly/db-ingestor/internal/config"
	"github.com/Conversly/db-ingestor/internal/loaders"
	"github.com/Conversly/db-ingestor/internal/routes"
	"github.com/Conversly/db-ingestor/internal/utils"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Warning: Error loading .env file", err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	cleanup := utils.InitLogger(cfg)
	defer cleanup()

	utils.Zlog.Info("Starting application", 
		zap.String("service", cfg.ServiceName),
		zap.String("environment", cfg.Environment),
		zap.String("port", cfg.ServerPort))

	db, err := loaders.NewPostgresClient(cfg.DatabaseURL, cfg.WorkerCount, cfg.BatchSize)
	if err != nil {
		utils.Zlog.Error("Failed to create database client", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			utils.Zlog.Error("Error closing database connection", zap.Error(err))
		}
	}()

	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	routes.SetupRoutes(router, db, cfg)

	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		utils.Zlog.Info("Starting HTTP server", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			utils.Zlog.Error("Failed to start server", zap.Error(err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	utils.Zlog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		utils.Zlog.Error("Server forced to shutdown", zap.Error(err))
		os.Exit(1)
	}

	utils.Zlog.Info("Server exited")
}