/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

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
	APIKey                        string
	Model                         string
	TextDirectiveGenerationPrompt string
	MusicConfigGenerationPrompt   string
	Timeout                       time.Duration
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
	SoundscapePrompt   string
	AugmentationPrompt string
	Exclusions         string
	VoiceStyle         string
	VoiceName          string
	VoiceTrait         string
}

type LyriaParams struct {
	BPM                 int     `json:"bpm,omitempty"`
	Density             float64 `json:"density,omitempty"`
	Brightness          float64 `json:"brightness,omitempty"`
	Guidance            float64 `json:"guidance,omitempty"`
	MuteBass            bool    `json:"mute_bass,omitempty"`
	MuteDrums           bool    `json:"mute_drums,omitempty"`
	OnlyBassAndDrums    bool    `json:"only_bass_and_drums,omitempty"`
	MusicGenerationMode string  `json:"music_generation_mode,omitempty"`
	Scale               string  `json:"scale,omitempty"`
}

type MusicAnalysisResponse struct {
	MusicPrompt      string      `json:"music_prompt"`
	GenerationConfig LyriaParams `json:"generation_config"`
}

// GenerateTextDirective analyzes the PDF and returns a plain text string of instructions.
func (a *Analyzer) GenerateTextDirective(ctx context.Context, pdfData []byte, input AnalysisInput) (string, error) {
	return a.generateContent(ctx, pdfData, input, a.cfg.TextDirectiveGenerationPrompt, "text/plain")
}

// GenerateMusicConfig analyzes the PDF and returns a structured configuration for Lyria.
func (a *Analyzer) GenerateMusicConfig(ctx context.Context, pdfData []byte, input AnalysisInput) (*MusicAnalysisResponse, error) {
	jsonStr, err := a.generateContent(ctx, pdfData, input, a.cfg.MusicConfigGenerationPrompt, "application/json")
	if err != nil {
		return nil, err
	}

	// Clean markdown block if present (defensive)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")

	var resp MusicAnalysisResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		a.logger.Errorf("Failed to parse Music Config JSON: %s", jsonStr)
		return nil, fmt.Errorf("parse json response: %w", err)
	}

	return &resp, nil
}

// generateContent is a helper to handle the common flow: upload PDF -> execute prompt -> call Gemini.
func (a *Analyzer) generateContent(
	ctx context.Context,
	pdfData []byte,
	input AnalysisInput,
	promptTemplate string,
	responseMIMEType string,
) (string, error) {
	// 1. Write PDF to temp file for upload
	tmpFile, err := os.CreateTemp("", "analyze-*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			a.logger.Warnf("Failed to remove temp file %s: %v", tmpFile.Name(), err)
		}
	}()

	if _, err := io.Copy(tmpFile, bytes.NewReader(pdfData)); err != nil {
		_ = tmpFile.Close() // Best effort close
		return "", fmt.Errorf("write pdf data: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	// 2. Upload file to Gemini
	uploadConfig := &genai.UploadFileConfig{
		DisplayName: fmt.Sprintf("analyze-%d", time.Now().UnixNano()),
		MIMEType:    "application/pdf",
	}
	// Re-open file for reading
	f, err := os.Open(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("open temp file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			a.logger.Warnf("Failed to close temp file %s: %v", tmpFile.Name(), err)
		}
	}()

	uploadResult, err := a.client.Files.Upload(ctx, f, uploadConfig)
	if err != nil {
		return "", fmt.Errorf("upload file: %w", err)
	}

	// Defer deletion of the uploaded file from Gemini to save storage/cleanup
	defer func() {
		if _, err := a.client.Files.Delete(ctx, uploadResult.Name, nil); err != nil {
			a.logger.Warnf("Failed to delete remote file %s: %v", uploadResult.Name, err)
		}
	}()

	// 3. Prepare Prompt
	tmpl, err := template.New("prompt").Parse(promptTemplate)
	if err != nil {
		return "", fmt.Errorf("parse prompt template: %w", err)
	}

	var promptBuf bytes.Buffer
	if err := tmpl.Execute(&promptBuf, input); err != nil {
		return "", fmt.Errorf("execute prompt template: %w", err)
	}

	// 4. Call Generate Content
	promptText := promptBuf.String()

	// Configure generation options
	genConfig := &genai.GenerateContentConfig{}
	if responseMIMEType != "" {
		genConfig.ResponseMIMEType = responseMIMEType
	}

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
		genConfig,
	)
	if err != nil {
		return "", fmt.Errorf("generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content generated")
	}

	// 5. Extract Response Text
	var partText string
	for _, part := range resp.Candidates[0].Content.Parts {
		partText += part.Text
	}

	a.logger.Infof("Raw Gemini Response (%s): %s", responseMIMEType, partText)

	return partText, nil
}
