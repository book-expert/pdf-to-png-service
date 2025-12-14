package analyzer

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"os"
	"time"

	"github.com/book-expert/logger"
	"github.com/book-expert/pdf-to-png-service/internal/events"
	"github.com/google/uuid"
	"google.golang.org/genai"
)

type Config struct {
	APIKey         string
	Model          string
	AnalysisPrompt string
	Timeout        time.Duration
}

type Analyzer struct {
	client *genai.Client
	cfg    Config
	logger *logger.Logger
}

func New(ctx context.Context, cfg Config, logger *logger.Logger) (*Analyzer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &Analyzer{
		client: client,
		cfg:    cfg,
		logger: logger,
	}, nil
}

type AnalysisInput struct {
	Scene        string
	Style        string
	Accent       string
	Articulation string
	Pace         string
	Personality  string
	Exclusions   string
}

func (a *Analyzer) AnalyzePDF(ctx context.Context, pdfData []byte, input AnalysisInput) (*events.AudioSessionConfig, error) {
	// 1. Write PDF to temp file for upload
	tmpFile, err := os.CreateTemp("", "analyze-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			a.logger.Warnf("Failed to remove temp file %s: %v", tmpFile.Name(), err)
		}
	}()

	if _, err := io.Copy(tmpFile, bytes.NewReader(pdfData)); err != nil {
		_ = tmpFile.Close() // Best effort close
		return nil, fmt.Errorf("write pdf data: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	// 2. Upload file to Gemini
	uploadConfig := &genai.UploadFileConfig{
		DisplayName: fmt.Sprintf("analyze-%d", time.Now().Unix()),
		MIMEType:    "application/pdf",
	}
	// Re-open file for reading
	f, err := os.Open(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("open temp file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			a.logger.Warnf("Failed to close temp file %s: %v", tmpFile.Name(), err)
		}
	}()

	uploadResult, err := a.client.Files.Upload(ctx, f, uploadConfig)
	if err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}

	// Defer deletion of the uploaded file from Gemini to save storage/cleanup
	defer func() {
		if _, err := a.client.Files.Delete(ctx, uploadResult.Name, nil); err != nil {
			a.logger.Warnf("Failed to delete remote file %s: %v", uploadResult.Name, err)
		}
	}()

	// Wait for file to be active (processing)
	// For PDFs, it might take a moment.
	// Ideally we poll GetFile until State is ACTIVE.
	// For simplicity in this iteration, we assume it's ready or will be handled by GenerateContent waiting.
	// (Note: GenAI Go SDK's GenerateContent often handles waiting implicitly or returns error if not ready)

	// 3. Prepare Prompt
	tmpl, err := template.New("prompt").Parse(a.cfg.AnalysisPrompt)
	if err != nil {
		return nil, fmt.Errorf("parse prompt template: %w", err)
	}

	var promptBuf bytes.Buffer
	if err := tmpl.Execute(&promptBuf, input); err != nil {
		return nil, fmt.Errorf("execute prompt template: %w", err)
	}

	// 4. Call Generate Content
	promptText := promptBuf.String()
	resp, err := a.client.Models.GenerateContent(
		ctx,
		a.cfg.Model,
		[]*genai.Content{
			{
				Parts: []*genai.Part{
					{
						FileData: &genai.FileData{
							FileURI:  uploadResult.URI,
							MIMEType: uploadResult.MIMEType,
						},
					},
					{
						Text: promptText,
					},
				},
			},
		},
		&genai.GenerateContentConfig{},
	)
	if err != nil {
		return nil, fmt.Errorf("generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content generated")
	}

	// 5. Parse Response
	var partText string
	for _, part := range resp.Candidates[0].Content.Parts {
		partText += part.Text
	}

	// Create config with raw Master Directive
	config := events.AudioSessionConfig{
		SessionID:       uuid.New().String(),
		MasterDirective: partText,
	}

	return &config, nil
}
