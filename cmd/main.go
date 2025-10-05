// This file orchestrates the pdf-to-png service, initializing and running the NATS
// worker.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
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
	PDFToPNG    PDFToPNGServiceConfig          `toml:"pdf_to_png_service"`
}

// PathsConfig holds common path configurations.
type PathsConfig struct {
	BaseLogsDir string `toml:"base_logs_dir"`
}

// PDFToPNGServiceConfig holds service-specific settings.
type PDFToPNGServiceConfig struct {
	DeadLetterSubject string `toml:"dead_letter_subject"`
}

// job represents the context for processing a single message.
type job struct {
	msg       jetstream.Msg
	jetStream jetstream.JetStream
	pdfStore  jetstream.ObjectStore
	pngStore  jetstream.ObjectStore
	cfg       *Config
	appLogger *logger.Logger
	event     *events.PDFCreatedEvent
	header    *events.EventHeader
	pdfData   []byte
	pngData   [][]byte
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
		// Use a short-lived bootstrap logger to record fatal errors.
		bootstrapLogger, bErr := logger.New(os.TempDir(), "pdf-to-png-bootstrap.log")
		if bErr == nil {
			bootstrapLogger.Error("Fatal application error: %v", err)
			_ = bootstrapLogger.Close()
		} else {
			// Fallback to stderr if logger cannot be created.
			fmt.Fprintf(os.Stderr, "Fatal application error: %v\n", err)
		}

		return
	}

	// Log graceful shutdown using a bootstrap logger.
	shutdownLogger, sErr := logger.New(os.TempDir(), "pdf-to-png-bootstrap.log")
	if sErr == nil {
		shutdownLogger.Info("Application shut down gracefully.")
		_ = shutdownLogger.Close()
	}
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
			appLogger.Warn("failed to close app logger: %v", err)
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

	// Ensure DLQ stream for the configured subject exists.
	dlqErr := ensureDLQStream(ctx, jetStream, &cfg.ServiceNATS, cfg.PDFToPNG.DeadLetterSubject)
	if dlqErr != nil {
		return dlqErr
	}

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

var errDeadLetterSubjectNotConfigured = errors.New("dead letter subject must be configured")

// ensureDLQStream creates/updates a dedicated dlq stream for the subject if not present.
func ensureDLQStream(
	ctx context.Context,
	jetStream jetstream.JetStream,
	serviceNATS *configurator.ServiceNATSConfig,
	deadLetterSubject string,
) error {
	if strings.TrimSpace(deadLetterSubject) == "" {
		return errDeadLetterSubjectNotConfigured
	}

	for _, s := range serviceNATS.Streams {
		for _, subj := range s.Subjects {
			if subj == deadLetterSubject {
				return nil
			}
		}
	}

	dlq := configurator.StreamConfig{
		Name:     "dlq",
		Subjects: []string{deadLetterSubject},
	}

	err := configurator.CreateOrUpdateStreams(ctx, jetStream, []configurator.StreamConfig{dlq})
	if err != nil {
		return fmt.Errorf("ensure dlq stream: %w", err)
	}

	return nil
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
			// We are in bootstrap; best-effort warn via stderr if logger fails.
			fmt.Fprintf(os.Stderr, "Warning: failed to close temp logger: %v\n", closeErr)
		}
	}()

	loadErr := configurator.Load(&cfg, tempLogger)
	if loadErr != nil {
		return nil, nil, fmt.Errorf(
			"failed to load configuration from URL: %w",
			loadErr,
		)
	}

	tempLogger.Info("Configuration loaded")

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
		// Attempt DLQ publish of original payload.
		handleFailure(ctx, jetStream, msg, cfg.PDFToPNG.DeadLetterSubject, appLogger)

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
		msg:       msg,
		jetStream: jetStream,
		pdfStore:  pdfStore,
		pngStore:  pngStore,
		cfg:       cfg,
		appLogger: appLogger,
		event:     event,
		header:    &event.Header,
		pdfData:   nil,
		pngData:   nil,
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
	handler func(context.Context, error)
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

	processErr := j.processPDF(ctx)
	if processErr != nil {
		return &jobError{err: processErr, handler: j.nak}
	}

	publishErr := j.publishPNGs(ctx)
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

	processingErr := j.executeProcessingSteps(ctx)
	if processingErr != nil {
		j.handleProcessingError(ctx, processingErr)

		return
	}

	j.ack()
}

// handleProcessingError centralizes the logic for handling errors from the core
// processing steps.
func (j *job) handleProcessingError(ctx context.Context, processingErr error) {
	var jErr *jobError
	if errors.As(processingErr, &jErr) {
		j.appLogger.Error(
			"Job [%s] failed: %v",
			j.header.WorkflowID,
			jErr.err,
		)
		jErr.handler(ctx, jErr.err)
	} else {
		j.appLogger.Error(
			"Job [%s] failed with unexpected error: %v",
			j.header.WorkflowID,
			processingErr,
		)
		j.nak(ctx, processingErr)
	}
}

