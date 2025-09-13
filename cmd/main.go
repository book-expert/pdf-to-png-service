// ./cmd/pdf-to-png-service/main.go
// A NATS-driven worker that converts PDFs to PNGs, stores them in an
// object store, and publishes jobs for the next stage of processing.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nnikolov3/pdf-to-png-service/internal/converter"
	// You will need a library to handle the PDF conversion.
	// This is just an example; replace with your actual implementation.
)

// --- Constants for NATS Configuration ---
const (
	// Subject to listen on for new PDF jobs.
	pdfCreatedSubject = "pdf.created"
	// Consumer name for the PDF processing group.
	pdfConsumerName = "pdf-workers"
	// Stream that holds the PDF jobs.
	pdfStreamName = "PDF_JOBS"

	// Subject to publish to when a PNG is ready.
	pngCreatedSubject = "png.created"
	// The name of the JetStream Object Store for PNG files.
	pngObjectStoreName = "PNG_FILES"

	// General NATS configuration
	natsMaxAckPending = 10
	natsFetchTimeout  = 5 * time.Second
)

// PDFCreatedJob represents the incoming job message.
// For now, it contains a file path. In a cloud environment, this would
// be an S3 URI or another object key.
type PDFCreatedJob struct {
	PDFPath string `json:"pdfPath"`
	JobID   string `json:"jobId"`
}

// --- Main Application ---

func main() {
	// Setup graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Fatalf("Fatal application error: %v", err)
	}
	log.Println("Application shut down gracefully.")
}

func run(ctx context.Context) error {
	// --- Connect to NATS ---
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer nc.Close()
	log.Printf("Connected to NATS server at %s", nc.ConnectedUrl())

	// --- Get JetStream Context ---
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	// --- Ensure NATS Resources Exist ---
	if err := setupJetStream(js); err != nil {
		return fmt.Errorf("failed to set up JetStream resources: %w", err)
	}

	// --- Create a Pull Subscription to the PDF jobs consumer ---
	sub, err := js.PullSubscribe(
		ctx,
		"",
		pdfConsumerName,
		jetstream.Bind(pdfStreamName, pdfConsumerName),
	)
	if err != nil {
		return fmt.Errorf("failed to create pull subscription: %w", err)
	}

	log.Printf("Worker is running, listening for jobs on '%s'...", pdfCreatedSubject)

	// --- Start the message processing loop ---
	return processMessages(ctx, sub, js)
}

// setupJetStream ensures the required stream, consumer, and object store exist.
func setupJetStream(js jetstream.JetStream) error {
	ctx := context.Background()

	// 1. Create the PDF_JOBS stream
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     pdfStreamName,
		Subjects: []string{pdfCreatedSubject},
		Storage:  jetstream.FileStorage,
	})
	if err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	log.Printf("Stream '%s' is ready.", pdfStreamName)

	// 2. Create the consumer for the PDF jobs
	_, err = js.CreateConsumer(ctx, pdfStreamName, jetstream.ConsumerConfig{
		Durable:       pdfConsumerName,
		FilterSubject: pdfCreatedSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil && !errors.Is(err, jetstream.ErrConsumerNameAlreadyInUse) {
		return fmt.Errorf("failed to create consumer: %w", err)
	}
	log.Printf("Consumer '%s' is ready.", pdfConsumerName)

	// 3. Create the Object Store for PNGs
	_, err = js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:  pngObjectStoreName,
		Storage: jetstream.FileStorage,
	})
	if err != nil && !errors.Is(err, jetstream.ErrBucketExists) {
		return fmt.Errorf("failed to create object store: %w", err)
	}
	log.Printf("Object Store '%s' is ready.", pngObjectStoreName)

	return nil
}

// processMessages runs the main loop to fetch and handle messages.
func processMessages(
	ctx context.Context,
	sub jetstream.Subscription,
	js jetstream.JetStream,
) error {
	pngStore, err := js.ObjectStore(ctx, pngObjectStoreName)
	if err != nil {
		return fmt.Errorf("failed to bind to object store: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil // Shutdown signal received
		default:
			// Fetch a batch of messages.
			msgs, err := sub.Fetch(1, jetstream.FetchMaxWait(natsFetchTimeout))
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
					errors.Is(err, nats.ErrTimeout) {
					continue // Normal timeout, loop again.
				}
				log.Printf("Error fetching messages: %v", err)
				continue
			}

			for _, msg := range msgs {
				handleMessage(ctx, msg, js, pngStore)
			}
		}
	}
}

// handleMessage processes a single PDF job.
func handleMessage(
	ctx context.Context,
	msg jetstream.Msg,
	js jetstream.JetStream,
	pngStore jetstream.ObjectStore,
) {
	// For simplicity, we assume the message payload is the PDF file path.
	// In a real system, you'd unmarshal a JSON struct here.
	pdfPath := string(msg.Data())
	jobID := filepath.Base(pdfPath) // Use the filename as a simple job ID.

	log.Printf("Received job [%s]: processing PDF '%s'", jobID, pdfPath)

	// Terminate the message so it won't be redelivered while we process.
	// We'll re-Nack it if something goes wrong.
	msg.InProgress()

	// --- 1. Core Logic: Convert PDF to PNGs ---
	// This function returns paths to temporary PNG files.
	pngFilePaths, err := converter.ProcessPDF(pdfPath)
	if err != nil {
		log.Printf("Error processing job [%s]: %v", jobID, err)
		msg.Nak() // Tell NATS to redeliver the message later.
		return
	}

	log.Printf("Job [%s]: Successfully converted PDF to %d PNGs.", jobID, len(pngFilePaths))

	// --- 2. Producer Logic: Upload PNGs and Publish Jobs ---
	for i, pngPath := range pngFilePaths {
		file, err := os.Open(pngPath)
		if err != nil {
			log.Printf("Job [%s]: Failed to open PNG '%s': %v", jobID, pngPath, err)
			continue // Skip this file, but try others.
		}
		defer file.Close()
		defer os.Remove(pngPath) // Clean up temp file.

		// A. Upload to Object Store
		objectName := fmt.Sprintf(
			"%s_page_%d.png",
			strings.TrimSuffix(jobID, filepath.Ext(jobID)),
			i+1,
		)
		_, err = pngStore.Put(ctx, jetstream.ObjectMeta{Name: objectName}, file)
		if err != nil {
			log.Printf(
				"Job [%s]: Failed to upload '%s' to object store: %v",
				jobID,
				objectName,
				err,
			)
			continue
		}
		log.Printf("Job [%s]: Successfully uploaded '%s'", jobID, objectName)

		// B. Publish the 'png.created' job message. The payload is the object name.
		_, err = js.Publish(ctx, pngCreatedSubject, []byte(objectName))
		if err != nil {
			log.Printf("Job [%s]: Failed to publish job for '%s': %v", jobID, objectName, err)
		} else {
			log.Printf("Job [%s]: Published job for '%s'", jobID, objectName)
		}
	}

	// --- 3. Acknowledge the original PDF job message ---
	if err := msg.Ack(); err != nil {
		log.Printf("Job [%s]: Failed to acknowledge message: %v", jobID, err)
	} else {
		log.Printf("Job [%s]: Processing complete. Message acknowledged.", jobID)
	}
}
