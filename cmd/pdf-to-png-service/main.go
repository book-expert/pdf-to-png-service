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

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nnikolov3/configurator"
	"github.com/nnikolov3/events"
	"github.com/nnikolov3/logger"
	"github.com/nnikolov3/pdf-to-png-service/internal/pdfrender"
)

// Config represents the overall configuration structure for the pdf-to-png-service.
type Config struct {
	NATS  NATSConfig  `toml:"nats"`
	Paths PathsConfig `toml:"paths"`
}

// PathsConfig holds common path configurations.
type PathsConfig struct {
	BaseLogsDir string `toml:"base_logs_dir"`
}

// NATSConfig holds NATS-specific configuration for the pdf-to-png-service.
type NATSConfig struct {
	URL                  string `toml:"url"`
	PDFStreamName        string `toml:"pdf_stream_name"`
	PDFConsumerName      string `toml:"pdf_consumer_name"`
	PDFCreatedSubject    string `toml:"pdf_created_subject"`
	PDFObjectStoreBucket string `toml:"pdf_object_store_bucket"`
	PNGStreamName        string `toml:"png_stream_name"`
	PNGCreatedSubject    string `toml:"png_created_subject"`
	PNGObjectStoreBucket string `toml:"png_object_store_bucket"`
}

// job represents the context for processing a single message.
type job struct {
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
	ackWait               = 30 * time.Second
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

	runErr := run(ctx)
	if runErr != nil {
		log.Printf("Fatal application error: %v", runErr)
		os.Exit(1)
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
		if closeErr := appLogger.Close(); closeErr != nil {
			log.Printf("Warning: failed to close app logger: %v", closeErr)
		}
	}()

	natsConnection, connErr := nats.Connect(cfg.NATS.URL)
	if connErr != nil {
		return fmt.Errorf("failed to connect to NATS: %w", connErr)
	}
	defer natsConnection.Close()
	appLogger.Info("Connected to NATS server at %s", natsConnection.ConnectedUrl())

	jetStream, jsErr := jetstream.New(natsConnection)
	if jsErr != nil {
		return fmt.Errorf("failed to create JetStream context: %w", jsErr)
	}

	jsSetupErr := setupJetStream(ctx, jetStream, cfg)
	if jsSetupErr != nil {
		return fmt.Errorf("failed to set up JetStream resources: %w", jsSetupErr)
	}

	consumer, consumerErr := jetStream.Consumer(
		ctx,
		cfg.NATS.PDFStreamName,
		cfg.NATS.PDFConsumerName,
	)
	if consumerErr != nil {
		return fmt.Errorf("failed to get consumer: %w", consumerErr)
	}

	appLogger.Info("Worker is running, listening for jobs on '%s'...", cfg.NATS.PDFCreatedSubject)
	return processMessages(ctx, consumer, jetStream, cfg, appLogger)
}

// setupConfigAndLogger loads configuration and sets up the main application logger.
func setupConfigAndLogger() (*Config, *logger.Logger, error) {
	var cfg Config
	tempLogger, tempLoggerErr := logger.New(os.TempDir(), "pdf-to-png-bootstrap.log")
	if tempLoggerErr != nil {
		return nil, nil, fmt.Errorf("failed to create bootstrap logger: %w", tempLoggerErr)
	}
	defer func() {
		if closeErr := tempLogger.Close(); closeErr != nil {
			log.Printf("Warning: failed to close temp logger: %v", closeErr)
		}
	}()

	loadErr := configurator.LoadFromURL(configURL, &cfg, tempLogger)
	if loadErr != nil {
		return nil, nil, fmt.Errorf(
			"failed to load configuration from URL %s: %w",
			configURL,
			loadErr,
		)
	}
	log.Printf("Configuration loaded from %s", configURL)

	appLogger, loggerErr := logger.New(cfg.Paths.BaseLogsDir, "pdf-to-png-service.log")
	if loggerErr != nil {
		return nil, nil, fmt.Errorf("failed to initialize logger: %w", loggerErr)
	}

	return &cfg, appLogger, nil
}

