// This file orchestrates the pdf-to-png service, initializing and running the NATS
// worker.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/book-expert/configurator"
	"github.com/book-expert/events"
	"github.com/book-expert/logger"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
)

// Config represents the overall configuration structure for the pdf-to-png-service.
type Config struct {
	ServiceNATS configurator.ServiceNATSConfig `toml:"pdf-to-png-service"`
	Paths       PathsConfig                    `toml:"paths"`
}

// PathsConfig holds common path configurations.
type PathsConfig struct {
	BaseLogsDir string `toml:"base_logs_dir"`
}

// job represents the context for processing a single message.
type job struct {
	nats         *nats.Conn
	msg          jetstream.Msg
	jetStream    jetstream.JetStream
	pdfStore     jetstream.ObjectStore
	pngStore     jetstream.ObjectStore
	cfg          *Config
	appLogger    *logger.Logger
	event        *events.PDFCreatedEvent
	header       *events.EventHeader
	outputDir    string
	localPDFPath string
}

const (
	natsFetchTimeout      = 5 * time.Second
	defaultWorkerCount    = 4
	defaultDPI            = 300
	defaultFuzzPercent    = 5
	defaultNonWhiteThresh = 0.005
)

// main is the entry point of the application.
func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	err := run(ctx)
	if err != nil {
		log.Printf("Fatal application error: %v", err)

		return
	}

	log.Println("Application shut down gracefully.")
}

// run initializes all components and starts the message processing loop.
func run(ctx context.Context) error {
	cfg, appLogger, setupErr := setupConfigAndLogger()
	if setupErr != nil {
		return setupErr
	}

	defer func() {
		err := appLogger.Close()
		if err != nil {
			log.Printf("failed to close app logger: %v", err)
		}
	}()

	natsConnection, jetStream, consumer, err := setupNATSComponents(
		ctx,
		&cfg.ServiceNATS,
	)
	if err != nil {
		return fmt.Errorf("failed to setup NATS components: %w", err)
	}
	defer natsConnection.Close()

	appLogger.Info(
		"Worker is running, listening for jobs on '%s'...",
		cfg.ServiceNATS.Consumers[0].FilterSubject,
	)

	return processMessages(ctx, consumer, jetStream, cfg, appLogger)
}

// setupNATSComponents initializes and configures NATS connection, JetStream, and
// consumer.
//
// concrete type is not exported.
//
//nolint:ireturn // The jetstream functions return an interface, and the
func setupNATSComponents(
	ctx context.Context,
	cfg *configurator.ServiceNATSConfig,
) (*nats.Conn, jetstream.JetStream, jetstream.Consumer, error) {
	natsConnection, jetStream, err := configurator.SetupNATSComponents(cfg.NATS)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to setup nats components: %w", err)
	}

	err = configurator.CreateOrUpdateStreams(ctx, jetStream, cfg.Streams)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create or update streams: %w", err)
	}

	err = configurator.CreateOrUpdateConsumers(ctx, jetStream, cfg.Consumers)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create or update consumers: %w", err)
	}

	err = configurator.CreateOrUpdateObjectStores(ctx, jetStream, cfg.ObjectStores)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create or update object stores: %w", err)
	}

	consumer, err := jetStream.Consumer(ctx, cfg.Consumers[0].StreamName, cfg.Consumers[0].ConsumerName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get consumer: %w", err)
	}

	return natsConnection, jetStream, consumer, nil
}

