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

	"github.com/book-expert/common-events"
	"github.com/book-expert/logger"
	"github.com/book-expert/pdf-to-png-service/internal/analyzer"
	"github.com/book-expert/pdf-to-png-service/internal/config"
	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// processingJob represents the context for processing a single message.
type processingJob struct {
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
	natsConnection, jetStreamContext, consumer, natsSetupError := setupNATS(parentContext, configuration)
	if natsSetupError != nil {
		return natsSetupError
	}
	defer natsConnection.Close()

	appLogger.Infof(
		"Worker is running, listening for jobs on '%s'",
		configuration.NATS.Consumer.Subject,
	)

	// 3. Start Processing Loop
	return processMessages(parentContext, consumer, jetStreamContext, configuration, analyzerInstance, appLogger)
}

// setupNATS initializes NATS connection and JetStream consumer.
func setupNATS(parentContext context.Context, configuration *config.Config) (*nats.Conn, jetstream.JetStream, jetstream.Consumer, error) {
	natsConnection, connectionError := nats.Connect(configuration.NATS.URL)
	if connectionError != nil {
		return nil, nil, nil, connectionError
	}

	jetStreamContext, jetStreamError := jetstream.New(natsConnection)
	if jetStreamError != nil {
		return nil, nil, nil, jetStreamError
	}

	// Ensure the Consumer stream exists
	_, streamLookupError := jetStreamContext.Stream(parentContext, configuration.NATS.Consumer.Stream)
	if streamLookupError != nil {
		_, streamCreationError := jetStreamContext.CreateStream(parentContext, jetstream.StreamConfig{
			Name:     configuration.NATS.Consumer.Stream,
			Subjects: events.GetStreamSubjects(configuration.NATS.Consumer.Stream),
			Storage:  jetstream.FileStorage,
		})
		if streamCreationError != nil {
			_, retryStreamError := jetStreamContext.Stream(parentContext, configuration.NATS.Consumer.Stream)
			if retryStreamError != nil {
				return nil, nil, nil, streamCreationError
			}
		}
	}

	// Ensure the Producer stream exists
	_, producerLookupError := jetStreamContext.Stream(parentContext, configuration.NATS.Producer.Stream)
	if producerLookupError != nil {
		_, producerCreationError := jetStreamContext.CreateStream(parentContext, jetstream.StreamConfig{
			Name:     configuration.NATS.Producer.Stream,
			Subjects: events.GetStreamSubjects(configuration.NATS.Producer.Stream),
			Storage:  jetstream.FileStorage,
		})
		if producerCreationError != nil {
			_, retryProducerError := jetStreamContext.Stream(parentContext, configuration.NATS.Producer.Stream)
			if retryProducerError != nil {
				return nil, nil, nil, producerCreationError
			}
		}
	}

	consumer, consumerError := jetStreamContext.CreateOrUpdateConsumer(parentContext, configuration.NATS.Consumer.Stream, jetstream.ConsumerConfig{
		Durable:       configuration.NATS.Consumer.Durable,
		FilterSubject: configuration.NATS.Consumer.Subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
	})
	if consumerError != nil {
		return nil, nil, nil, consumerError
	}

	return natsConnection, jetStreamContext, consumer, nil
}

// processMessages implements the core worker loop.
func processMessages(
	parentContext context.Context,
	consumer jetstream.Consumer,
	jetStreamContext jetstream.JetStream,
	configuration *config.Config,
	analyzerInstance *analyzer.Analyzer,
	appLogger *logger.Logger,
) error {
	pdfStore, pngStore, storeError := getObjectStores(
		parentContext,
		jetStreamContext,
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
			handleMessage(parentContext, message, jetStreamContext, pdfStore, pngStore, analyzerInstance, configuration, appLogger)
		}
	}
}

