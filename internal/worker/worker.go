/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/book-expert/common-events"
	"github.com/book-expert/common-worker"
	"github.com/book-expert/logger"
	"github.com/book-expert/pdf-to-png-service/internal/analyzer"
	"github.com/book-expert/pdf-to-png-service/internal/config"
	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// Worker coordinates the conversion of PDF documents into PNG images and narration directives.
type Worker struct {
	baseWorker      *worker.Worker[*events.PDFCreatedEvent]
	jetStream       jetstream.JetStream
	pdfStore        jetstream.ObjectStore
	pngStore        jetstream.ObjectStore
	analyzer        *analyzer.Analyzer
	configuration   *config.Config
	logger          *logger.Logger
	producerSubject string
	startedSubject  string
	deadLetterQueue string
}

// New creates a new Worker instance using the common-worker library.
func New(
	jetStream jetstream.JetStream,
	pdfStore jetstream.ObjectStore,
	pngStore jetstream.ObjectStore,
	analyzerInstance *analyzer.Analyzer,
	configuration *config.Config,
	serviceLogger *logger.Logger,
	producerSubject string,
	startedSubject string,
	deadLetterQueue string,
	workerCount int,
) *Worker {
	pdfWorker := &Worker{
		jetStream:       jetStream,
		pdfStore:        pdfStore,
		pngStore:        pngStore,
		analyzer:        analyzerInstance,
		configuration:   configuration,
		logger:          serviceLogger,
		producerSubject: producerSubject,
		startedSubject:  startedSubject,
		deadLetterQueue: deadLetterQueue,
	}

	workerConfig := worker.Config{
		StreamName:    configuration.NATS.Consumer.Stream,
		ConsumerName:  configuration.NATS.Consumer.Durable,
		FilterSubject: configuration.NATS.Consumer.Subject,
		WorkerCount:   workerCount,
		MaxDeliver:    3,
	}

	pdfWorker.baseWorker = worker.New(jetStream, serviceLogger, workerConfig, pdfWorker.handleMessage)
	return pdfWorker
}

// Start initiates the parallel consumption of PDF processing requests.
func (pdfWorker *Worker) Start(parentContext context.Context) error {
	return pdfWorker.baseWorker.Start(parentContext)
}

func (pdfWorker *Worker) handleMessage(parentContext context.Context, event *events.PDFCreatedEvent, message jetstream.Msg) error {
	pdfWorker.logger.Infof("Processing PDF: %s", event.PDFKey)

	// Publish PDFProcessingStartedEvent for Bridge Service
	if pdfWorker.startedSubject != "" {
		startedEvent := events.PDFProcessingStartedEvent{
			Header: event.Header,
		}
		data, _ := json.Marshal(startedEvent)
		if _, publishError := pdfWorker.jetStream.Publish(parentContext, pdfWorker.startedSubject, data); publishError != nil {
			pdfWorker.logger.Warnf("Failed to publish processing started event: %v", publishError)
		}
	}

	if workflowError := pdfWorker.executeWorkflow(parentContext, event); workflowError != nil {
		pdfWorker.logger.Errorf("Workflow failed for %s: %v", event.PDFKey, workflowError)

		// If move to DLQ fails, we return error to let common-worker Nak
		if dlqError := pdfWorker.moveToDeadLetterQueue(parentContext, message); dlqError != nil {
			pdfWorker.logger.Errorf("Failed to move to DLQ: %v", dlqError)
		} else {
			// Successfully moved to DLQ, we can Ack the original message
			_ = message.Ack()
			return nil
		}

		return workflowError
	}

	pdfWorker.logger.Successf("Completed: %s", event.PDFKey)
	return nil
}

func (pdfWorker *Worker) moveToDeadLetterQueue(parentContext context.Context, message jetstream.Msg) error {
	if pdfWorker.deadLetterQueue == "" {
		return nil
	}
	_, publishError := pdfWorker.jetStream.Publish(parentContext, pdfWorker.deadLetterQueue, message.Data())
	return publishError
}

func (pdfWorker *Worker) executeWorkflow(parentContext context.Context, event *events.PDFCreatedEvent) error {
	// 1. Download
	pdfData, downloadError := pdfWorker.downloadPDF(parentContext, event.PDFKey)
	if downloadError != nil {
		return fmt.Errorf("download failed: %w", downloadError)
	}

	// 2. Analysis Step
	if analysisError := pdfWorker.analyzePDF(parentContext, event, pdfData); analysisError != nil {
		return fmt.Errorf("analysis failed: %w", analysisError)
	}

	// 3. Processing Step (Render to PNGs)
	pngData, processingError := pdfWorker.processPDF(parentContext, pdfData)
	if processingError != nil {
		return fmt.Errorf("processing failed: %w", processingError)
	}

	// 4. Publish Step
	if publishError := pdfWorker.publishPNGs(parentContext, event, pngData); publishError != nil {
		return fmt.Errorf("publish failed: %w", publishError)
	}

	return nil
}