// setupConfigAndLogger loads configuration and sets up the main application logger.
func setupConfigAndLogger() (*Config, *logger.Logger, error) {
	var cfg Config

	tempLogger, tempLoggerErr := logger.New(os.TempDir(), "pdf-to-png-bootstrap.log")
	if tempLoggerErr != nil {
		return nil, nil, fmt.Errorf(
			"failed to create bootstrap logger: %w",
			tempLoggerErr,
		)
	}

	defer func() {
		closeErr := tempLogger.Close()
		if closeErr != nil {
			log.Printf("Warning: failed to close temp logger: %v", closeErr)
		}
	}()

	loadErr := configurator.Load(&cfg, tempLogger)
	if loadErr != nil {
		return nil, nil, fmt.Errorf(
			"failed to load configuration from URL: %w",
			loadErr,
		)
	}

	log.Printf("Configuration loaded")

	appLogger, loggerErr := logger.New(
		cfg.Paths.BaseLogsDir,
		"pdf-to-png-service.log",
	)
	if loggerErr != nil {
		return nil, nil, fmt.Errorf("failed to initialize logger: %w", loggerErr)
	}

	return &cfg, appLogger, nil
}

// processMessages implements the core worker loop.
func processMessages(
	ctx context.Context,
	consumer jetstream.Consumer,
	jetStream jetstream.JetStream,
	cfg *Config,
	appLogger *logger.Logger,
) error {
	pdfStore, pngStore, err := getObjectStores(
		ctx,
		jetStream,
		cfg.ServiceNATS.ObjectStores[0].BucketName,
		cfg.ServiceNATS.ObjectStores[1].BucketName,
	)
	if err != nil {
		return err
	}

	for {
		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("context error in message loop: %w", err)
		}

		processSingleBatch(
			ctx,
			consumer,
			jetStream,
			pdfStore,
			pngStore,
			cfg,
			appLogger,
		)
	}
}

// processSingleBatch fetches and processes one batch of messages from the consumer.
func processSingleBatch(
	ctx context.Context,
	consumer jetstream.Consumer,
	jetStream jetstream.JetStream,
	pdfStore jetstream.ObjectStore,
	pngStore jetstream.ObjectStore,
	cfg *Config,
	appLogger *logger.Logger,
) {
	batch, err := handleMessageBatch(consumer, appLogger)
	if err != nil {
		if errors.Is(err, errNoMessage) {
			return // Not a fatal error, just continue the loop.
		}

		appLogger.Error("Error fetching messages: %v", err)

		return // Logged the error, continue the loop.
	}

	processBatch(ctx, batch, jetStream, pdfStore, pngStore, cfg, appLogger)
}

// getObjectStores retrieves the object stores for PDFs and PNGs.
//
// concrete type is not exported.
//
//nolint:ireturn // The jetstream.ObjectStore function returns an interface, and the
func getObjectStores(
	ctx context.Context,
	jetStream jetstream.JetStream,
	pdfBucket, pngBucket string,
) (pdfStore, pngStore jetstream.ObjectStore, err error) {
	pdfStore, err = jetStream.ObjectStore(ctx, pdfBucket)
	if err != nil {
		err = fmt.Errorf("failed to bind to PDF object store: %w", err)

		return pdfStore, pngStore, err
	}

	pngStore, err = jetStream.ObjectStore(ctx, pngBucket)
	if err != nil {
		err = fmt.Errorf("failed to bind to PNG object store: %w", err)

		return pdfStore, pngStore, err
	}

	return pdfStore, pngStore, err
}

var errNoMessage = errors.New("no message in batch")

// handleMessageBatch fetches a batch of messages from the consumer.
//
// type is not exported.
//
//nolint:ireturn // The consumer.Fetch function returns an interface, and the concrete
func handleMessageBatch(
	consumer jetstream.Consumer,
	appLogger *logger.Logger,
) (jetstream.MessageBatch, error) {
	batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(natsFetchTimeout))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, nats.ErrTimeout) {
			return nil, errNoMessage
		}

		appLogger.Error("Error fetching messages: %v", err)

		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	return batch, nil
}