// getObjectStores retrieves the object stores for PDFs and PNGs, creating them if they don't exist.
func getObjectStores(
	parentContext context.Context,
	jetStreamContext jetstream.JetStream,
	pdfBucket, pngBucket string,
) (pdfStore, pngStore jetstream.ObjectStore, finalError error) {
	var pdfBindError error
	pdfStore, pdfBindError = jetStreamContext.ObjectStore(parentContext, pdfBucket)
	if pdfBindError != nil {
		var createError error
		pdfStore, createError = jetStreamContext.CreateObjectStore(parentContext, jetstream.ObjectStoreConfig{
			Bucket: pdfBucket,
		})
		if createError != nil {
			return nil, nil, createError
		}
	}

	var pngBindError error
	pngStore, pngBindError = jetStreamContext.ObjectStore(parentContext, pngBucket)
	if pngBindError != nil {
		var createError error
		pngStore, createError = jetStreamContext.CreateObjectStore(parentContext, jetstream.ObjectStoreConfig{
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
	parentContext context.Context, jetStreamMessage jetstream.Msg, jetStreamContext jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore, analyzerInstance *analyzer.Analyzer, configuration *config.Config, appLogger *logger.Logger,
) {
	currentJob, jobInitError := newJob(jetStreamMessage, jetStreamContext, pdfStore, pngStore, analyzerInstance, configuration, appLogger)
	if jobInitError != nil {
		appLogger.Errorf("Failed to create job: %v", jobInitError)
		if nakError := jetStreamMessage.Nak(); nakError != nil {
			appLogger.Errorf("Failed to NAK message: %v", nakError)
		}
		return
	}

	currentJob.run(parentContext)
}

// newJob creates a new job handler.
func newJob(
	jetStreamMessage jetstream.Msg,
	jetStreamContext jetstream.JetStream,
	pdfStore, pngStore jetstream.ObjectStore,
	analyzerInstance *analyzer.Analyzer,
	configuration *config.Config,
	appLogger *logger.Logger,
) (*processingJob, error) {
	var event events.PDFCreatedEvent
	if unmarshalError := json.Unmarshal(jetStreamMessage.Data(), &event); unmarshalError != nil {
		return nil, unmarshalError
	}

	return &processingJob{
			message:       jetStreamMessage,
			jetStream:     jetStreamContext,
			pdfStore:      pdfStore,
			pngStore:      pngStore,
			analyzer:      analyzerInstance,
			configuration: configuration,
			appLogger:     appLogger,
			event:         &event,
			header:        &event.Header,
		},
		nil
}

// executeProcessingSteps runs the core logic of the job.
func (job *processingJob) run(parentContext context.Context) {
	job.appLogger.Infof("Processing PDF: %s", job.event.PDFKey)
	if inProgressError := job.message.InProgress(); inProgressError != nil {
		job.appLogger.Warnf("Failed to send InProgress: %v", inProgressError)
	}

	// Publish PDFProcessingStartedEvent for Bridge Service
	if job.configuration.NATS.Producer.PDFProcessingStartedSubject != "" {
		startedEvent := events.PDFProcessingStartedEvent{
			Header: *job.header,
		}
		data, _ := json.Marshal(startedEvent)
		if _, publishError := job.jetStream.Publish(parentContext, job.configuration.NATS.Producer.PDFProcessingStartedSubject, data); publishError != nil {
			job.appLogger.Warnf("Failed to publish processing started event: %v", publishError)
		}
	}

	if downloadError := job.downloadPDF(parentContext); downloadError != nil {
		job.appLogger.Errorf("Download failed: %v", downloadError)
		if nakError := job.message.Nak(); nakError != nil {
			job.appLogger.Errorf("Failed to NAK after download fail: %v", nakError)
		}
		return
	}

	// Analysis Step
	if analysisError := job.analyzePDF(parentContext); analysisError != nil {
		job.appLogger.Errorf("Analysis failed: %v", analysisError)
		if _, publishError := job.jetStream.Publish(parentContext, job.configuration.NATS.DLQSubject, job.message.Data()); publishError != nil {
			job.appLogger.Errorf("Failed to publish to DLQ: %v", publishError)
		}
		if terminalError := job.message.Term(); terminalError != nil {
			job.appLogger.Errorf("Failed to Term message: %v", terminalError)
		}
		return
	}

	if processingError := job.processPDF(parentContext); processingError != nil {
		job.appLogger.Errorf("Processing failed: %v", processingError)
		if _, publishError := job.jetStream.Publish(parentContext, job.configuration.NATS.DLQSubject, job.message.Data()); publishError != nil {
			job.appLogger.Errorf("Failed to publish to DLQ: %v", publishError)
		}
		if terminalError := job.message.Term(); terminalError != nil {
			job.appLogger.Errorf("Failed to Term message: %v", terminalError)
		}
		return
	}

	if publishError := job.publishPNGs(parentContext); publishError != nil {
		job.appLogger.Errorf("Publish failed: %v", publishError)
		if nakError := job.message.Nak(); nakError != nil {
			job.appLogger.Errorf("Failed to NAK after publish fail: %v", nakError)
		}
		return
	}

	if acknowledgeError := job.message.Ack(); acknowledgeError != nil {
		job.appLogger.Errorf("Failed to ACK message: %v", acknowledgeError)
	}
	job.appLogger.Successf("Completed: %s", job.event.PDFKey)
}

func (job *processingJob) downloadPDF(parentContext context.Context) error {
	object, getError := job.pdfStore.Get(parentContext, job.event.PDFKey)
	if getError != nil {
		return getError
	}
	defer func() {
		if closeError := object.Close(); closeError != nil {
			job.appLogger.Warnf("Failed to close object store object: %v", closeError)
		}
	}()

	var readError error
	job.pdfData, readError = io.ReadAll(object)
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

func (job *processingJob) analyzePDF(parentContext context.Context) error {
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
			if trait, ok := job.configuration.Voices[voiceID]; ok {
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
	textDirective, textDirectiveError := job.analyzer.GenerateTextDirective(parentContext, job.pdfData, input)
	if textDirectiveError != nil {
		return textDirectiveError
	}

	// 3. Generate Music Configuration (Conditional)
	var musicPrompt string
	var generationConfig *events.LyriaGenerationConfig

	if job.event.Settings.SoundscapePrompt == "" {
		musicPrompt = events.NoSoundscapeDirective
		job.appLogger.Infof("Empty SoundscapePrompt from user. Setting MusicPrompt to '%s'.", musicPrompt)
	} else {
		musicResponse, musicAnalysisError := job.analyzer.GenerateMusicConfig(parentContext, job.pdfData, input)
		if musicAnalysisError != nil {
			// RESILIENCE: Music failure is non-fatal for text extraction.
			job.appLogger.Warnf("Music configuration analysis failed: %v. Proceeding without soundscape.", musicAnalysisError)
			musicPrompt = events.NoSoundscapeDirective
		} else {
			musicPrompt = musicResponse.MusicPrompt
			generationConfig = &musicResponse.GenerationConfig
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

func (job *processingJob) processPDF(parentContext context.Context) error {
	options := &pdfrender.Options{
		DPI:                    job.configuration.Service.DPI,
		Workers:                job.configuration.Service.Workers,
		BlankFuzzPercent:       job.configuration.Service.BlankFuzzPercent,
		BlankNonWhiteThreshold: job.configuration.Service.BlankNonWhiteThreshold,
		ProgressBarOutput:      io.Discard, // Simple logs are enough
	}

	// Create processor
	processor := pdfrender.NewProcessor(options, job.appLogger)

	renderedPNGs, renderError := processor.ProcessSinglePDFFromBytes(parentContext, job.pdfData)
	if renderError != nil {
		return renderError
	}
	job.pngData = renderedPNGs
	return nil
}

func (job *processingJob) publishPNGs(parentContext context.Context) error {
	totalPages := len(job.pngData)
	for index, pngContent := range job.pngData {
		pngKey := fmt.Sprintf("%s-%d.png", job.event.PDFKey, index+1)

		if _, uploadError := job.pngStore.PutBytes(parentContext, pngKey, pngContent); uploadError != nil {
			return uploadError
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
			PageNumber: index + 1,
			TotalPages: totalPages,
			Settings:   job.event.Settings, // Settings now include the AudioSessionConfig
		}

		eventData, _ := json.Marshal(event)
		if _, publishError := job.jetStream.Publish(parentContext, job.configuration.NATS.Producer.Subject, eventData); publishError != nil {
			return publishError
		}
	}
	return nil
}
