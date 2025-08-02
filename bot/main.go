package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"Ransomware-Bot/internal/config"
	"Ransomware-Bot/internal/logger"
	"Ransomware-Bot/internal/scheduler"

	log "github.com/sirupsen/logrus"
)

// Application version
const Version = "1.0.0"

func main() {
	// Parse command line flags
	configDir := flag.String("config-dir", "./configs", "Directory containing configuration files")
	version := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Show version and exit if requested
	if *version {
		fmt.Printf("Discord Threat Intelligence Bot v%s\n", Version)
		os.Exit(0)
	}

	// Validate config directory exists
	if _, err := os.Stat(*configDir); os.IsNotExist(err) {
		fmt.Printf("Error: Config directory '%s' does not exist\n", *configDir)
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configDir)
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	logDir := "./logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("Error creating logs directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger with configurable rotation settings
	rotationConfig := logger.LogRotationConfig{
		MaxSizeMB:  cfg.LogRotation.MaxSizeMB,
		MaxBackups: cfg.LogRotation.MaxBackups,
		MaxAgeDays: cfg.LogRotation.MaxAgeDays,
		Compress:   cfg.LogRotation.Compress,
	}
	_, err = logger.NewLogger(cfg.LogLevel, filepath.Join(logDir, "bot.log"), rotationConfig)
	if err != nil {
		fmt.Printf("Error setting up logger: %v\n", err)
		os.Exit(1)
	}

	// Log initial message using the standard logrus logger
	log.WithField("version", Version).Info("Starting Discord Threat Intelligence Bot")

	// Create application context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize scheduler
	sched, err := scheduler.New(cfg)
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize scheduler")
	}

	// Start scheduler
	if err := sched.Start(ctx); err != nil {
		log.WithError(err).Fatal("Failed to start scheduler")
	}

	log.Info("Bot started successfully")

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	log.Info("Shutdown signal received, stopping bot...")

	// Cancel context to signal all goroutines to stop
	cancel()

	// Stop scheduler
	sched.Stop()

	log.Info("Bot stopped successfully")
}
