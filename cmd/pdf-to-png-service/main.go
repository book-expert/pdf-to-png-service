/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

// This file orchestrates the pdf-to-png service, initializing and running the NATS
// worker.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/book-expert/logger"
	"github.com/book-expert/pdf-to-png-service/internal/analyzer"
	"github.com/book-expert/pdf-to-png-service/internal/config"
	pdfworker "github.com/book-expert/pdf-to-png-service/internal/worker"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	logFileName = "pdf-to-png-service.log"
)

// main is the entry point of the application.
func main() {
	rootContext, stopSignal := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stopSignal()

	// 1. Initial configuration load for logging
	configuration, configurationError := config.Load("project.toml")
	if configurationError != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", configurationError)
		os.Exit(1)
	}

	// 2. Setup Bootstrap Logger
	logDirectory := os.Getenv("LOG_DIR")
	if logDirectory == "" {
		logDirectory = configuration.Service.LogDir
	}

	appLogger, loggerError := logger.New(logDirectory, logFileName)
	if loggerError != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", loggerError)
		os.Exit(1)
	}
	defer func() {
		_ = appLogger.Close()
	}()

	if runError := run(rootContext, configuration, appLogger); runError != nil {
		appLogger.Errorf("Fatal application error: %v", runError)
		os.Exit(1)
	}
}

// run initializes all components and starts the worker.
func run(parentContext context.Context, configuration *config.Config, appLogger *logger.Logger) error {
	appLogger.Infof("Configuration loaded. Workers: %d, DPI: %d", configuration.Service.Workers, configuration.Service.DPI)

	// 1. Setup Analyzer
	apiKey := os.Getenv(configuration.LLM.APIKeyVariable)
	if apiKey == "" {
		appLogger.Warnf("LLM API Key variable '%s' is not set. Analyzer will fail if called.", configuration.LLM.APIKeyVariable)
	}

	analyzerInstance, analyzerError := analyzer.New(parentContext, analyzer.Config{
		APIKey:                        apiKey,
		Model:                         configuration.LLM.Model,
		TextDirectiveGenerationPrompt: configuration.LLM.TextDirectiveGenerationPrompt,
		MusicConfigGenerationPrompt:   configuration.LLM.MusicConfigGenerationPrompt,
		Timeout:                       time.Duration(configuration.LLM.TimeoutSeconds) * time.Second,
	}, appLogger)
	if analyzerError != nil {
		return analyzerError
	}

	// 2. Setup NATS Connection
	natsConnection, jetStreamContext, natsSetupError := setupNATS(configuration)
	if natsSetupError != nil {
		return natsSetupError
	}
	defer natsConnection.Close()

	// 3. Setup Object Stores
	pdfStore, pngStore, storeError := getObjectStores(
		parentContext,
		jetStreamContext,
		configuration.NATS.ObjectStore.PDFBucket,
		configuration.NATS.ObjectStore.PNGBucket,
	)
	if storeError != nil {
		return storeError
	}

	// 4. Initialize and Start Worker
	workerInstance := pdfworker.New(
		natsConnection,
		jetStreamContext,
		pdfStore,
		pngStore,
		analyzerInstance,
		configuration,
		appLogger,
		configuration.NATS.Producer.Subject,
		configuration.NATS.Producer.PDFProcessingStartedSubject,
		configuration.NATS.DLQSubject,
		configuration.Service.Workers,
	)

	return workerInstance.Start(parentContext)
}

// setupNATS initializes NATS connection and JetStream context.
func setupNATS(configuration *config.Config) (*nats.Conn, jetstream.JetStream, error) {
	natsConnection, connectionError := nats.Connect(configuration.NATS.URL)
	if connectionError != nil {
		return nil, nil, connectionError
	}

	jetStreamContext, jetStreamError := jetstream.New(natsConnection)
	if jetStreamError != nil {
		return nil, nil, jetStreamError
	}

	return natsConnection, jetStreamContext, nil
}

// getObjectStores binds to the object stores for PDFs and PNGs.
func getObjectStores(
	parentContext context.Context,
	jetStreamContext jetstream.JetStream,
	pdfBucket, pngBucket string,
) (pdfStore, pngStore jetstream.ObjectStore, finalError error) {
	var pdfBindError error
	pdfStore, pdfBindError = jetStreamContext.ObjectStore(parentContext, pdfBucket)
	if pdfBindError != nil {
		return nil, nil, fmt.Errorf("failed to bind to PDF object store %s: %w", pdfBucket, pdfBindError)
	}

	var pngBindError error
	pngStore, pngBindError = jetStreamContext.ObjectStore(parentContext, pngBucket)
	if pngBindError != nil {
		return nil, nil, fmt.Errorf("failed to bind to PNG object store %s: %w", pngBucket, pngBindError)
	}

	return pdfStore, pngStore, nil
}