// setupJetStream ensures all required NATS streams and object stores exist.
func setupJetStream(ctx context.Context, jetStream jetstream.JetStream, cfg *Config) error {
	streamCfg := newStreamConfig(cfg.NATS.PDFStreamName, cfg.NATS.PDFCreatedSubject)
	_, streamErr := jetStream.CreateStream(ctx, *streamCfg)
	if streamErr != nil && !errors.Is(streamErr, jetstream.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("failed to create PDF stream: %w", streamErr)
	}

	consumerCfg := newConsumerConfig(cfg)
	stream, streamErr := jetStream.Stream(ctx, cfg.NATS.PDFStreamName)
	if streamErr != nil {
		return fmt.Errorf("failed to get PDF stream handle: %w", streamErr)
	}
	_, consumerErr := stream.CreateOrUpdateConsumer(ctx, *consumerCfg)
	if consumerErr != nil {
		return fmt.Errorf("failed to create PDF consumer: %w", consumerErr)
	}

	pngStreamCfg := newStreamConfig(cfg.NATS.PNGStreamName, cfg.NATS.PNGCreatedSubject)
	_, pngStreamErr := jetStream.CreateStream(ctx, *pngStreamCfg)
	if pngStreamErr != nil && !errors.Is(pngStreamErr, jetstream.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("failed to create PNG stream: %w", pngStreamErr)
	}

	for _, bucket := range []string{cfg.NATS.PDFObjectStoreBucket, cfg.NATS.PNGObjectStoreBucket} {
		objStoreCfg := newObjectStoreConfig(bucket)
		_, objStoreErr := jetStream.CreateObjectStore(ctx, *objStoreCfg)
		if objStoreErr != nil && !errors.Is(objStoreErr, jetstream.ErrBucketExists) {
			return fmt.Errorf("failed to create object store '%s': %w", bucket, objStoreErr)
		}
	}
	return nil
}

func newStreamConfig(name, subject string) *jetstream.StreamConfig {
	return &jetstream.StreamConfig{
		Name:                   name,
		Description:            "",
		Subjects:               []string{subject},
		Retention:              jetstream.WorkQueuePolicy,
		MaxConsumers:           -1,
		MaxMsgs:                -1,
		MaxBytes:               -1,
		Discard:                jetstream.DiscardOld,
		DiscardNewPerSubject:   false,
		MaxAge:                 0,
		MaxMsgsPerSubject:      -1,
		MaxMsgSize:             -1,
		Storage:                jetstream.FileStorage,
		Replicas:               1,
		NoAck:                  false,
		Duplicates:             0,
		Placement:              nil,
		Mirror:                 nil,
		Sources:                nil,
		Sealed:                 false,
		DenyDelete:             false,
		DenyPurge:              false,
		AllowRollup:            false,
		Compression:            jetstream.NoCompression,
		FirstSeq:               0,
		SubjectTransform:       nil,
		RePublish:              nil,
		AllowDirect:            false,
		MirrorDirect:           false,
		ConsumerLimits:         jetstream.StreamConsumerLimits{},
		Metadata:               nil,
		Template:               "",
		AllowMsgTTL:            false,
		SubjectDeleteMarkerTTL: 0,
	}
}

func newConsumerConfig(cfg *Config) *jetstream.ConsumerConfig {
	return &jetstream.ConsumerConfig{
		Durable:            cfg.NATS.PDFConsumerName,
		Name:               "",
		Description:        "",
		FilterSubject:      cfg.NATS.PDFCreatedSubject,
		AckPolicy:          jetstream.AckExplicitPolicy,
		AckWait:            ackWait,
		MaxDeliver:         -1,
		DeliverPolicy:      jetstream.DeliverAllPolicy,
		OptStartSeq:        0,
		OptStartTime:       nil,
		BackOff:            nil,
		ReplayPolicy:       jetstream.ReplayInstantPolicy,
		RateLimit:          0,
		SampleFrequency:    "",
		MaxWaiting:         0,
		MaxAckPending:      -1,
		HeadersOnly:        false,
		MaxRequestBatch:    0,
		MaxRequestExpires:  0,
		MaxRequestMaxBytes: 0,
		InactiveThreshold:  0,
		Replicas:           0,
		MemoryStorage:      false,
		FilterSubjects:     nil,
		Metadata:           nil,
		PauseUntil:         nil,
		PriorityPolicy:     0,
		PinnedTTL:          0,
		PriorityGroups:     nil,
		DeliverSubject:     "",
		DeliverGroup:       "",
		FlowControl:        false,
		IdleHeartbeat:      0,
	}
}

func newObjectStoreConfig(bucket string) *jetstream.ObjectStoreConfig {
	return &jetstream.ObjectStoreConfig{
		Bucket:      bucket,
		Description: "",
		TTL:         0,
		MaxBytes:    -1,
		Storage:     jetstream.FileStorage,
		Replicas:    1,
		Placement:   nil,
		Compression: false, // This was the error
		Metadata:    nil,
	}
}