func (j *job) downloadPDF(ctx context.Context) error {
	obj, err := j.pdfStore.Get(ctx, j.event.PDFKey)
	if err != nil {
		return fmt.Errorf("failed to get PDF '%s' from object store: %w", j.event.PDFKey, err)
	}

	defer func() {
		err := obj.Close()
		if err != nil {
			j.appLogger.Warn("failed to close object store object: %v", err)
		}
	}()

	pdfData, err := io.ReadAll(obj)
	if err != nil {
		return fmt.Errorf("failed to read PDF data from object store: %w", err)
	}

	j.pdfData = pdfData

	return nil
}

// processPDF handles the PDF to PNG conversion.
func (j *job) processPDF(ctx context.Context) error {
	opts := &pdfrender.Options{
		InputPath:              "",
		OutputPath:             "",
		ProjectRoot:            "",
		DPI:                    defaultDPI,
		Workers:                defaultWorkerCount,
		BlankFuzzPercent:       defaultFuzzPercent,
		BlankNonWhiteThreshold: defaultNonWhiteThresh,
		ProgressBarOutput:      os.Stdout,
	}
	processor := pdfrender.NewProcessor(opts, j.appLogger)

	pngs, processErr := processor.ProcessSinglePDFFromBytes(ctx, j.pdfData)
	if processErr != nil {
		return fmt.Errorf("failed to process PDF: %w", processErr)
	}

	j.pngData = pngs

	return nil
}

// publishPNGs uploads PNGs to the object store and publishes events.
func (j *job) publishPNGs(ctx context.Context) error {
	pageCount := len(j.pngData)

	j.appLogger.Info(
		"Job [%s]: Found %d PNG(s) to publish.",
		j.header.WorkflowID,
		pageCount,
	)

	for index, pngData := range j.pngData {
		err := j.publishSinglePNG(ctx, pngData, pageCount, index)
		if err != nil {
			return err
		}
	}

	return nil
}

func (j *job) publishSinglePNG(
	ctx context.Context,
	pngData []byte,
	pageCount, index int,
) error {
	objectName := fmt.Sprintf(
		"%s/%s/page_%04d.png",
		j.header.TenantID,
		j.header.WorkflowID,
		index+1,
	)

	uploadErr := uploadBytesToObjectStore(ctx, j.pngStore, objectName, pngData)
	if uploadErr != nil {
		return fmt.Errorf("failed to upload '%s': %w", objectName, uploadErr)
	}

	j.appLogger.Info("Job [%s]: Uploaded '%s'", j.header.WorkflowID, objectName)

	publishEventErr := j.publishPNGCreatedEvent(ctx, objectName, pageCount, index+1, j.event.Augmentation)
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
		PNGKey:       pngKey,
		PageNumber:   pageNum,
		TotalPages:   totalPages,
		Augmentation: j.event.Augmentation,
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

func (j *job) nak(ctx context.Context, reason error) {
	j.appLogger.Error("NAK'ing (via DLQ policy) job [%s]: %v", j.header.WorkflowID, reason)
	handleFailure(ctx, j.jetStream, j.msg, j.cfg.PDFToPNG.DeadLetterSubject, j.appLogger)
}

func (j *job) term(ctx context.Context, reason error) {
	j.appLogger.Error(
		"Terminating (via DLQ policy) job [%s]: %v",
		j.header.WorkflowID,
		reason,
	)
	handleFailure(ctx, j.jetStream, j.msg, j.cfg.PDFToPNG.DeadLetterSubject, j.appLogger)
}

const (
	dlqPublishMaxRetries      = 3
	dlqPublishBackoffDuration = 100 * time.Millisecond
)

// handleFailure publishes the failed payload to the DLQ subject and Ack/NakWithDelay accordingly.
func handleFailure(
	ctx context.Context,
	jetStream jetstream.JetStream,
	msg jetstream.Msg,
	deadLetterSubject string,
	log *logger.Logger,
) {
	if strings.TrimSpace(deadLetterSubject) == "" {
		// No DLQ configured; Nak to avoid loss.
		err := msg.Nak()
		if err != nil {
			log.Error("Failed to NAK without DLQ: %v", err)
		}

		return
	}

	var lastErr error

	payload := msg.Data()
	for attempt := 1; attempt <= dlqPublishMaxRetries; attempt++ {
		_, err := jetStream.Publish(ctx, deadLetterSubject, payload)
		if err == nil {
			ackErr := msg.Ack()
			if ackErr != nil {
				log.Error("Failed to ACK after DLQ publish: %v", ackErr)
			}

			return
		}

		lastErr = err
		log.Warn("DLQ publish attempt %d/%d failed: %v", attempt, dlqPublishMaxRetries, err)
		time.Sleep(dlqPublishBackoffDuration)
	}

	log.Error("Exhausted DLQ publish retries: %v", lastErr)

	err := msg.NakWithDelay(dlqPublishBackoffDuration)
	if err != nil {
		log.Error("Failed to NAK with delay after DLQ failure: %v", err)
	}
}

func uploadBytesToObjectStore(
	ctx context.Context,
	store jetstream.ObjectStore,
	objectName string,
	data []byte,
) error {
	_, putErr := store.PutBytes(ctx, objectName, data)
	if putErr != nil {
		return fmt.Errorf("failed to put file in object store: %w", putErr)
	}

	return nil
}
