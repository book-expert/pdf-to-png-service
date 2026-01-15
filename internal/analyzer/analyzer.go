/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/book-expert/common-events"
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
	client        *genai.Client
	configuration Config
	serviceLogger *logger.Logger
}

func New(parentContext context.Context, configuration Config, serviceLogger *logger.Logger) (*Analyzer, error) {
	client, initializationError := genai.NewClient(parentContext, &genai.ClientConfig{
		APIKey: configuration.APIKey,
	})
	if initializationError != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", initializationError)
	}

	return &Analyzer{
		client:        client,
		configuration: configuration,
		serviceLogger: serviceLogger,
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

type MusicAnalysisResponse struct {
	MusicPrompt      string                       `json:"music_prompt"`
	GenerationConfig events.LyriaGenerationConfig `json:"generation_config"`
}

// GenerateTextDirective analyzes the PDF and returns a plain text string of instructions.
func (analyzer *Analyzer) GenerateTextDirective(parentContext context.Context, pdfData []byte, input AnalysisInput) (string, error) {
	return analyzer.generateContent(parentContext, pdfData, input, analyzer.configuration.TextDirectiveGenerationPrompt, "text/plain")
}

// GenerateMusicConfig analyzes the PDF and returns a structured configuration for Lyria.
func (analyzer *Analyzer) GenerateMusicConfig(parentContext context.Context, pdfData []byte, input AnalysisInput) (*MusicAnalysisResponse, error) {
	jsonString, generationError := analyzer.generateContent(parentContext, pdfData, input, analyzer.configuration.MusicConfigGenerationPrompt, "application/json")
	if generationError != nil {
		return nil, generationError
	}

	// Clean markdown block if present (defensive)
	jsonString = strings.TrimPrefix(jsonString, "```json")
	jsonString = strings.TrimPrefix(jsonString, "```")
	jsonString = strings.TrimSuffix(jsonString, "```")

	var response MusicAnalysisResponse
	if unmarshalError := json.Unmarshal([]byte(jsonString), &response); unmarshalError != nil {
		analyzer.serviceLogger.Errorf("Failed to parse Music Config JSON: %s", jsonString)
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
	temporaryFile, creationError := os.CreateTemp("", "analyze-*.pdf")
	if creationError != nil {
		return "", fmt.Errorf("create temp file: %w", creationError)
	}
	defer func() {
		if removalError := os.Remove(temporaryFile.Name()); removalError != nil {
			analyzer.serviceLogger.Warnf("Failed to remove temp file %s: %v", temporaryFile.Name(), removalError)
		}
	}()

	if _, copyError := io.Copy(temporaryFile, bytes.NewReader(pdfData)); copyError != nil {
		_ = temporaryFile.Close() // Best effort close
		return "", fmt.Errorf("write pdf data: %w", copyError)
	}
	if closeError := temporaryFile.Close(); closeError != nil {
		return "", fmt.Errorf("close temp file: %w", closeError)
	}

	// 2. Upload file to Gemini
	uploadConfig := &genai.UploadFileConfig{
		DisplayName: fmt.Sprintf("analyze-%d", time.Now().UnixNano()),
		MIMEType:    "application/pdf",
	}
	// Re-open file for reading
	file, openError := os.Open(temporaryFile.Name())
	if openError != nil {
		return "", fmt.Errorf("open temp file: %w", openError)
	}
	defer func() {
		if closeError := file.Close(); closeError != nil {
			analyzer.serviceLogger.Warnf("Failed to close temp file %s: %v", temporaryFile.Name(), closeError)
		}
	}()

	uploadResult, uploadError := analyzer.client.Files.Upload(parentContext, file, uploadConfig)
	if uploadError != nil {
		return "", fmt.Errorf("upload file: %w", uploadError)
	}

	// Defer deletion of the uploaded file from Gemini to save storage/cleanup
	defer func() {
		if _, deletionError := analyzer.client.Files.Delete(parentContext, uploadResult.Name, nil); deletionError != nil {
			analyzer.serviceLogger.Warnf("Failed to delete remote file %s: %v", uploadResult.Name, deletionError)
		}
	}()

	// 3. Prepare Prompt
	textTemplate, parseError := template.New("prompt").Parse(promptTemplate)
	if parseError != nil {
		return "", fmt.Errorf("parse prompt template: %w", parseError)
	}

	var promptBuffer bytes.Buffer
	if executionError := textTemplate.Execute(&promptBuffer, input); executionError != nil {
		return "", fmt.Errorf("execute prompt template: %w", executionError)
	}

	// 4. Call Generate Content
	promptText := promptBuffer.String()

	// Configure generation options
	generationConfig := &genai.GenerateContentConfig{}
	if responseMIMEType != "" {
		generationConfig.ResponseMIMEType = responseMIMEType
	}

	response, generationError := analyzer.client.Models.GenerateContent(
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
	if generationError != nil {
		return "", fmt.Errorf("generate content: %w", generationError)
	}

	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content generated")
	}

	// 5. Extract Response Text
	var partText string
	for _, part := range response.Candidates[0].Content.Parts {
		partText += part.Text
	}

	analyzer.serviceLogger.Infof("Raw Gemini Response (%s): %s", responseMIMEType, partText)

	return partText, nil
}
