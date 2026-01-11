/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

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
	message       jetstream.Msg
	jetStream     jetstream.JetStream
	pdfStore      jetstream.ObjectStore
	pngStore      jetstream.ObjectStore
	analyzer      *analyzer.Analyzer
	configuration *config.Config
	appLogger     *logger.Logger
	event         *events.PDFCreatedEvent
	header        *events.EventHeader
	pdfData       []byte
	pngData       [][]byte
}

const (
	natsFetchTimeout = 5 * time.Second
	logFileName      = "pdf-to-png-service.log"
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
		os.Exit(1)
	}

	// 2. Setup Bootstrap Logger
	appLogger, loggerError := logger.New(configuration.Service.LogDir, logFileName)
	if loggerError != nil {
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

// run initializes all components and starts the message processing loop.
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

	// 2. Setup NATS
	natsConnection, jetStream, consumer, natsError := setupNATS(parentContext, configuration)
	if natsError != nil {
		return natsError
	}
	defer natsConnection.Close()

	appLogger.Infof(
		"Worker is running, listening for jobs on '%s'",
		configuration.NATS.Consumer.Subject,
	)

	// 3. Start Processing Loop
	return processMessages(parentContext, consumer, jetStream, configuration, analyzerInstance, appLogger)
}

// setupNATS initializes NATS connection and JetStream consumer.
func setupNATS(parentContext context.Context, configuration *config.Config) (*nats.Conn, jetstream.JetStream, jetstream.Consumer, error) {
	natsConnection, connectionError := nats.Connect(configuration.NATS.URL)
	if connectionError != nil {
		return nil, nil, nil, connectionError
	}

	jetStream, jetStreamError := jetstream.New(natsConnection)
	if jetStreamError != nil {
		return nil, nil, nil, jetStreamError
	}

	// Ensure the Consumer stream exists
	_, streamError := jetStream.Stream(parentContext, configuration.NATS.Consumer.Stream)
	if streamError != nil {
		_, createError := jetStream.CreateStream(parentContext, jetstream.StreamConfig{
			Name:     configuration.NATS.Consumer.Stream,
			Subjects: []string{configuration.NATS.Consumer.Stream + ".*"},
			Storage:  jetstream.FileStorage,
		})
		if createError != nil {
			_, retryError := jetStream.Stream(parentContext, configuration.NATS.Consumer.Stream)
			if retryError != nil {
				return nil, nil, nil, createError
			}
		}
	}

	// Ensure the Producer stream exists
	_, streamError = jetStream.Stream(parentContext, configuration.NATS.Producer.Stream)
	if streamError != nil {
		_, createError := jetStream.CreateStream(parentContext, jetstream.StreamConfig{
			Name:     configuration.NATS.Producer.Stream,
			Subjects: []string{configuration.NATS.Producer.Stream + ".*"},
			Storage:  jetstream.FileStorage,
		})
		if createError != nil {
			_, retryError := jetStream.Stream(parentContext, configuration.NATS.Producer.Stream)
			if retryError != nil {
				return nil, nil, nil, createError
			}
		}
	}

	consumer, consumerError := jetStream.CreateOrUpdateConsumer(parentContext, configuration.NATS.Consumer.Stream, jetstream.ConsumerConfig{
		Durable:       configuration.NATS.Consumer.Durable,
		FilterSubject: configuration.NATS.Consumer.Subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
	})
	if consumerError != nil {
		return nil, nil, nil, consumerError
	}

	return natsConnection, jetStream, consumer, nil
}

