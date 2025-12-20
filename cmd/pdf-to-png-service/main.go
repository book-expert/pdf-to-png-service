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

	"github.com/book-expert/logger"
	"github.com/book-expert/pdf-to-png-service/internal/analyzer"
	"github.com/book-expert/pdf-to-png-service/internal/events"
	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pelletier/go-toml/v2"
)

// Config represents the configuration structure.
type Config struct {
	Service struct {
		LogDir                 string  `toml:"log_dir"`
		Workers                int     `toml:"workers"`
		DPI                    int     `toml:"dpi"`
		BlankFuzzPercent       int     `toml:"blank_fuzz_percent"`
		BlankNonWhiteThreshold float64 `toml:"blank_non_white_threshold"`
	} `toml:"service"`
	Voices map[string]string `toml:"voices"`
	LLM    struct {
		APIKeyVariable string  `toml:"api_key_variable"`
		Model          string  `toml:"model"`
		AnalysisPrompt string  `toml:"analysis_prompt"`
		TimeoutSeconds int     `toml:"timeout_seconds"`
		Temperature    float64 `toml:"temperature"`
	} `toml:"llm"`
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
	analyzer  *analyzer.Analyzer
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

	// 3. Setup Analyzer
	apiKey := os.Getenv(cfg.LLM.APIKeyVariable)
	if apiKey == "" {
		appLogger.Warnf("LLM API Key variable '%s' is not set. Analyzer will fail if called.", cfg.LLM.APIKeyVariable)
	}

	analyzerInstance, err := analyzer.New(ctx, analyzer.Config{
		APIKey:         apiKey,
		Model:          cfg.LLM.Model,
		AnalysisPrompt: cfg.LLM.AnalysisPrompt,
		Timeout:        time.Duration(cfg.LLM.TimeoutSeconds) * time.Second,
		Voices:         cfg.Voices,
	}, appLogger)
	if err != nil {
		return fmt.Errorf("failed to initialize analyzer: %w", err)
	}

	// 4. Setup NATS
	nc, js, consumer, err := setupNATS(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to setup NATS: %w", err)
	}
	defer nc.Close()

	appLogger.Infof(
		"Worker is running, listening for jobs on '%s'",
		cfg.NATS.Consumer.Subject,
	)

	// 5. Start Processing Loop
	return processMessages(ctx, consumer, js, cfg, analyzerInstance, appLogger)
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
func setupNATS(ctx context.Context, cfg *Config) (*nats.Conn, jetstream.JetStream, jetstream.Consumer, error) {
	nc, err := nats.Connect(cfg.NATS.URL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("jetstream init: %w", err)
	}

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
	analyzer *analyzer.Analyzer,
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
			handleMessage(ctx, msg, jetStream, pdfStore, pngStore, analyzer, cfg, appLogger)
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
	pdfStore, pngStore jetstream.ObjectStore, analyzer *analyzer.Analyzer, cfg *Config, appLogger *logger.Logger,
) {
	job, err := newJob(msg, jetStream, pdfStore, pngStore, analyzer, cfg, appLogger)
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
	analyzer *analyzer.Analyzer,
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
		analyzer:  analyzer,
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

	// Analysis Step
	if err := j.analyzePDF(ctx); err != nil {
		j.appLogger.Errorf("Analysis failed: %v", err)
		// We might want to continue even if analysis fails (fallback), but for now we fail strict.
		// Or maybe publish to DLQ.
		if _, pubErr := j.jetStream.Publish(ctx, j.cfg.NATS.DLQSubject, j.msg.Data()); pubErr != nil {
			j.appLogger.Errorf("Failed to publish to DLQ: %v", pubErr)
		}
		if termErr := j.msg.Term(); termErr != nil {
			j.appLogger.Errorf("Failed to Term message: %v", termErr)
		}
		return
	}

	if err := j.processPDF(ctx); err != nil {
		j.appLogger.Errorf("Processing failed: %v", err)
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

// parseVoice splits a composite voice string (e.g., "niko - calm, deep") into a voice ID and a style.
func parseVoice(voice string) (voiceID, voiceStyle string) {
	parts := strings.SplitN(voice, "-", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	// If there's no "-", the whole string is the ID.
	return strings.TrimSpace(voice), ""
}

func (j *job) analyzePDF(ctx context.Context) error {
	j.appLogger.Infof("Analyzing PDF for Narration Directive...")

	// 1. Initialize inputs and parse voice string.
	input := analyzer.AnalysisInput{}
	var voiceID, voiceStyle, voiceTrait string

	if j.event.Settings != nil {
		input.SoundscapePrompt = j.event.Settings.SoundscapePrompt
		input.Exclusions = j.event.Settings.Exclusions
		if j.event.Settings.Voice != "" {
			voiceID, voiceStyle = parseVoice(j.event.Settings.Voice)

			// Look up the trait from the config, fallback to "unknown"
			if trait, ok := j.cfg.Voices[voiceID]; ok {
				voiceTrait = trait
			} else {
				voiceTrait = "unknown"
			}

			// Fallback: If no style provided in input, use the configured trait
			if voiceStyle == "" {
				voiceStyle = voiceTrait
			}

			input.VoiceName = voiceID
			input.VoiceStyle = voiceStyle
			input.VoiceTrait = voiceTrait
		}
	}
	j.appLogger.Infof("Parsed voice. ID: '%s', Style: '%s', Trait: '%s'", voiceID, voiceStyle, voiceTrait)

	// 2. Call the analyzer to get LLM-generated directives.
	analysisResp, err := j.analyzer.AnalyzePDF(ctx, j.pdfData, input)
	if err != nil {
		return fmt.Errorf("gemini analysis: %w", err)
	}
	j.appLogger.Infof("Analysis Complete. Music Prompt: '%s', Text Directive: '%s'", analysisResp.MusicPrompt, analysisResp.TextDirective)

	// 3. Create the comprehensive AudioSessionConfig.
	config := &events.AudioSessionConfig{
		SessionID:        uuid.New().String(),
		SourceDocumentID: j.event.PDFKey,
		VoiceID:          voiceID,
		VoiceStyle:       voiceStyle,
		MusicPrompt:      analysisResp.MusicPrompt,
		TextDirective:    analysisResp.TextDirective,
	}

	// 4. Update the event settings with the new config.
	if j.event.Settings == nil {
		j.event.Settings = &events.JobSettings{}
	}
	j.event.Settings.AudioSessionConfig = config

	return nil
}

func (j *job) processPDF(ctx context.Context) error {
	opts := &pdfrender.Options{
		DPI:                    j.cfg.Service.DPI,
		Workers:                j.cfg.Service.Workers,
		BlankFuzzPercent:       j.cfg.Service.BlankFuzzPercent,
		BlankNonWhiteThreshold: j.cfg.Service.BlankNonWhiteThreshold,
		ProgressBarOutput:      io.Discard, // Simple logs are enough
	}

	// Create processor
	processor := pdfrender.NewProcessor(opts, j.appLogger)

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
			PNGKey:     pngKey,
			PageNumber: i + 1,
			TotalPages: totalPages,
			Settings:   j.event.Settings, // Settings now include the AudioSessionConfig
		}

		data, _ := json.Marshal(event)
		if _, err := j.jetStream.Publish(ctx, j.cfg.NATS.Producer.Subject, data); err != nil {
			return err
		}
	}
	return nil
}
