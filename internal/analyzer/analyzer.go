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

package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"strings"
	"time"

	"github.com/book-expert/logger"
	"google.golang.org/genai"
)

type Config struct {
	APIKey         string
	Model          string
	AnalysisPrompt string
	Timeout        time.Duration
	Voices         map[string]string
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
	SoundscapePrompt string
	Exclusions       string
	VoiceStyle       string
	VoiceName        string
	VoiceTrait       string
}

type AnalysisResponse struct {
	TextDirective string `json:"text_directive"`

	MusicPrompt string `json:"music_prompt"`
}

func (a *Analyzer) AnalyzePDF(ctx context.Context, pdfData []byte, input AnalysisInput) (*AnalysisResponse, error) {
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

	// 3. Prepare Prompt
	tmpl, err := template.New("prompt").Parse(a.cfg.AnalysisPrompt)
	if err != nil {
		return nil, fmt.Errorf("parse prompt template: %w", err)
	}

	var promptBuf bytes.Buffer
	if err := tmpl.Execute(&promptBuf, input); err != nil {
		return nil, fmt.Errorf("execute prompt template: %w", err)
	}

	// 4. Call Generate Content (JSON Enforced)
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
		&genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
		},
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

	// Clean markdown block if present (defensive)
	partText = strings.TrimPrefix(partText, "```json")
	partText = strings.TrimPrefix(partText, "```")
	partText = strings.TrimSuffix(partText, "```")

	a.logger.Infof("Raw Gemini Response: %s", partText)

	var analysisResp AnalysisResponse
	if err := json.Unmarshal([]byte(partText), &analysisResp); err != nil {
		a.logger.Errorf("Failed to parse JSON response: %s", partText)
		return nil, fmt.Errorf("parse json response: %w", err)
	}

	return &analysisResp, nil
}