func processBatch(
	ctx context.Context,
	batch jetstream.MessageBatch,
	jetStream jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore,
	cfg *Config,
	appLogger *logger.Logger,
) {
	for msg := range batch.Messages() {
		handleMessage(
			ctx,
			msg,
			jetStream,
			pdfStore,
			pngStore,
			cfg,
			appLogger,
		)
	}

	err := batch.Error()
	if err != nil {
		appLogger.Error("Error during message batch processing: %v", err)
	}
}

// handleMessage processes a single message.
func handleMessage(
	ctx context.Context, msg jetstream.Msg, jetStream jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore, cfg *Config, appLogger *logger.Logger,
) {
	job, jobErr := newJob(msg, jetStream, pdfStore, pngStore, cfg, appLogger)
	if jobErr != nil {
		appLogger.Error("Failed to create job: %v", jobErr)

		return
	}

	job.run(ctx)
}

// newJob creates a new job handler.
func newJob(
	msg jetstream.Msg,
	jetStream jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore,
	cfg *Config,
	appLogger *logger.Logger,
) (*job, error) {
	event, unmarshalErr := unmarshalEvent(msg)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}

	return &job{
		nats:         nil,
		msg:          msg,
		jetStream:    jetStream,
		pdfStore:     pdfStore,
		pngStore:     pngStore,
		cfg:          cfg,
		appLogger:    appLogger,
		event:        event,
		header:       &event.Header,
		outputDir:    "", // Will be set by setupWorkDir
		localPDFPath: "", // Will be set by setupWorkDir
	}, nil
}

// unmarshalEvent unmarshals the PDFCreatedEvent from a message.
func unmarshalEvent(msg jetstream.Msg) (*events.PDFCreatedEvent, error) {
	var event events.PDFCreatedEvent

	err := json.Unmarshal(msg.Data(), &event)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal PDFCreatedEvent: %w", err)
	}

	return &event, nil
}

// jobError is a custom error to wrap job processing failures with the appropriate
// NATS message handler.
type jobError struct {
	err     error
	handler func(error)
}

// Error implements the error interface for jobError.
func (e *jobError) Error() string {
	return e.err.Error()
}

// executeProcessingSteps runs the core logic of the job after the initial setup.
func (j *job) executeProcessingSteps(ctx context.Context) error {
	downloadErr := j.downloadPDF(ctx)
	if downloadErr != nil {
		return &jobError{err: downloadErr, handler: j.term}
	}

	pngsFinalDir, processErr := j.processPDF(ctx)
	if processErr != nil {
		return &jobError{err: processErr, handler: j.nak}
	}

	publishErr := j.publishPNGs(ctx, pngsFinalDir)
	if publishErr != nil {
		return &jobError{err: publishErr, handler: j.nak}
	}

	return nil
}

// run executes the full lifecycle of a job.
func (j *job) run(ctx context.Context) {
	j.appLogger.Info(
		"Received job for WorkflowID [%s]: processing PDF key '%s'",
		j.header.WorkflowID,
		j.event.PDFKey,
	)

	progErr := j.msg.InProgress()
	if progErr != nil {
		j.appLogger.Warn("Failed to send InProgress update: %v", progErr)
	}

	dirErr := j.setupWorkDir()
	if dirErr != nil {
		j.appLogger.Error(
			"Error setting up work directory for job [%s]: %v",
			j.header.WorkflowID,
			dirErr,
		)
		j.nak(dirErr)

		return
	}
	defer j.cleanupWorkDir()

	processingErr := j.executeProcessingSteps(ctx)
	if processingErr != nil {
		j.handleProcessingError(processingErr)

		return
	}

	j.ack()
}

// handleProcessingError centralizes the logic for handling errors from the core
// processing steps.
func (j *job) handleProcessingError(processingErr error) {
	var jErr *jobError
	if errors.As(processingErr, &jErr) {
		j.appLogger.Error(
			"Job [%s] failed: %v",
			j.header.WorkflowID,
			jErr.err,
		)
		jErr.handler(jErr.err)
	} else {
		j.appLogger.Error(
			"Job [%s] failed with unexpected error: %v",
			j.header.WorkflowID,
			processingErr,
		)
		j.nak(processingErr)
	}
}

