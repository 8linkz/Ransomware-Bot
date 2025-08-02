package logger

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogRotationConfig contains log rotation settings
type LogRotationConfig struct {
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// NewLogger creates a new logger instance with file and console output
// Uses lumberjack for automatic log rotation with configurable settings
func NewLogger(logLevel string, logFilePath string, rotationConfig LogRotationConfig) (*logrus.Logger, error) {
	logger := logrus.New()

	// Set log level
	level, err := parseLogLevel(logLevel)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}
	logger.SetLevel(level)

	// Create lumberjack logger with configurable rotation settings
	rotatingLogFile := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    rotationConfig.MaxSizeMB,
		MaxBackups: rotationConfig.MaxBackups,
		MaxAge:     rotationConfig.MaxAgeDays,
		Compress:   rotationConfig.Compress,
	}

	// Set up multi-writer to write to both file and console
	multiWriter := io.MultiWriter(os.Stdout, rotatingLogFile)
	logger.SetOutput(multiWriter)

	// Set custom formatter
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		ForceColors:     false, // Disable colors for file output
	})

	// Log initial message
	logger.WithFields(logrus.Fields{
		"level":       logLevel,
		"log_file":    logFilePath,
		"max_size":    fmt.Sprintf("%dMB", rotationConfig.MaxSizeMB),
		"max_backups": rotationConfig.MaxBackups,
		"max_age":     fmt.Sprintf("%d days", rotationConfig.MaxAgeDays),
		"compress":    rotationConfig.Compress,
	}).Info("Logger initialized with configurable rotation")

	return logger, nil
}

// parseLogLevel converts string log level to logrus.Level
func parseLogLevel(level string) (logrus.Level, error) {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return logrus.DebugLevel, nil
	case "INFO":
		return logrus.InfoLevel, nil
	case "WARNING", "WARN":
		return logrus.WarnLevel, nil
	case "ERROR":
		return logrus.ErrorLevel, nil
	default:
		return logrus.InfoLevel, fmt.Errorf("unknown log level: %s", level)
	}
}