// processMessages implements the core worker loop.
func processMessages(
	parentContext context.Context,
	consumer jetstream.Consumer,
	jetStream jetstream.JetStream,
	configuration *config.Config,
	analyzer *analyzer.Analyzer,
	appLogger *logger.Logger,
) error {
	pdfStore, pngStore, storeError := getObjectStores(
		parentContext,
		jetStream,
		configuration.NATS.ObjectStore.PDFBucket,
		configuration.NATS.ObjectStore.PNGBucket,
	)
	if storeError != nil {
		return storeError
	}

	for {
		if parentContext.Err() != nil {
			return parentContext.Err()
		}

		batch, fetchError := consumer.Fetch(1, jetstream.FetchMaxWait(natsFetchTimeout))
		if fetchError != nil {
			if errors.Is(fetchError, nats.ErrTimeout) || errors.Is(fetchError, context.DeadlineExceeded) {
				continue
			}
			appLogger.Errorf("Error fetching messages: %v", fetchError)
			time.Sleep(1 * time.Second) // Backoff
			continue
		}

		for message := range batch.Messages() {
			handleMessage(parentContext, message, jetStream, pdfStore, pngStore, analyzer, configuration, appLogger)
		}
	}
}

// getObjectStores retrieves the object stores for PDFs and PNGs, creating them if they don't exist.
func getObjectStores(
	parentContext context.Context,
	jetStream jetstream.JetStream,
	pdfBucket, pngBucket string,
) (pdfStore, pngStore jetstream.ObjectStore, finalError error) {
	var pdfBindError error
	pdfStore, pdfBindError = jetStream.ObjectStore(parentContext, pdfBucket)
	if pdfBindError != nil {
		var createError error
		pdfStore, createError = jetStream.CreateObjectStore(parentContext, jetstream.ObjectStoreConfig{
			Bucket: pdfBucket,
		})
		if createError != nil {
			return nil, nil, createError
		}
	}

	var pngBindError error
	pngStore, pngBindError = jetStream.ObjectStore(parentContext, pngBucket)
	if pngBindError != nil {
		var createError error
		pngStore, createError = jetStream.CreateObjectStore(parentContext, jetstream.ObjectStoreConfig{
			Bucket: pngBucket,
		})
		if createError != nil {
			return nil, nil, createError
		}
	}

	return pdfStore, pngStore, nil
}

// handleMessage processes a single message.
func handleMessage(
	parentContext context.Context, message jetstream.Msg, jetStream jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore, analyzer *analyzer.Analyzer, configuration *config.Config, appLogger *logger.Logger,
) {
	processingJob, jobInitError := newJob(message, jetStream, pdfStore, pngStore, analyzer, configuration, appLogger)
	if jobInitError != nil {
		appLogger.Errorf("Failed to create job: %v", jobInitError)
		if nakError := message.Nak(); nakError != nil {
			appLogger.Errorf("Failed to NAK message: %v", nakError)
		}
		return
	}

	processingJob.run(parentContext)
}

// newJob creates a new job handler.
func newJob(
	message jetstream.Msg,
	jetStream jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore,
	analyzer *analyzer.Analyzer,
	configuration *config.Config,
	appLogger *logger.Logger,
) (*job, error) {
	var event events.PDFCreatedEvent
	if unmarshalError := json.Unmarshal(message.Data(), &event); unmarshalError != nil {
		return nil, unmarshalError
	}

	return &job{
			message:       message,
			jetStream:     jetStream,
			pdfStore:      pdfStore,
			pngStore:      pngStore,
			analyzer:      analyzer,
			configuration: configuration,
			appLogger:     appLogger,
			event:         &event,
			header:        &event.Header,
		},
		nil
}