// processMessages implements the core worker loop.
func processMessages(
	ctx context.Context,
	consumer jetstream.Consumer,
	jetStream jetstream.JetStream,
	cfg *Config,
	appLogger *logger.Logger,
) error {
	pdfStore, pdfStoreErr := jetStream.ObjectStore(ctx, cfg.NATS.PDFObjectStoreBucket)
	if pdfStoreErr != nil {
		return fmt.Errorf("failed to bind to PDF object store: %w", pdfStoreErr)
	}
	pngStore, pngStoreErr := jetStream.ObjectStore(ctx, cfg.NATS.PNGObjectStoreBucket)
	if pngStoreErr != nil {
		return fmt.Errorf("failed to bind to PNG object store: %w", pngStoreErr)
	}

	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("context error in message loop: %w", ctxErr)
		}
		batch, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(natsFetchTimeout))
		if fetchErr != nil {
			if errors.Is(fetchErr, context.Canceled) || errors.Is(fetchErr, nats.ErrTimeout) {
				continue
			}
			appLogger.Error("Error fetching messages: %v", fetchErr)
			continue
		}
		for msg := range batch.Messages() {
			handleMessage(ctx, msg, jetStream, pdfStore, pngStore, cfg, appLogger)
		}
		if batchErr := batch.Error(); batchErr != nil {
			appLogger.Error("Error during message batch processing: %v", batchErr)
		}
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
	msg jetstream.Msg, jetStream jetstream.JetStream, pdfStore, pngStore jetstream.ObjectStore,
	cfg *Config, appLogger *logger.Logger,
) (*job, error) {
	event, unmarshalErr := unmarshalEvent(msg)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return &job{
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
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal PDFCreatedEvent: %w", err)
	}
	return &event, nil
}