func (j *job) setupWorkDir() error {
	outputDir, err := os.MkdirTemp("", fmt.Sprintf("pdf-%s-", j.header.WorkflowID))
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	j.outputDir = outputDir
	j.localPDFPath = filepath.Join(outputDir, j.event.PDFKey)

	return nil
}

func (j *job) cleanupWorkDir() {
	err := os.RemoveAll(j.outputDir)
	if err != nil {
		j.appLogger.Warn(
			"Failed to remove temp directory '%s': %v",
			j.outputDir,
			err,
		)
	}
}

func (j *job) downloadPDF(ctx context.Context) error {
	err := j.pdfStore.GetFile(ctx, j.event.PDFKey, j.localPDFPath)
	if err != nil {
		return fmt.Errorf(
			"failed to get PDF '%s' from object store: %w",
			j.event.PDFKey,
			err,
		)
	}

	return nil
}

// processPDF handles the PDF to PNG conversion.
func (j *job) processPDF(ctx context.Context) (string, error) {
	exeDir, exeErr := getExecutableDir()
	if exeErr != nil {
		return "", fmt.Errorf(
			"could not determine executable directory: %w",
			exeErr,
		)
	}

	opts := &pdfrender.Options{
		InputPath:              filepath.Dir(j.localPDFPath),
		OutputPath:             j.outputDir,
		ProjectRoot:            filepath.Dir(exeDir),
		DPI:                    defaultDPI,
		Workers:                defaultWorkerCount,
		BlankFuzzPercent:       defaultFuzzPercent,
		BlankNonWhiteThreshold: defaultNonWhiteThresh,
		ProgressBarOutput:      os.Stdout,
	}
	processor := pdfrender.NewProcessor(opts, j.appLogger)

	outputDir, processErr := processor.ProcessSinglePDF(ctx, j.localPDFPath)
	if processErr != nil {
		return "", fmt.Errorf("failed to process PDF: %w", processErr)
	}

	return outputDir, nil
}

// publishPNGs uploads PNGs to the object store and publishes events.
func (j *job) publishPNGs(ctx context.Context, pngsFinalDir string) error {
	files, readDirErr := os.ReadDir(pngsFinalDir)
	if readDirErr != nil {
		return fmt.Errorf(
			"could not read final output directory '%s': %w",
			pngsFinalDir,
			readDirErr,
		)
	}

	pngFiles := filterPNGFiles(files)
	pageCount := len(pngFiles)

	j.appLogger.Info(
		"Job [%s]: Found %d PNG(s) to publish.",
		j.header.WorkflowID,
		pageCount,
	)

	for index, file := range pngFiles {
		err := j.publishSinglePNG(ctx, pngsFinalDir, file, pageCount, index)
		if err != nil {
			return err
		}
	}

	return nil
}

// filterPNGFiles filters a slice of directory entries, returning only PNG files.
func filterPNGFiles(files []os.DirEntry) []os.DirEntry {
	var pngFiles []os.DirEntry

	for _, file := range files {
		if !file.IsDir() &&
			strings.HasSuffix(strings.ToLower(file.Name()), ".png") {
			pngFiles = append(pngFiles, file)
		}
	}

	return pngFiles
}