// executeProcessingSteps runs the core logic of the job.
func (processingJob *job) run(parentContext context.Context) {
	processingJob.appLogger.Infof("Processing PDF: %s", processingJob.event.PDFKey)
	if inProgressError := processingJob.message.InProgress(); inProgressError != nil {
		processingJob.appLogger.Warnf("Failed to send InProgress: %v", inProgressError)
	}

	// Publish PDFProcessingStartedEvent for Bridge Service
	if processingJob.configuration.NATS.Producer.PDFProcessingStartedSubject != "" {
		startedEvent := events.PDFProcessingStartedEvent{
			Header: *processingJob.header,
		}
		data, _ := json.Marshal(startedEvent)
		if _, publishError := processingJob.jetStream.Publish(parentContext, processingJob.configuration.NATS.Producer.PDFProcessingStartedSubject, data); publishError != nil {
			processingJob.appLogger.Warnf("Failed to publish processing started event: %v", publishError)
		}
	}

	if downloadError := processingJob.downloadPDF(parentContext); downloadError != nil {
		processingJob.appLogger.Errorf("Download failed: %v", downloadError)
		if nakError := processingJob.message.Nak(); nakError != nil {
			processingJob.appLogger.Errorf("Failed to NAK after download fail: %v", nakError)
		}
		return
	}

	// Analysis Step
	if analysisError := processingJob.analyzePDF(parentContext); analysisError != nil {
		processingJob.appLogger.Errorf("Analysis failed: %v", analysisError)
		if _, publishError := processingJob.jetStream.Publish(parentContext, processingJob.configuration.NATS.DLQSubject, processingJob.message.Data()); publishError != nil {
			processingJob.appLogger.Errorf("Failed to publish to DLQ: %v", publishError)
		}
		if terminalError := processingJob.message.Term(); terminalError != nil {
			processingJob.appLogger.Errorf("Failed to Term message: %v", terminalError)
		}
		return
	}

	if processingError := processingJob.processPDF(parentContext); processingError != nil {
		processingJob.appLogger.Errorf("Processing failed: %v", processingError)
		if _, publishError := processingJob.jetStream.Publish(parentContext, processingJob.configuration.NATS.DLQSubject, processingJob.message.Data()); publishError != nil {
			processingJob.appLogger.Errorf("Failed to publish to DLQ: %v", publishError)
		}
		if terminalError := processingJob.message.Term(); terminalError != nil {
			processingJob.appLogger.Errorf("Failed to Term message: %v", terminalError)
		}
		return
	}

	if publishError := processingJob.publishPNGs(parentContext); publishError != nil {
		processingJob.appLogger.Errorf("Publish failed: %v", publishError)
		if nakError := processingJob.message.Nak(); nakError != nil {
			processingJob.appLogger.Errorf("Failed to NAK after publish fail: %v", nakError)
		}
		return
	}

	if acknowledgeError := processingJob.message.Ack(); acknowledgeError != nil {
		processingJob.appLogger.Errorf("Failed to ACK message: %v", acknowledgeError)
	}
	processingJob.appLogger.Successf("Completed: %s", processingJob.event.PDFKey)
}

