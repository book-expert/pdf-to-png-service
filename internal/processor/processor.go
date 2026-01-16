/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/book-expert/common-events"
	worker "github.com/book-expert/common-worker"
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

// Processor coordinates the conversion of PDF documents into PNG image sequences.
type Processor struct {
	engine             *worker.Worker[*events.PdfCreatedEvent]
	jetStreamPublisher JetStreamPublisher
	producerSubject    string
	pdfObjectStore     jetstream.ObjectStore
	pngObjectStore     jetstream.ObjectStore
	pdfRenderer        *pdfrender.Processor
	serviceLogger      *logger.Logger
	configuration      *config.Config
}

// NewProcessor initializes a new Processor with all necessary dependencies.
func NewProcessor(
	natsConnection *nats.Conn,
	jetStreamContext jetstream.JetStream,
	jetStreamPublisher JetStreamPublisher,
	subscriptionStream string,
	subscriptionSubject string,
	consumerDurableName string,
	producerSubject string,
	pdfObjectStore jetstream.ObjectStore,
	pngObjectStore jetstream.ObjectStore,
	pdfRenderer *pdfrender.Processor,
	serviceLogger *logger.Logger,
	configuration *config.Config,
) (*Processor, error) {
	pdfProcessor := &Processor{
		jetStreamPublisher: jetStreamPublisher,
		producerSubject:    producerSubject,
		pdfObjectStore:     pdfObjectStore,
		pngObjectStore:     pngObjectStore,
		pdfRenderer:        pdfRenderer,
		serviceLogger:      serviceLogger,
		configuration:      configuration,
	}

	workerConfiguration := worker.Config{
		StreamName:    subscriptionStream,
		ConsumerName:  consumerDurableName,
		FilterSubject: subscriptionSubject,
		WorkerCount:   configuration.Service.Workers,
		MaxDeliver:    5,
	}

	pdfProcessor.engine = worker.New(natsConnection, jetStreamContext, serviceLogger, workerConfiguration, pdfProcessor.handleMessage)
	return pdfProcessor, nil
}

// Start executes the underlying processor engine.
func (processor *Processor) Start(systemContext context.Context) error {
	return processor.engine.Start(systemContext)
}

func (processor *Processor) handleMessage(requestContext context.Context, event *events.PdfCreatedEvent, message jetstream.Msg) error {
	parentContext, cancelProcessing := context.WithTimeout(requestContext, MessageProcessingTimeout)
	defer cancelProcessing()

	processor.serviceLogger.Infof("Processing PDF: %s", event.PdfKey)

	// Signal PDF Started
	processor.publishSimpleLifecycleEvent(parentContext, event.Header, events.SubjectPdfStarted)

	if workflowExecutionError := processor.executeWorkflow(parentContext, event); workflowExecutionError != nil {
		processor.serviceLogger.Errorf("Workflow failed for %s: %v", event.PdfKey, workflowExecutionError)
		// Nak with delay to allow retry
		_ = message.NakWithDelay(10 * time.Second)
		return workflowExecutionError
	}

	// Signal PDF Completed
	processor.publishSimpleLifecycleEvent(parentContext, event.Header, events.SubjectPdfCompleted)

	processor.serviceLogger.Successf("Completed: %s", event.PdfKey)
	return nil
}

