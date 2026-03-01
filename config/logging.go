package config

import (
	"fmt"
	"os"
	"path"
	"runtime"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logConfig zap.Config

func CreateLogger() (*zap.Logger, error) {
	logConfig = zap.Config{
		Level: zap.NewAtomicLevelAt(zapcore.InfoLevel),
	}

	logFolder, err := getLogFolder()
	if err != nil {
		return nil, err
	}

	file := zapcore.AddSync(&lumberjack.Logger{
		Filename:   path.Join(logFolder, "app.log"),
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     1, // days
	})

	productionCfg := zap.NewProductionEncoderConfig()
	productionCfg.TimeKey = "timestamp"
	productionCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	fileEncoder := zapcore.NewJSONEncoder(productionCfg)

	// Only log to stderr at WARN level or above to avoid interfering with
	// ProxyCommand usage where stderr output can confuse SSH clients (e.g. VS Code).
	stderr := zapcore.AddSync(os.Stderr)

	developmentCfg := zap.NewDevelopmentEncoderConfig()
	developmentCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder

	consoleEncoder := zapcore.NewConsoleEncoder(developmentCfg)
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, stderr, zap.NewAtomicLevelAt(zapcore.WarnLevel)),
		zapcore.NewCore(fileEncoder, file, logConfig.Level),
	)

	return zap.New(core), nil
}

func SetLogLevel(logLevel string) error {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		return fmt.Errorf("invalid log level %q: %w", logLevel, err)
	}
	logConfig.Level.SetLevel(level)
	return nil
}

func getLogFolder() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user home directory: %w", err)
	}

	switch os := runtime.GOOS; os {
	case "windows":
		return homeDir + "\\AppData\\Local\\ssm-session-client\\logs", nil
	case "darwin":
		return homeDir + "/Library/Logs/ssm-session-client", nil
	default: // Linux and other Unix-like systems
		return homeDir + "/.ssm-session-client/logs", nil
	}
}

// initLogging initializes the logger with the appropriate configuration