// run executes the full lifecycle of a job.
func (j *job) run(ctx context.Context) {
	j.appLogger.Info(
		"Received job for WorkflowID [%s]: processing PDF key '%s'",
		j.header.WorkflowID,
		j.event.PDFKey,
	)
	if progErr := j.msg.InProgress(); progErr != nil {
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

	if downloadErr := j.downloadPDF(ctx); downloadErr != nil {
		j.appLogger.Error(
			"Error downloading PDF for job [%s]: %v",
			j.header.WorkflowID,
			downloadErr,
		)
		j.term(downloadErr)
		return
	}

	pngsFinalDir, processErr := j.processPDF(ctx)
	if processErr != nil {
		j.appLogger.Error("Error processing PDF for job [%s]: %v", j.header.WorkflowID, processErr)
		j.nak(processErr)
		return
	}

	if publishErr := j.publishPNGs(ctx, pngsFinalDir); publishErr != nil {
		j.appLogger.Error("Error publishing PNGs for job [%s]: %v", j.header.WorkflowID, publishErr)
		j.nak(publishErr)
		return
	}

	j.ack()
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
	if err := os.RemoveAll(j.outputDir); err != nil {
		j.appLogger.Warn("Failed to remove temp directory '%s': %v", j.outputDir, err)
	}
}

func (j *job) downloadPDF(ctx context.Context) error {
	err := j.pdfStore.GetFile(ctx, j.event.PDFKey, j.localPDFPath)
	if err != nil {
		return fmt.Errorf("failed to get PDF '%s' from object store: %w", j.event.PDFKey, err)
	}
	return nil
}

// processPDF handles the PDF to PNG conversion.
func (j *job) processPDF(ctx context.Context) (string, error) {
	exeDir, exeErr := getExecutableDir()
	if exeErr != nil {
		return "", fmt.Errorf("could not determine executable directory: %w", exeErr)
	}

	opts := &pdfrender.Options{
		InputPath:              j.localPDFPath,
		OutputPath:             j.outputDir,
		ProjectRoot:            filepath.Dir(exeDir),
		DPI:                    defaultDPI,
		Workers:                defaultWorkerCount,
		BlankFuzzPercent:       defaultFuzzPercent,
		BlankNonWhiteThreshold: defaultNonWhiteThresh,
		ProgressBarOutput:      os.Stdout,
	}
	processor := pdfrender.NewProcessor(opts, j.appLogger)

	processErr := processor.ProcessOnePDF(ctx, j.localPDFPath)
	if processErr != nil {
		return "", fmt.Errorf("failed to process PDF: %w", processErr)
	}

	pdfBaseName := strings.TrimSuffix(j.event.PDFKey, filepath.Ext(j.event.PDFKey))
	return filepath.Join(j.outputDir, pdfBaseName, "png"), nil
}

// publishPNGs uploads PNGs to the object store and publishes events.
func (j *job) publishPNGs(ctx context.Context, pngsFinalDir string) error {
	files, readDirErr := os.ReadDir(pngsFinalDir)
	if readDirErr != nil {
		j.appLogger.Warn("Job [%s]: Could not read final output directory '%s': %v",
			j.header.WorkflowID, pngsFinalDir, readDirErr)
		return nil // Not a fatal error
	}

	processor := pdfrender.NewProcessor(&pdfrender.Options{
		ProgressBarOutput: os.Stdout, // Only need a subset of options here
	}, j.appLogger)
	pageCount, pageCountErr := processor.GetPDFPages(ctx, j.localPDFPath)
	if pageCountErr != nil {
		return fmt.Errorf("could not re-verify page count: %w", pageCountErr)
	}

	j.appLogger.Info("Job [%s]: Found %d PNG(s) to publish.", j.header.WorkflowID, len(files))

	for index, file := range files {
		if file.IsDir() || !strings.HasSuffix(strings.ToLower(file.Name()), ".png") {
			continue
		}
		j.publishSinglePNG(ctx, pngsFinalDir, file, pageCount, index)
	}
	return nil
}

func (j *job) publishSinglePNG(
	ctx context.Context,
	pngsFinalDir string,
	file os.DirEntry,
	pageCount, index int,
) {
	localPNGPath := filepath.Join(pngsFinalDir, file.Name())
	objectName := fmt.Sprintf(
		"%s/%s/page_%04d.png",
		j.header.TenantID,
		j.header.WorkflowID,
		index+1,
	)

	if uploadErr := uploadFileToObjectStore(ctx, j.pngStore, objectName, localPNGPath); uploadErr != nil {
		j.appLogger.Error(
			"Job [%s]: Failed to upload '%s': %v",
			j.header.WorkflowID,
			objectName,
			uploadErr,
		)
		return // Continue to next file
	}
	j.appLogger.Info("Job [%s]: Uploaded '%s'", j.header.WorkflowID, objectName)

	publishEventErr := j.publishPNGCreatedEvent(ctx, objectName, pageCount, index+1)
	if publishEventErr != nil {
		j.appLogger.Error(
			"Job [%s]: Failed to publish event for '%s': %v",
			j.header.WorkflowID,
			objectName,
			publishEventErr,
		)
	} else {
		j.appLogger.Info("Job [%s]: Published job for '%s'", j.header.WorkflowID, objectName)
	}
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
	}
	eventJSON, marshalErr := json.Marshal(pngEvent)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal PNGCreatedEvent: %w", marshalErr)
	}
	_, pubErr := j.jetStream.Publish(ctx, j.cfg.NATS.PNGCreatedSubject, eventJSON)
	if pubErr != nil {
		return fmt.Errorf("failed to publish PNGCreatedEvent: %w", pubErr)
	}
	return nil
}

func (j *job) ack() {
	if err := j.msg.Ack(); err != nil {
		j.appLogger.Error("Job [%s]: Failed to acknowledge message: %v", j.header.WorkflowID, err)
	} else {
		j.appLogger.Success("Job [%s]: Processing complete. Acknowledged.", j.header.WorkflowID)
	}
}

func (j *job) nak(reason error) {
	j.appLogger.Error("NAK'ing message for job [%s]: %v", j.header.WorkflowID, reason)
	if err := j.msg.Nak(); err != nil {
		j.appLogger.Error("Failed to NAK message: %v", err)
	}
}

func (j *job) term(reason error) {
	j.appLogger.Error("Terminating message for job [%s]: %v", j.header.WorkflowID, reason)
	if err := j.msg.Term(); err != nil {
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
	file, openErr := os.Open(filePath)
	if openErr != nil {
		return fmt.Errorf("failed to open file for upload: %w", openErr)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Warning: failed to close file '%s': %v", filePath, closeErr)
		}
	}()

	meta := jetstream.ObjectMeta{
		Name:        objectName,
		Description: "",
		Headers:     nil,
		Metadata:    nil,
	}
	_, putErr := store.Put(ctx, meta, file)
	if putErr != nil {
		return fmt.Errorf("failed to put file in object store: %w", putErr)
	}
	return nil
}