func (processingJob *job) downloadPDF(parentContext context.Context) error {
	object, getError := processingJob.pdfStore.Get(parentContext, processingJob.event.PDFKey)
	if getError != nil {
		return getError
	}
	defer func() {
		if closeError := object.Close(); closeError != nil {
			processingJob.appLogger.Warnf("Failed to close object store object: %v", closeError)
		}
	}()

	var readError error
	processingJob.pdfData, readError = io.ReadAll(object)
	return readError
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

func (processingJob *job) analyzePDF(parentContext context.Context) error {
	processingJob.appLogger.Infof("Analyzing PDF for Narration Directive...")

	// 1. Initialize inputs and parse voice string.
	input := analyzer.AnalysisInput{}
	var voiceID, voiceStyle, voiceTrait string

	if processingJob.event.Settings != nil {
		input.SoundscapePrompt = processingJob.event.Settings.SoundscapePrompt
		input.AugmentationPrompt = processingJob.event.Settings.AugmentationPrompt
		input.Exclusions = processingJob.event.Settings.Exclusions
		if processingJob.event.Settings.Voice != "" {
			voiceID, voiceStyle = parseVoice(processingJob.event.Settings.Voice)

			// Look up the trait from the config, fallback to "unknown"
			if trait, ok := processingJob.configuration.Voices[voiceID]; ok {
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
	processingJob.appLogger.Infof("Parsed voice. ID: '%s', Style: '%s', Trait: '%s'", voiceID, voiceStyle, voiceTrait)

	// 2. Generate Text Directive (Always required for extraction)
	textDirective, textDirectiveError := processingJob.analyzer.GenerateTextDirective(parentContext, processingJob.pdfData, input)
	if textDirectiveError != nil {
		return textDirectiveError
	}

	// 3. Generate Music Configuration (Conditional)
	var musicPrompt string
	var generationConfig *events.LyriaGenerationConfig

	if processingJob.event.Settings.SoundscapePrompt == "" {
		musicPrompt = events.NoSoundscapeDirective
		processingJob.appLogger.Infof("Empty SoundscapePrompt from user. Setting MusicPrompt to '%s'.", musicPrompt)
	} else {
		musicResponse, musicAnalysisError := processingJob.analyzer.GenerateMusicConfig(parentContext, processingJob.pdfData, input)
		if musicAnalysisError != nil {
			// RESILIENCE: Music failure is non-fatal for text extraction.
			processingJob.appLogger.Warnf("Music configuration analysis failed: %v. Proceeding without soundscape.", musicAnalysisError)
			musicPrompt = events.NoSoundscapeDirective
		} else {
			musicPrompt = musicResponse.MusicPrompt
			generationConfig = &musicResponse.GenerationConfig
			processingJob.appLogger.Infof("Music Configuration generated successfully.")
		}
	}

	// 4. Create the comprehensive AudioSessionConfig.
	audioConfig := &events.AudioSessionConfig{
		SessionID:        uuid.New().String(),
		SourceDocumentID: processingJob.event.PDFKey,
		VoiceID:          voiceID,
		VoiceStyle:       voiceStyle,
		MusicPrompt:      musicPrompt,
		GenerationConfig: generationConfig,
		TextDirective:    textDirective,
	}

	// 5. Update the event settings with the new config.
	if processingJob.event.Settings == nil {
		processingJob.event.Settings = &events.JobSettings{}
	}
	processingJob.event.Settings.AudioSessionConfig = audioConfig

	return nil
}

func (processingJob *job) processPDF(parentContext context.Context) error {
	options := &pdfrender.Options{
		DPI:                    processingJob.configuration.Service.DPI,
		Workers:                processingJob.configuration.Service.Workers,
		BlankFuzzPercent:       processingJob.configuration.Service.BlankFuzzPercent,
		BlankNonWhiteThreshold: processingJob.configuration.Service.BlankNonWhiteThreshold,
		ProgressBarOutput:      io.Discard, // Simple logs are enough
	}

	// Create processor
	processor := pdfrender.NewProcessor(options, processingJob.appLogger)

	pngs, renderError := processor.ProcessSinglePDFFromBytes(parentContext, processingJob.pdfData)
	if renderError != nil {
		return renderError
	}
	processingJob.pngData = pngs
	return nil
}

func (processingJob *job) publishPNGs(parentContext context.Context) error {
	totalPages := len(processingJob.pngData)
	for index, png := range processingJob.pngData {
		pngKey := fmt.Sprintf("%s-%d.png", processingJob.event.PDFKey, index+1)

		if _, uploadError := processingJob.pngStore.PutBytes(parentContext, pngKey, png); uploadError != nil {
			return uploadError
		}

		event := events.PNGCreatedEvent{
			Header: events.EventHeader{
				WorkflowID: processingJob.header.WorkflowID,
				UserID:     processingJob.header.UserID,
				TenantID:   processingJob.header.TenantID,
				EventID:    uuid.New().String(),
				Timestamp:  time.Now(),
			},
			PNGKey:     pngKey,
			PageNumber: index + 1,
			TotalPages: totalPages,
			Settings:   processingJob.event.Settings, // Settings now include the AudioSessionConfig
		}

		data, _ := json.Marshal(event)
		if _, publishError := processingJob.jetStream.Publish(parentContext, processingJob.configuration.NATS.Producer.Subject, data); publishError != nil {
			return publishError
		}
	}
	return nil
}
