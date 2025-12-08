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
	"syscall"
	"time"

	"github.com/book-expert/events"
	"github.com/book-expert/logger"
	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pelletier/go-toml/v2"
)

// Config represents the simplified configuration structure.
type Config struct {
	Service struct {
		LogDir                 string  `toml:"log_dir"`
		Workers                int     `toml:"workers"`
		DPI                    int     `toml:"dpi"`
		BlankFuzzPercent       int     `toml:"blank_fuzz_percent"`
		BlankNonWhiteThreshold float64 `toml:"blank_non_white_threshold"`
	} `toml:"service"`
	NATS struct {
		URL        string `toml:"url"`
		DLQSubject string `toml:"dlq_subject"`
		Consumer   struct {
			Stream  string `toml:"stream"`
			Subject string `toml:"subject"`
			Durable string `toml:"durable"`
		} `toml:"consumer"`
		Producer struct {
			Stream  string `toml:"stream"`
			Subject string `toml:"subject"`
		} `toml:"producer"`
		ObjectStore struct {
			PDFBucket string `toml:"pdf_bucket"`
			PNGBucket string `toml:"png_bucket"`
		} `toml:"object_store"`
	} `toml:"nats"`
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
	natsFetchTimeout = 5 * time.Second
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
		fmt.Fprintf(os.Stderr, "Fatal application error: %v\n", err)
		os.Exit(1)
	}
}

// run initializes all components and starts the message processing loop.
func run(ctx context.Context) error {
	// 1. Load Configuration
	cfg, err := loadConfig("project.toml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Setup Logger
	appLogger, err := logger.New(cfg.Service.LogDir, "pdf-to-png-service.log")
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() {
		if err := appLogger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close logger: %v\n", err)
		}
	}()

	appLogger.Infof("Configuration loaded. Workers: %d, DPI: %d", cfg.Service.Workers, cfg.Service.DPI)

	// 3. Setup NATS
	nc, js, consumer, err := setupNATS(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to setup NATS: %w", err)
	}
	defer nc.Close()

	appLogger.Infof(
		"Worker is running, listening for jobs on '%s'...",
		cfg.NATS.Consumer.Subject,
	)

	// 4. Start Processing Loop
	return processMessages(ctx, consumer, js, cfg, appLogger)
}

// loadConfig reads and parses the project.toml file.
func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "failed to close config file: %v\n", closeErr)
		}
	}()

	var cfg Config
	decoder := toml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// setupNATS initializes NATS connection and JetStream consumer.
// It assumes streams are already created (simplified approach).
func setupNATS(ctx context.Context, cfg *Config) (*nats.Conn, jetstream.JetStream, jetstream.Consumer, error) {
	nc, err := nats.Connect(cfg.NATS.URL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("jetstream init: %w", err)
	}

	// Get existing consumer or create if not exists (simplified)
	consumer, err := js.CreateOrUpdateConsumer(ctx, cfg.NATS.Consumer.Stream, jetstream.ConsumerConfig{
		Durable:       cfg.NATS.Consumer.Durable,
		FilterSubject: cfg.NATS.Consumer.Subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get/create consumer: %w", err)
	}

	return nc, js, consumer, nil
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
		cfg.NATS.ObjectStore.PDFBucket,
		cfg.NATS.ObjectStore.PNGBucket,
	)
	if err != nil {
		return err
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(natsFetchTimeout))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			appLogger.Errorf("Error fetching messages: %v", err)
			time.Sleep(1 * time.Second) // Backoff
			continue
		}

		for msg := range batch.Messages() {
			handleMessage(ctx, msg, jetStream, pdfStore, pngStore, cfg, appLogger)
		}
	}
}

// getObjectStores retrieves the object stores for PDFs and PNGs.
func getObjectStores(
	ctx context.Context,
	jetStream jetstream.JetStream,
	pdfBucket, pngBucket string,
) (pdfStore, pngStore jetstream.ObjectStore, err error) {
	pdfStore, err = jetStream.ObjectStore(ctx, pdfBucket)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to bind to PDF object store: %w", err)
	}

	pngStore, err = jetStream.ObjectStore(ctx, pngBucket)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to bind to PNG object store: %w", err)
	}

	return pdfStore, pngStore, nil
}

