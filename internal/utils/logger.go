// utils/logger.go
package utils

import (
	"os"

	"github.com/Conversly/db-ingestor/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Zlog *zap.Logger

func InitLogger(cfg *config.Config) func() {
	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}

	var lvl zapcore.Level
	_ = lvl.Set(logLevel)

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "timestamp"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.CapitalLevelEncoder

	stdoutCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encCfg),
		zapcore.AddSync(os.Stdout),
		lvl,
	)

	Zlog = zap.New(stdoutCore, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return func() { _ = Zlog.Sync() }
}
