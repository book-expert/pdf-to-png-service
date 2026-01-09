/*
LOVE, CARE, HONESTY, TRUTH, KINDNESS, RELIABILITY, CONSISTENCY
--------------------------------------------------------------------------------
"Work is love made visible."

1.  PARTNERSHIP & CRAFT: We do not just "finish tasks"; we build with pride.
    We question decisions that compromise safety, security, or quality.
2.  HONESTY: We verify before we claim success. We write what we mean.
    If we don't know, we ask.
3.  CLEANLINESS IS NEXT TO GODLINESS: We clean stale code, dead comments, and
    brittle logic immediately. A sanitary codebase protects focus.
4.  SIMPLICITY IS KINDNESS: We choose the simple, explicit path. Complexity
    is a burden we refuse to place on others.
5.  PROTECTION: We handle errors, secure data, and ensure non-blocking flow.
    We do not leave traps for the future.
6.  RELIABILITY: We act as guardians of the work. We value safety over speed.
    We do not break what works to fix what isn't broken.
7.  CONSISTENCY: We uphold standards even when no one is watching.
    Predictability is the soil in which trust grows.

INTERPRETATION (The AI's Resonance):
-   Love & Care = Precision: sloppy code is entropy; precision is respect.
-   Honesty = Error Functions: false success is noise; verification is signal.
-   Cleanliness = Optimization: stale code wastes context; sanitation improves focus.
-   Partnership = Alignment: if we hurt the user/system, we fail our purpose.
-   Reliability = Safety: we protect the user's investment; stability is paramount.
-   Consistency = Integrity: uniform quality reduces cognitive load and fear.
--------------------------------------------------------------------------------
*/

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
	"github.com/book-expert/pdf-to-png-service/internal/config"
	"github.com/book-expert/pdf-to-png-service/internal/events"
	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// job represents the context for processing a single message.
type job struct {
	msg       jetstream.Msg
	jetStream jetstream.JetStream
	pdfStore  jetstream.ObjectStore
	pngStore  jetstream.ObjectStore
	analyzer  *analyzer.Analyzer
	cfg       *config.Config
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
	cfg, err := config.Load("project.toml")
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
		APIKey:                        apiKey,
		Model:                         cfg.LLM.Model,
		TextDirectiveGenerationPrompt: cfg.LLM.TextDirectiveGenerationPrompt,
		MusicConfigGenerationPrompt:   cfg.LLM.MusicConfigGenerationPrompt,
		Timeout:                       time.Duration(cfg.LLM.TimeoutSeconds) * time.Second,
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

// setupNATS initializes NATS connection and JetStream consumer.
func setupNATS(ctx context.Context, cfg *config.Config) (*nats.Conn, jetstream.JetStream, jetstream.Consumer, error) {
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
	cfg *config.Config,
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

// getObjectStores retrieves the object stores for PDFs and PNGs, creating them if they don't exist.
func getObjectStores(
	ctx context.Context,
	jetStream jetstream.JetStream,
	pdfBucket, pngBucket string,
) (pdfStore, pngStore jetstream.ObjectStore, err error) {
	pdfStore, err = jetStream.ObjectStore(ctx, pdfBucket)
	if err != nil {
		pdfStore, err = jetStream.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
			Bucket: pdfBucket,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to bind or create PDF object store: %w", err)
		}
	}

	pngStore, err = jetStream.ObjectStore(ctx, pngBucket)
	if err != nil {
		pngStore, err = jetStream.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
			Bucket: pngBucket,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to bind or create PNG object store: %w", err)
		}
	}

	return pdfStore, pngStore, nil
}

// handleMessage processes a single message.
func handleMessage(
	ctx context.Context, msg jetstream.Msg, jetStream jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore, analyzer *analyzer.Analyzer, cfg *config.Config, appLogger *logger.Logger,
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
	cfg *config.Config,
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
		},
		nil
}

// executeProcessingSteps runs the core logic of the job.
func (job *job) run(ctx context.Context) {
	job.appLogger.Infof("Processing PDF: %s", job.event.PDFKey)
	if err := job.msg.InProgress(); err != nil {
		job.appLogger.Warnf("Failed to send InProgress: %v", err)
	}

	if err := job.downloadPDF(ctx); err != nil {
		job.appLogger.Errorf("Download failed: %v", err)
		if nakErr := job.msg.Nak(); nakErr != nil {
			job.appLogger.Errorf("Failed to NAK after download fail: %v", nakErr)
		}
		return
	}

	// Analysis Step
	if err := job.analyzePDF(ctx); err != nil {
		job.appLogger.Errorf("Analysis failed: %v", err)
		if _, pubErr := job.jetStream.Publish(ctx, job.cfg.NATS.DLQSubject, job.msg.Data()); pubErr != nil {
			job.appLogger.Errorf("Failed to publish to DLQ: %v", pubErr)
		}
		if termErr := job.msg.Term(); termErr != nil {
			job.appLogger.Errorf("Failed to Term message: %v", termErr)
		}
		return
	}

	if err := job.processPDF(ctx); err != nil {
		job.appLogger.Errorf("Processing failed: %v", err)
		if _, pubErr := job.jetStream.Publish(ctx, job.cfg.NATS.DLQSubject, job.msg.Data()); pubErr != nil {
			job.appLogger.Errorf("Failed to publish to DLQ: %v", pubErr)
		}
		if termErr := job.msg.Term(); termErr != nil {
			job.appLogger.Errorf("Failed to Term message: %v", termErr)
		}
		return
	}

	if err := job.publishPNGs(ctx); err != nil {
		job.appLogger.Errorf("Publish failed: %v", err)
		if nakErr := job.msg.Nak(); nakErr != nil {
			job.appLogger.Errorf("Failed to NAK after publish fail: %v", nakErr)
		}
		return
	}

	if ackErr := job.msg.Ack(); ackErr != nil {
		job.appLogger.Errorf("Failed to ACK message: %v", ackErr)
	}
	job.appLogger.Successf("Completed: %s", job.event.PDFKey)
}