// handleMessage processes a single message.
func handleMessage(
	ctx context.Context, msg jetstream.Msg, jetStream jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore, cfg *Config, appLogger *logger.Logger,
) {
	job, err := newJob(msg, jetStream, pdfStore, pngStore, cfg, appLogger)
	if err != nil {
		appLogger.Errorf("Failed to create job: %v", err)
		if nakErr := msg.Nak(); nakErr != nil {
			appLogger.Errorf("Failed to NAK message: %v", nakErr)
		}
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
	var event events.PDFCreatedEvent
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}

	return &job{
		msg:       msg,
		jetStream: jetStream,
		pdfStore:  pdfStore,
		pngStore:  pngStore,
		cfg:       cfg,
		appLogger: appLogger,
		event:     &event,
		header:    &event.Header,
	}, nil
}

// executeProcessingSteps runs the core logic of the job.
func (j *job) run(ctx context.Context) {
	j.appLogger.Infof("Processing PDF: %s", j.event.PDFKey)
	if err := j.msg.InProgress(); err != nil {
		j.appLogger.Warnf("Failed to send InProgress: %v", err)
	}

	if err := j.downloadPDF(ctx); err != nil {
		j.appLogger.Errorf("Download failed: %v", err)
		if nakErr := j.msg.Nak(); nakErr != nil {
			j.appLogger.Errorf("Failed to NAK after download fail: %v", nakErr)
		}
		return
	}

	if err := j.processPDF(ctx); err != nil {
		j.appLogger.Errorf("Processing failed: %v", err)
		// Publish to DLQ
		if _, pubErr := j.jetStream.Publish(ctx, j.cfg.NATS.DLQSubject, j.msg.Data()); pubErr != nil {
			j.appLogger.Errorf("Failed to publish to DLQ: %v", pubErr)
		}
		if termErr := j.msg.Term(); termErr != nil {
			j.appLogger.Errorf("Failed to Term message: %v", termErr)
		}
		return
	}

	if err := j.publishPNGs(ctx); err != nil {
		j.appLogger.Errorf("Publish failed: %v", err)
		if nakErr := j.msg.Nak(); nakErr != nil {
			j.appLogger.Errorf("Failed to NAK after publish fail: %v", nakErr)
		}
		return
	}

	if ackErr := j.msg.Ack(); ackErr != nil {
		j.appLogger.Errorf("Failed to ACK message: %v", ackErr)
	}
	j.appLogger.Successf("Completed: %s", j.event.PDFKey)
}

func (j *job) downloadPDF(ctx context.Context) error {
	obj, err := j.pdfStore.Get(ctx, j.event.PDFKey)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := obj.Close(); closeErr != nil {
			j.appLogger.Warnf("Failed to close object store object: %v", closeErr)
		}
	}()

	j.pdfData, err = io.ReadAll(obj)
	return err
}

func (j *job) processPDF(ctx context.Context) error {
	opts := &pdfrender.Options{
		DPI:                    j.cfg.Service.DPI,
		Workers:                j.cfg.Service.Workers,
		BlankFuzzPercent:       j.cfg.Service.BlankFuzzPercent,
		BlankNonWhiteThreshold: j.cfg.Service.BlankNonWhiteThreshold,
		ProgressBarOutput:      io.Discard, // Simple logs are enough
	}

	// Create processor (simplified instantiation)
	processor := pdfrender.NewProcessor(opts, j.appLogger)

	// We assume ProcessSinglePDFFromBytes is the method we want to keep
	pngs, err := processor.ProcessSinglePDFFromBytes(ctx, j.pdfData)
	if err != nil {
		return err
	}
	j.pngData = pngs
	return nil
}

func (j *job) publishPNGs(ctx context.Context) error {
	totalPages := len(j.pngData)
	for i, png := range j.pngData {
		pngKey := fmt.Sprintf("%s-%d.png", j.event.PDFKey, i+1)

		if _, err := j.pngStore.PutBytes(ctx, pngKey, png); err != nil {
			return err
		}

		event := events.PNGCreatedEvent{
			Header: events.EventHeader{
				WorkflowID: j.header.WorkflowID,
				UserID:     j.header.UserID,
				TenantID:   j.header.TenantID,
				EventID:    uuid.New().String(),
				Timestamp:  time.Now(),
			},
			PNGKey:       pngKey,
			PageNumber:   i + 1,
			TotalPages:   totalPages,
			Augmentation: j.event.Augmentation,
		}

		data, _ := json.Marshal(event)
		if _, err := j.jetStream.Publish(ctx, j.cfg.NATS.Producer.Subject, data); err != nil {
			return err
		}
	}
	return nil
}
