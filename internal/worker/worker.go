// DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS

/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/book-expert/common-events"
	"github.com/book-expert/common-worker"
	"github.com/book-expert/logger"
	"github.com/book-expert/pdf-to-png-service/internal/config"
	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// MessageProcessingTimeout defines the maximum duration allowed for processing a single PDF rendering job.
	MessageProcessingTimeout = 600 * time.Second
)

// JetStreamPublisher defines the interface for publishing messages to JetStream.
type JetStreamPublisher interface {
	Publish(requestContext context.Context, subject string, data []byte, options ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// Worker manages the lifecycle of processing PDF-to-PNG conversion requests from NATS.
type Worker struct {
	baseWorker         *worker.Worker[*events.PdfCreatedEvent]
	jetStreamPublisher JetStreamPublisher
	producerSubject    string
	pdfStore           jetstream.ObjectStore
	pngStore           jetstream.ObjectStore
	renderer           *pdfrender.Processor
	logger             *logger.Logger
	configuration      *config.Config
}

// New initializes a new Worker with all necessary dependencies.
func New(
	natsConnection *nats.Conn,
	jetStreamContext jetstream.JetStream,
	jetStreamPublisher JetStreamPublisher,
	subscriptionStream string,
	subscriptionSubject string,
	consumerDurableName string,
	producerSubject string,
	pdfStore jetstream.ObjectStore,
	pngStore jetstream.ObjectStore,
	renderer *pdfrender.Processor,
	serviceLogger *logger.Logger,
	configuration *config.Config,
) (*Worker, error) {
	pdfWorker := &Worker{
		jetStreamPublisher: jetStreamPublisher,
		producerSubject:    producerSubject,
		pdfStore:           pdfStore,
		pngStore:           pngStore,
		renderer:           renderer,
		logger:             serviceLogger,
		configuration:      configuration,
	}

	workerConfiguration := worker.Config{
		StreamName:    subscriptionStream,
		ConsumerName:  consumerDurableName,
		FilterSubject: subscriptionSubject,
		WorkerCount:   configuration.Service.Workers,
		MaxDeliver:    5,
	}

	pdfWorker.baseWorker = worker.New(natsConnection, jetStreamContext, serviceLogger, workerConfiguration, pdfWorker.handleMessage)
	return pdfWorker, nil
}

// Run executes the main worker loop.
func (pdfWorker *Worker) Run(systemContext context.Context) error {
	return pdfWorker.baseWorker.Start(systemContext)
}

func (pdfWorker *Worker) handleMessage(requestContext context.Context, event *events.PdfCreatedEvent, message jetstream.Msg) error {
	parentContext, cancelProcessing := context.WithTimeout(requestContext, MessageProcessingTimeout)
	defer cancelProcessing()

	pdfWorker.logger.Infof("Processing PDF: %s", event.PdfKey)

	if workflowError := pdfWorker.executeWorkflow(parentContext, event); workflowError != nil {
		pdfWorker.logger.Errorf("Workflow failed for %s: %v", event.PdfKey, workflowError)
		// Nak with delay to allow retry
		_ = message.NakWithDelay(10 * time.Second)
		return workflowError
	}

	pdfWorker.logger.Successf("Completed: %s", event.PdfKey)
	return nil
}

func (pdfWorker *Worker) executeWorkflow(parentContext context.Context, event *events.PdfCreatedEvent) error {
	// 1. Download PDF
	pdfData, downloadError := pdfWorker.downloadPDF(parentContext, event.PdfKey)
	if downloadError != nil {
		return fmt.Errorf("download failed: %w", downloadError)
	}

	// 2. Render PNGs
	pages, renderError := pdfWorker.renderer.ProcessSinglePDFFromBytes(parentContext, pdfData)
	if renderError != nil {
		return fmt.Errorf("render failed: %w", renderError)
	}

	// 3. Process Job Settings (Extract Audio Session Config if not present)
	// If settings are present, we propagate them. We also inject the parsed voice config.
	if event.Settings != nil && event.Settings.Voice != "" && (event.Settings.AudioSessionConfig == nil || event.Settings.AudioSessionConfig.VoiceIdentifier == "") {
		voiceIdentifier, voiceStyle := pdfWorker.parseVoice(event.Settings.Voice)
		var voiceTrait string
		if trait, ok := pdfWorker.configuration.Voices[voiceIdentifier]; ok {
			voiceTrait = trait
		}

		if event.Settings.AudioSessionConfig == nil {
			event.Settings.AudioSessionConfig = &events.AudioSessionConfig{}
		}

		event.Settings.AudioSessionConfig.SessionIdentifier = uuid.New().String()
		event.Settings.AudioSessionConfig.SourceDocumentIdentifier = event.PdfKey
		event.Settings.AudioSessionConfig.VoiceIdentifier = voiceIdentifier
		event.Settings.AudioSessionConfig.VoiceStyle = voiceStyle
		// Injected trait if we have one locally, otherwise UI provides it
		if event.Settings.AudioSessionConfig.TextDirective == "" {
			event.Settings.AudioSessionConfig.TextDirective = voiceTrait
		}
	}

	// 4. Upload each page and publish event
	for index, pngContent := range pages {
		pngKey := fmt.Sprintf("%s-%d.png", event.PdfKey, index+1)
		if _, uploadError := pdfWorker.pngStore.PutBytes(parentContext, pngKey, pngContent); uploadError != nil {
			return fmt.Errorf("upload page %d failed: %w", index+1, uploadError)
		}

		completionEvent := events.PngCreatedEvent{
			Header: events.EventHeader{
				WorkflowIdentifier: event.Header.WorkflowIdentifier,
				UserIdentifier:     event.Header.UserIdentifier,
				TenantIdentifier:   event.Header.TenantIdentifier,
				EventIdentifier:    uuid.New().String(),
				Timestamp:          time.Now().UTC(),
			},
			PngKey:     pngKey,
			PageNumber: index + 1,
			TotalPages: len(pages),
			Settings:   event.Settings,
		}

		data, _ := json.Marshal(completionEvent)
		if _, publishError := pdfWorker.jetStreamPublisher.Publish(parentContext, pdfWorker.producerSubject, data); publishError != nil {
			return fmt.Errorf("publish page %d failed: %w", index+1, publishError)
		}
	}

	return nil
}

func (pdfWorker *Worker) downloadPDF(parentContext context.Context, pdfKey string) ([]byte, error) {
	object, getError := pdfWorker.pdfStore.Get(parentContext, pdfKey)
	if getError != nil {
		return nil, getError
	}
	defer func() {
		_ = object.Close()
	}()

	info, _ := object.Info()
	data := make([]byte, info.Size)
	_, readError := object.Read(data)
	return data, readError
}

func (pdfWorker *Worker) parseVoice(voice string) (voiceIdentifier, voiceStyle string) {
	// Simple parser: "Voice Name (Style Description)"
	// Example: "Niko (Calm, mature)" -> voiceIdentifier="Niko", voiceStyle="Calm, mature"
	if voice == "" {
		return "", ""
	}

	// Find the style part in parentheses
	start := -1
	end := -1
	for index, character := range voice {
		if character == '(' {
			start = index
		} else if character == ')' {
			end = index
		}
	}

	if start != -1 && end != -1 && end > start {
		voiceIdentifier = strings.TrimSpace(voice[:start])
		voiceStyle = strings.TrimSpace(voice[start+1 : end])
		return voiceIdentifier, voiceStyle
	}

	// No style found
	return strings.TrimSpace(voice), ""
}