func (pdfWorker *Worker) downloadPDF(parentContext context.Context, pdfKey string) ([]byte, error) {
	object, getError := pdfWorker.pdfStore.Get(parentContext, pdfKey)
	if getError != nil {
		return nil, getError
	}
	defer func() {
		if closeError := object.Close(); closeError != nil {
			pdfWorker.logger.Warnf("Failed to close object store handle: %v", closeError)
		}
	}()

	return io.ReadAll(object)
}

func (pdfWorker *Worker) analyzePDF(parentContext context.Context, event *events.PDFCreatedEvent, pdfData []byte) error {
	pdfWorker.logger.Infof("Analyzing PDF for Narration Directive...")

	input := analyzer.AnalysisInput{}
	var voiceID, voiceStyle, voiceTrait string

	if event.Settings != nil {
		input.SoundscapePrompt = event.Settings.SoundscapePrompt
		input.AugmentationPrompt = event.Settings.AugmentationPrompt
		input.Exclusions = event.Settings.Exclusions
		if event.Settings.Voice != "" {
			voiceID, voiceStyle = pdfWorker.parseVoice(event.Settings.Voice)

			if trait, ok := pdfWorker.configuration.Voices[voiceID]; ok {
				voiceTrait = trait
			} else {
				voiceTrait = "unknown"
			}

			if voiceStyle == "" {
				voiceStyle = voiceTrait
			}

			input.VoiceName = voiceID
			input.VoiceStyle = voiceStyle
			input.VoiceTrait = voiceTrait
		}
	}

	textDirective, textDirectiveError := pdfWorker.analyzer.GenerateTextDirective(parentContext, pdfData, input)
	if textDirectiveError != nil {
		return textDirectiveError
	}

	var musicPrompt string
	var generationConfig *events.LyriaGenerationConfig

	if event.Settings.SoundscapePrompt == "" {
		musicPrompt = events.NoSoundscapeDirective
	} else {
		musicResponse, musicAnalysisError := pdfWorker.analyzer.GenerateMusicConfig(parentContext, pdfData, input)
		if musicAnalysisError != nil {
			pdfWorker.logger.Warnf("Music configuration analysis failed: %v. Proceeding without soundscape.", musicAnalysisError)
			musicPrompt = events.NoSoundscapeDirective
		} else {
			musicPrompt = musicResponse.MusicPrompt
			generationConfig = &musicResponse.GenerationConfig
		}
	}

	audioConfig := &events.AudioSessionConfig{
		SessionID:        uuid.New().String(),
		SourceDocumentID: event.PDFKey,
		VoiceID:          voiceID,
		VoiceStyle:       voiceStyle,
		MusicPrompt:      musicPrompt,
		GenerationConfig: generationConfig,
		TextDirective:    textDirective,
	}

	if event.Settings == nil {
		event.Settings = &events.JobSettings{}
	}
	event.Settings.AudioSessionConfig = audioConfig

	return nil
}

func (pdfWorker *Worker) parseVoice(voice string) (voiceID, voiceStyle string) {
	parts := strings.SplitN(voice, "-", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(voice), ""
}

func (pdfWorker *Worker) processPDF(parentContext context.Context, pdfData []byte) ([][]byte, error) {
	options := &pdfrender.Options{
		DPI:                    pdfWorker.configuration.Service.DPI,
		Workers:                pdfWorker.configuration.Service.Workers,
		BlankFuzzPercent:       pdfWorker.configuration.Service.BlankFuzzPercent,
		BlankNonWhiteThreshold: pdfWorker.configuration.Service.BlankNonWhiteThreshold,
		ProgressBarOutput:      io.Discard,
	}

	processor := pdfrender.NewProcessor(options, pdfWorker.logger)
	return processor.ProcessSinglePDFFromBytes(parentContext, pdfData)
}

func (pdfWorker *Worker) publishPNGs(parentContext context.Context, event *events.PDFCreatedEvent, pngData [][]byte) error {
	totalPages := len(pngData)
	for index, pngContent := range pngData {
		pngKey := fmt.Sprintf("%s-%d.png", event.PDFKey, index+1)

		if _, uploadError := pdfWorker.pngStore.PutBytes(parentContext, pngKey, pngContent); uploadError != nil {
			return uploadError
		}

		createdEvent := events.PNGCreatedEvent{
			Header: events.EventHeader{
				WorkflowID: event.Header.WorkflowID,
				UserID:     event.Header.UserID,
				TenantID:   event.Header.TenantID,
				EventID:    uuid.New().String(),
				Timestamp:  time.Now(),
			},
			PNGKey:     pngKey,
			PageNumber: index + 1,
			TotalPages: totalPages,
			Settings:   event.Settings,
		}

		eventData, _ := json.Marshal(createdEvent)
		if _, publishError := pdfWorker.jetStream.Publish(parentContext, pdfWorker.producerSubject, eventData); publishError != nil {
			return publishError
		}
	}
	return nil
}