func (j *job) publishSinglePNG(
	ctx context.Context,
	pngsFinalDir string,
	file os.DirEntry,
	pageCount, index int,
) error {
	localPNGPath := filepath.Join(pngsFinalDir, file.Name())
	objectName := fmt.Sprintf(
		"%s/%s/page_%04d.png",
		j.header.TenantID,
		j.header.WorkflowID,
		index+1,
	)

	uploadErr := uploadFileToObjectStore(ctx, j.pngStore, objectName, localPNGPath)
	if uploadErr != nil {
		return fmt.Errorf("failed to upload '%s': %w", objectName, uploadErr)
	}

	j.appLogger.Info("Job [%s]: Uploaded '%s'", j.header.WorkflowID, objectName)

	publishEventErr := j.publishPNGCreatedEvent(ctx, objectName, pageCount, index+1)
	if publishEventErr != nil {
		return fmt.Errorf(
			"failed to publish event for '%s': %w",
			objectName,
			publishEventErr,
		)
	}

	j.appLogger.Info(
		"Job [%s]: Published job for '%s'",
		j.header.WorkflowID,
		objectName,
	)

	return nil
}

// publishPNGCreatedEvent marshals and publishes a PNGCreatedEvent.
func (j *job) publishPNGCreatedEvent(
	ctx context.Context,
	pngKey string,
	totalPages, pageNum int,
) error {
	pngEvent := events.PNGCreatedEvent{
		Header: events.EventHeader{
			WorkflowID: j.header.WorkflowID,
			UserID:     j.header.UserID,
			TenantID:   j.header.TenantID,
			EventID:    uuid.New().String(),
			Timestamp:  time.Now(),
		},
		PNGKey:     pngKey,
		PageNumber: pageNum,
		TotalPages: totalPages,
		Augmentation: &events.AugmentationPreferences{
			Commentary: events.AugmentationCommentarySettings{
				Enabled:            false,
				CustomInstructions: "",
			},
			Summary: events.AugmentationSummarySettings{
				Enabled:            false,
				Placement:          "",
				CustomInstructions: "",
			},
		},
	}

	eventJSON, marshalErr := json.Marshal(pngEvent)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal PNGCreatedEvent: %w", marshalErr)
	}

	_, pubErr := j.jetStream.Publish(ctx, j.cfg.ServiceNATS.Streams[1].Subjects[0], eventJSON)
	if pubErr != nil {
		return fmt.Errorf("failed to publish PNGCreatedEvent: %w", pubErr)
	}

	return nil
}

func (j *job) ack() {
	err := j.msg.Ack()
	if err != nil {
		j.appLogger.Error(
			"Job [%s]: Failed to acknowledge message: %v",
			j.header.WorkflowID,
			err,
		)
	} else {
		j.appLogger.Success("Job [%s]: Processing complete. Acknowledged.", j.header.WorkflowID)
	}
}

func (j *job) nak(reason error) {
	j.appLogger.Error("NAK'ing message for job [%s]: %v", j.header.WorkflowID, reason)

	err := j.msg.Nak()
	if err != nil {
		j.appLogger.Error("Failed to NAK message: %v", err)
	}
}

func (j *job) term(reason error) {
	j.appLogger.Error(
		"Terminating message for job [%s]: %v",
		j.header.WorkflowID,
		reason,
	)

	err := j.msg.Term()
	if err != nil {
		j.appLogger.Error("Failed to TERM message: %v", err)
	}
}

func getExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	return filepath.Dir(exePath), nil
}

func uploadFileToObjectStore(
	ctx context.Context,
	store jetstream.ObjectStore,
	objectName, filePath string,
) error {
	file, openErr := os.Open(filePath) // #nosec G304 -- filePath points to renderer-managed output in the job workspace
	if openErr != nil {
		return fmt.Errorf("failed to open file for upload: %w", openErr)
	}

	defer func() {
		closeErr := file.Close()
		if closeErr != nil {
			log.Printf(
				"Warning: failed to close file '%s': %v",
				filePath,
				closeErr,
			)
		}
	}()

	meta := jetstream.ObjectMeta{
		Name:        objectName,
		Description: "",
		Headers:     nil,
		Metadata:    nil,
		Opts:        nil,
	}

	_, putErr := store.Put(ctx, meta, file)
	if putErr != nil {
		return fmt.Errorf("failed to put file in object store: %w", putErr)
	}

	return nil
}