func (processor *Processor) executeWorkflow(parentContext context.Context, event *events.PdfCreatedEvent) error {
	// 1. Download PDF
	pdfData, downloadError := processor.downloadPDF(parentContext, event.PdfKey)
	if downloadError != nil {
		return fmt.Errorf("download failed: %w", downloadError)
	}

	// 2. Render PNGs
	pages, renderingError := processor.pdfRenderer.ProcessSinglePDFFromBytes(parentContext, pdfData)
	if renderingError != nil {
		return fmt.Errorf("render failed: %w", renderingError)
	}

	// 3. Process Job Settings (Extract Audio Session Config if not present)
	if event.Settings != nil && event.Settings.Voice != "" && (event.Settings.AudioSessionConfig == nil || event.Settings.AudioSessionConfig.VoiceIdentifier == "") {
		voiceIdentifier, voiceStyle := processor.parseVoice(event.Settings.Voice)

		if event.Settings.AudioSessionConfig == nil {
			event.Settings.AudioSessionConfig = &events.AudioSessionConfig{}
		}

		event.Settings.AudioSessionConfig.SessionIdentifier = uuid.New().String()
		event.Settings.AudioSessionConfig.SourceDocumentIdentifier = event.PdfKey
		event.Settings.AudioSessionConfig.VoiceIdentifier = voiceIdentifier
		event.Settings.AudioSessionConfig.VoiceStyle = voiceStyle
	}

	// 4. Lifecycle: PNGs Initialized
	for index := range pages {
		processor.publishPngLifecycleEvent(parentContext, event.Header, index+1, len(pages), events.SubjectPngInitialized)
	}

	// 5. Upload each page and publish events
	for index, pngContent := range pages {
		pageNumber := index + 1
		total := len(pages)

		// Signal PNG Started
		processor.publishPngLifecycleEvent(parentContext, event.Header, pageNumber, total, events.SubjectPngStarted)

		pngKey := fmt.Sprintf("%s-%d.png", event.PdfKey, pageNumber)
		if _, uploadError := processor.pngObjectStore.PutBytes(parentContext, pngKey, pngContent); uploadError != nil {
			return fmt.Errorf("upload page %d failed: %w", pageNumber, uploadError)
		}

		// Signal PNG Created (triggers next step)
		completionEvent := events.PngCreatedEvent{
			Header: events.EventHeader{
				WorkflowIdentifier: event.Header.WorkflowIdentifier,
				UserIdentifier:     event.Header.UserIdentifier,
				TenantIdentifier:   event.Header.TenantIdentifier,
				EventIdentifier:    uuid.New().String(),
				Timestamp:          time.Now().UTC(),
			},
			PngKey:     pngKey,
			PageNumber: pageNumber,
			TotalPages: total,
			Settings:   event.Settings,
		}

		data, _ := json.Marshal(completionEvent)
		if _, publishError := processor.jetStreamPublisher.Publish(parentContext, processor.producerSubject, data); publishError != nil {
			return fmt.Errorf("publish page %d failed: %w", pageNumber, publishError)
		}

		// Signal PNG Completed
		processor.publishPngLifecycleEvent(parentContext, event.Header, pageNumber, total, events.SubjectPngCompleted)
	}

	return nil
}

func (processor *Processor) publishSimpleLifecycleEvent(ctx context.Context, header events.EventHeader, subject string) {
	lifecycleEvent := events.StartedEvent{
		Header: events.EventHeader{
			WorkflowIdentifier: header.WorkflowIdentifier,
			UserIdentifier:     header.UserIdentifier,
			TenantIdentifier:   header.TenantIdentifier,
			EventIdentifier:    uuid.New().String(),
			Timestamp:          time.Now().UTC(),
		},
	}
	data, _ := json.Marshal(lifecycleEvent)
	_, _ = processor.jetStreamPublisher.Publish(ctx, subject, data)
}

func (processor *Processor) publishPngLifecycleEvent(ctx context.Context, header events.EventHeader, page, total int, subject string) {
	lifecycleEvent := events.PngCreatedEvent{
		Header: events.EventHeader{
			WorkflowIdentifier: header.WorkflowIdentifier,
			UserIdentifier:     header.UserIdentifier,
			TenantIdentifier:   header.TenantIdentifier,
			EventIdentifier:    uuid.New().String(),
			Timestamp:          time.Now().UTC(),
		},
		PageNumber: page,
		TotalPages: total,
	}
	data, _ := json.Marshal(lifecycleEvent)
	_, _ = processor.jetStreamPublisher.Publish(ctx, subject, data)
}

func (processor *Processor) downloadPDF(parentContext context.Context, pdfKey string) ([]byte, error) {
	object, getError := processor.pdfObjectStore.Get(parentContext, pdfKey)
	if getError != nil {
		return nil, getError
	}
	defer func() {
		_ = object.Close()
	}()

	information, _ := object.Info()
	data := make([]byte, information.Size)
	_, readError := object.Read(data)
	return data, readError
}

func (processor *Processor) parseVoice(voice string) (voiceIdentifier, voiceStyle string) {
	if voice == "" {
		return "", ""
	}

	start := -1
	end := -1
	for index, character := range voice {
		switch character {
		case '(':
			start = index
		case ')':
			end = index
		}
	}

	if start != -1 && end != -1 && end > start {
		voiceIdentifier = strings.TrimSpace(voice[:start])
		voiceStyle = strings.TrimSpace(voice[start+1 : end])
		return voiceIdentifier, voiceStyle
	}

	return strings.TrimSpace(voice), ""
}
