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
	configuration    Config
	logger *logger.Logger
}

func New(parentContext context.Context, configuration Config, logger *logger.Logger) (*Analyzer, error) {
	client, error := genai.NewClient(parentContext, &genai.ClientConfig{
		APIKey: configuration.APIKey,
	})
	if error != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", error)
	}

	return &Analyzer{
		client: client,
		configuration:    configuration,
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
func (analyzer *Analyzer) GenerateTextDirective(parentContext context.Context, pdfData []byte, input AnalysisInput) (string, error) {
	return analyzer.generateContent(parentContext, pdfData, input, analyzer.configuration.TextDirectiveGenerationPrompt, "text/plain")
}

// GenerateMusicConfig analyzes the PDF and returns a structured configuration for Lyria.
func (analyzer *Analyzer) GenerateMusicConfig(parentContext context.Context, pdfData []byte, input AnalysisInput) (*MusicAnalysisResponse, error) {
	jsonString, error := analyzer.generateContent(parentContext, pdfData, input, analyzer.configuration.MusicConfigGenerationPrompt, "application/json")
	if error != nil {
		return nil, error
	}

	// Clean markdown block if present (defensive)
	jsonString = strings.TrimPrefix(jsonString, "```json")
	jsonString = strings.TrimPrefix(jsonString, "```")
	jsonString = strings.TrimSuffix(jsonString, "```")

	var response MusicAnalysisResponse
	if unmarshalError := json.Unmarshal([]byte(jsonString), &response); unmarshalError != nil {
		analyzer.logger.Errorf("Failed to parse Music Config JSON: %s", jsonString)
		return nil, fmt.Errorf("parse json response: %w", unmarshalError)
	}

	return &response, nil
}

// generateContent is a helper to handle the common flow: upload PDF -> execute prompt -> call Gemini.
func (analyzer *Analyzer) generateContent(
	parentContext context.Context,
	pdfData []byte,
	input AnalysisInput,
	promptTemplate string,
	responseMIMEType string,
) (string, error) {
	// 1. Write PDF to temp file for upload
	temporaryFile, error := os.CreateTemp("", "analyze-*.pdf")
	if error != nil {
		return "", fmt.Errorf("create temp file: %w", error)
	}
	defer func() {
		if removalError := os.Remove(temporaryFile.Name()); removalError != nil {
			analyzer.logger.Warnf("Failed to remove temp file %s: %v", temporaryFile.Name(), removalError)
		}
	}()

	if _, error := io.Copy(temporaryFile, bytes.NewReader(pdfData)); error != nil {
		_ = temporaryFile.Close() // Best effort close
		return "", fmt.Errorf("write pdf data: %w", error)
	}
	if error := temporaryFile.Close(); error != nil {
		return "", fmt.Errorf("close temp file: %w", error)
	}

	// 2. Upload file to Gemini
	uploadConfig := &genai.UploadFileConfig{
		DisplayName: fmt.Sprintf("analyze-%d", time.Now().UnixNano()),
		MIMEType:    "application/pdf",
	}
	// Re-open file for reading
	file, error := os.Open(temporaryFile.Name())
	if error != nil {
		return "", fmt.Errorf("open temp file: %w", error)
	}
	defer func() {
		if closeError := file.Close(); closeError != nil {
			analyzer.logger.Warnf("Failed to close temp file %s: %v", temporaryFile.Name(), closeError)
		}
	}()

	uploadResult, error := analyzer.client.Files.Upload(parentContext, file, uploadConfig)
	if error != nil {
		return "", fmt.Errorf("upload file: %w", error)
	}

	// Defer deletion of the uploaded file from Gemini to save storage/cleanup
	defer func() {
		if _, deletionError := analyzer.client.Files.Delete(parentContext, uploadResult.Name, nil); deletionError != nil {
			analyzer.logger.Warnf("Failed to delete remote file %s: %v", uploadResult.Name, deletionError)
		}
	}()

	// 3. Prepare Prompt
	textTemplate, error := template.New("prompt").Parse(promptTemplate)
	if error != nil {
		return "", fmt.Errorf("parse prompt template: %w", error)
	}

	var promptBuffer bytes.Buffer
	if error := textTemplate.Execute(&promptBuffer, input); error != nil {
		return "", fmt.Errorf("execute prompt template: %w", error)
	}

	// 4. Call Generate Content
	promptText := promptBuffer.String()

	// Configure generation options
	generationConfig := &genai.GenerateContentConfig{}
	if responseMIMEType != "" {
		generationConfig.ResponseMIMEType = responseMIMEType
	}

	response, error := analyzer.client.Models.GenerateContent(
		parentContext,
		analyzer.configuration.Model,
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
		generationConfig,
	)
	if error != nil {
		return "", fmt.Errorf("generate content: %w", error)
	}

	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content generated")
	}

	// 5. Extract Response Text
	var partText string
	for _, part := range response.Candidates[0].Content.Parts {
		partText += part.Text
	}

	analyzer.logger.Infof("Raw Gemini Response (%s): %s", responseMIMEType, partText)

	return partText, nil
}