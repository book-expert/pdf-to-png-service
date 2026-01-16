/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/book-expert/common-events"
	"github.com/book-expert/logger"
	"github.com/book-expert/pdf-to-png-service/internal/config"
	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
	"github.com/book-expert/pdf-to-png-service/internal/processor"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	logFileName = "pdf-to-png-service.log"
)

func main() {
	rootContext, stopSignal := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stopSignal()

	configuration, configurationLoadError := config.Load("")
	if configurationLoadError != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", configurationLoadError)
		os.Exit(1)
	}

	logDirectory := os.Getenv("LOG_DIR")
	if logDirectory == "" {
		logDirectory = configuration.Service.LogDirectory
	}

	appLogger, loggerInitializationError := logger.New(logDirectory, logFileName)
	if loggerInitializationError != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", loggerInitializationError)
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

func run(parentContext context.Context, configuration *config.Config, appLogger *logger.Logger) error {
	appLogger.Infof("Configuration loaded. Workers: %d, DotsPerInch: %d", configuration.Service.Workers, configuration.Service.DotsPerInch)

	natsConnection, jetStreamContext, natsSetupError := setupNATS(configuration)
	if natsSetupError != nil {
		return natsSetupError
	}
	defer natsConnection.Close()

	pdfObjectStore, pngObjectStore, storeError := getObjectStores(
		parentContext,
		jetStreamContext,
		events.BucketPdfFiles,
		events.BucketPngFiles,
	)
	if storeError != nil {
		return storeError
	}

	pdfRenderer := pdfrender.NewProcessor(&pdfrender.Options{
		DotsPerInch: configuration.Service.DotsPerInch,
		Workers:     configuration.Service.Workers,
	},
		appLogger,
	)

	processorInstance, processorError := processor.NewProcessor(
		natsConnection,
		jetStreamContext,
		jetStreamContext,
		events.StreamPdfFiles,
		events.SubjectPdfCreated,
		"pdf-to-png-consumer",
		events.SubjectPngCreated,
		pdfObjectStore,
		pngObjectStore,
		pdfRenderer,
		appLogger,
		configuration,
	)
	if processorError != nil {
		return processorError
	}

	return processorInstance.Start(parentContext)
}

func setupNATS(configuration *config.Config) (*nats.Conn, jetstream.JetStream, error) {
	natsConnection, connectionError := nats.Connect(configuration.NATS.Address)
	if connectionError != nil {
		return nil, nil, connectionError
	}

	jetStreamContext, jetStreamError := jetstream.New(natsConnection)
	if jetStreamError != nil {
		natsConnection.Close()
		return nil, nil, jetStreamError
	}

	return natsConnection, jetStreamContext, nil
}

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