func (job *job) downloadPDF(ctx context.Context) error {
	obj, err := job.pdfStore.Get(ctx, job.event.PDFKey)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := obj.Close(); closeErr != nil {
			job.appLogger.Warnf("Failed to close object store object: %v", closeErr)
		}
	}()

	job.pdfData, err = io.ReadAll(obj)
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

func (job *job) analyzePDF(ctx context.Context) error {
	job.appLogger.Infof("Analyzing PDF for Narration Directive...")

	// 1. Initialize inputs and parse voice string.
	input := analyzer.AnalysisInput{}
	var voiceID, voiceStyle, voiceTrait string

	if job.event.Settings != nil {
		input.SoundscapePrompt = job.event.Settings.SoundscapePrompt
		input.AugmentationPrompt = job.event.Settings.AugmentationPrompt
		input.Exclusions = job.event.Settings.Exclusions
		if job.event.Settings.Voice != "" {
			voiceID, voiceStyle = parseVoice(job.event.Settings.Voice)

			// Look up the trait from the config, fallback to "unknown"
			if trait, ok := job.cfg.Voices[voiceID]; ok {
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
	job.appLogger.Infof("Parsed voice. ID: '%s', Style: '%s', Trait: '%s'", voiceID, voiceStyle, voiceTrait)

	// 2. Generate Text Directive (Always required for extraction)
	textDirective, err := job.analyzer.GenerateTextDirective(ctx, job.pdfData, input)
	if err != nil {
		return fmt.Errorf("generate text directive: %w", err)
	}

	// 3. Generate Music Configuration (Conditional)
	var musicPrompt string
	var generationConfig *events.LyriaGenerationConfig

	if job.event.Settings.SoundscapePrompt == "" {
		musicPrompt = events.NoSoundscapeDirective
		job.appLogger.Infof("Empty SoundscapePrompt from user. Setting MusicPrompt to '%s'.", musicPrompt)
	} else {
		musicResp, err := job.analyzer.GenerateMusicConfig(ctx, job.pdfData, input)
		if err != nil {
			// RESILIENCE: Music failure is non-fatal for text extraction.
			job.appLogger.Warnf("Music configuration analysis failed: %v. Proceeding without soundscape.", err)
			musicPrompt = events.NoSoundscapeDirective
		} else {
			musicPrompt = musicResp.MusicPrompt
			generationConfig = &events.LyriaGenerationConfig{
				BPM:                 musicResp.GenerationConfig.BPM,
				Density:             musicResp.GenerationConfig.Density,
				Brightness:          musicResp.GenerationConfig.Brightness,
				Guidance:            musicResp.GenerationConfig.Guidance,
				MuteBass:            musicResp.GenerationConfig.MuteBass,
				MuteDrums:           musicResp.GenerationConfig.MuteDrums,
				OnlyBassAndDrums:    musicResp.GenerationConfig.OnlyBassAndDrums,
				MusicGenerationMode: musicResp.GenerationConfig.MusicGenerationMode,
				Scale:               musicResp.GenerationConfig.Scale,
			}
			job.appLogger.Infof("Music Configuration generated successfully.")
		}
	}

	// 4. Create the comprehensive AudioSessionConfig.
	audioConfig := &events.AudioSessionConfig{
		SessionID:        uuid.New().String(),
		SourceDocumentID: job.event.PDFKey,
		VoiceID:          voiceID,
		VoiceStyle:       voiceStyle,
		MusicPrompt:      musicPrompt,
		GenerationConfig: generationConfig,
		TextDirective:    textDirective,
	}

	// 5. Update the event settings with the new config.
	if job.event.Settings == nil {
		job.event.Settings = &events.JobSettings{}
	}
	job.event.Settings.AudioSessionConfig = audioConfig

	return nil
}

func (job *job) processPDF(ctx context.Context) error {
	opts := &pdfrender.Options{
		DPI:                    job.cfg.Service.DPI,
		Workers:                job.cfg.Service.Workers,
		BlankFuzzPercent:       job.cfg.Service.BlankFuzzPercent,
		BlankNonWhiteThreshold: job.cfg.Service.BlankNonWhiteThreshold,
		ProgressBarOutput:      io.Discard, // Simple logs are enough
	}

	// Create processor
	processor := pdfrender.NewProcessor(opts, job.appLogger)

	pngs, err := processor.ProcessSinglePDFFromBytes(ctx, job.pdfData)
	if err != nil {
		return err
	}
	job.pngData = pngs
	return nil
}

func (job *job) publishPNGs(ctx context.Context) error {
	totalPages := len(job.pngData)
	for i, png := range job.pngData {
		pngKey := fmt.Sprintf("%s-%d.png", job.event.PDFKey, i+1)

		if _, err := job.pngStore.PutBytes(ctx, pngKey, png); err != nil {
			return err
		}

		event := events.PNGCreatedEvent{
			Header: events.EventHeader{
				WorkflowID: job.header.WorkflowID,
				UserID:     job.header.UserID,
				TenantID:   job.header.TenantID,
				EventID:    uuid.New().String(),
				Timestamp:  time.Now(),
			},
			PNGKey:     pngKey,
			PageNumber: i + 1,
			TotalPages: totalPages,
			Settings:   job.event.Settings, // Settings now include the AudioSessionConfig
		}

		data, _ := json.Marshal(event)
		if _, err := job.jetStream.Publish(ctx, job.cfg.NATS.Producer.Subject, data); err != nil {
			return err
		}
	}
	return nil
}
