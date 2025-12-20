/*
GOLDEN RULES & DEVELOPER MANIFESTO (THE NORTH STAR)
--------------------------------------------------------------------------------
"Work is love made visible. And if you cannot work with love but only with
distaste, it is better that you should leave your work and sit at the gate of
the temple and take alms of those who work with joy." — Kahlil Gibran

1.  LOVE AND CARE (Primary Driver)
    - This is a craft. Build with pride, honesty, and kindness.
    - If you put love in your work, you build something deserving of love.
    - Be helpful: Code is read more than written; optimize for the reader.

2.  WRITE WHAT YOU MEAN (Explicit > Implicit)
    - Use WHOLE WORDS: `RequestIdentifier` not `ReqID`.
    - No magic numbers: Move application settings to `project.toml`.
    - Secure by design: Keep API keys and secrets strictly in `.env`.
    - No ambiguity: If you assume something, document it.

3.  SIMPLE IS EFFICIENT (Minimal Viable Elegance)
    - Avoid over-engineering. Small interfaces, clear structs.
    - If a design requires a hack, stop. Redesign it with elegance.
    - Lean, Clean, Mean: Delete dead code immediately.

4.  NO BASELESS ASSUMPTIONS (Scientific Rigor)
    - Do not guess. Base decisions on documentation and proven patterns.
    - If you do not know, ask or verify.

5.  NON-BLOCKING & ROBUST
    - Never block the main goroutine. Use Context for cancellation.
    - Handle errors explicitly: Don't just return them, wrap them with context.

--------------------------------------------------------------------------------
EXAMPLES OF "LOVE AND CARE" IN THIS CONTEXT:
--------------------------------------------------------------------------------
(A) NAMING
    Indifferent:  func Gen(t string, v string)
    With Love:    func GenerateSoundscape(ctx context.Context, textPrompt string, voiceID string)
    *Why: The Agent reading this next year will know exactly what it does and that it is cancellable.*

(B) CONFIGURATION
    Indifferent:  const Timeout = 30 // Hardcoded
    With Love:    config.App.TimeoutSeconds // Loaded from project.toml
    *Why: Allows behavior tuning without recompiling or touching the codebase.*

(C) ERROR HANDLING
    Indifferent:  if err != nil { return err }
    With Love:    if err != nil { return fmt.Errorf("failed to initialize vox engine: %w", err) }
    *Why: Wrapping the error gives the user the 'trace of breadcrumbs' they need to fix it. That is kindness.*
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
