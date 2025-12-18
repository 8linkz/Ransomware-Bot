package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// LogRotationConfig contains log rotation settings
type LogRotationConfig struct {
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// logFile holds the reference to the open log file for cleanup
var (
	logFile     *os.File
	logFileMu   sync.Mutex
)

// NewLogger creates a new logger instance with file and console output
// Sets the global logrus logger configuration
func NewLogger(logLevel string, logFilePath string, rotationConfig LogRotationConfig) error {
	// Set log level on global logger
	level, err := parseLogLevel(logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	logrus.SetLevel(level)

	// Open log file with append mode
	logFileMu.Lock()
	defer logFileMu.Unlock()

	// Close previous log file if open
	if logFile != nil {
		logFile.Close()
	}

	var fileErr error
	logFile, fileErr = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if fileErr != nil {
		return fmt.Errorf("failed to open log file: %w", fileErr)
	}

	// Set up multi-writer to write to both file and console
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	logrus.SetOutput(multiWriter)

	// Set custom formatter on global logger
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		ForceColors:     false, // Disable colors for file output
	})

	// Log initial message
	logrus.WithFields(logrus.Fields{
		"level":       logLevel,
		"log_file":    logFilePath,
		"max_size":    fmt.Sprintf("%dMB", rotationConfig.MaxSizeMB),
		"max_backups": rotationConfig.MaxBackups,
		"max_age":     fmt.Sprintf("%d days", rotationConfig.MaxAgeDays),
		"compress":    rotationConfig.Compress,
	}).Info("Logger initialized with file output")

	return nil
}

// Close closes the log file and should be called during application shutdown
func Close() error {
	logFileMu.Lock()
	defer logFileMu.Unlock()

	if logFile != nil {
		err := logFile.Close()
		logFile = nil
		return err
	}
	return nil
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
