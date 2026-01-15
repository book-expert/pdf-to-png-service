/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package config

import (
	"os"
	"strconv"
)

// Config represents the configuration structure.
type Config struct {
	Service struct {
		LogDirectory           string
		Workers                int
		DotsPerInch            int
		BlankFuzzPercent       int
		BlankNonWhiteThreshold float64
	}
	LLM struct {
		APIKeyEnvironmentVariable     string
		Model                         string
		TextDirectiveGenerationPrompt string
		MusicConfigGenerationPrompt   string
		MaxRetries                    int
		TimeoutSeconds                int
		Temperature                   float64
	}
	NATS NATSConfig
}

// NATSConfig supplies the connection information for the message queue.
type NATSConfig struct {
	Address string
}

// Load retrieves the configuration from environment variables.
func Load(_ string) (*Config, error) {
	var configuration Config

	// Service
	configuration.Service.LogDirectory = getEnvironmentVariable("PDF_TO_PNG_LOG_DIR", "/home/niko/development/logs/tts-logs")
	configuration.Service.Workers = getEnvironmentVariableAsInteger("PDF_TO_PNG_WORKERS", 4)
	configuration.Service.DotsPerInch = getEnvironmentVariableAsInteger("PDF_TO_PNG_DPI", 300)
	configuration.Service.BlankFuzzPercent = getEnvironmentVariableAsInteger("PDF_TO_PNG_BLANK_FUZZ_PERCENT", 5)
	configuration.Service.BlankNonWhiteThreshold = getEnvironmentVariableAsFloat("PDF_TO_PNG_BLANK_NON_WHITE_THRESHOLD", 0.005)

	// LLM
	configuration.LLM.APIKeyEnvironmentVariable = "GEMINI_API_KEY"
	configuration.LLM.Model = getEnvironmentVariable("PDF_TO_PNG_LLM_MODEL", "gemini-2.5-flash")
	configuration.LLM.MaxRetries = getEnvironmentVariableAsInteger("PDF_TO_PNG_MAX_RETRIES", 3)
	configuration.LLM.TimeoutSeconds = getEnvironmentVariableAsInteger("PDF_TO_PNG_TIMEOUT_SECONDS", 60)
	configuration.LLM.Temperature = getEnvironmentVariableAsFloat("PDF_TO_PNG_TEMPERATURE", 0.5)
	configuration.LLM.TextDirectiveGenerationPrompt = os.Getenv("PDF_TO_PNG_TEXT_DIRECTIVE_GENERATION_PROMPT")
	configuration.LLM.MusicConfigGenerationPrompt = os.Getenv("PDF_TO_PNG_MUSIC_CONFIG_GENERATION_PROMPT")

	// NATS
	configuration.NATS.Address = getEnvironmentVariable("NATS_ADDRESS", "nats://localhost:4222")

	return &configuration, nil
}

func getEnvironmentVariable(keyName, fallbackValue string) string {
	if value, exists := os.LookupEnv(keyName); exists {
		return value
	}
	return fallbackValue
}

func getEnvironmentVariableAsInteger(keyName string, fallbackValue int) int {
	valueString := getEnvironmentVariable(keyName, "")
	if valueString == "" {
		return fallbackValue
	}
	value, error := strconv.Atoi(valueString)
	if error != nil {
		return fallbackValue
	}
	return value
}

func getEnvironmentVariableAsFloat(keyName string, fallbackValue float64) float64 {
	valueString := getEnvironmentVariable(keyName, "")
	if valueString == "" {
		return fallbackValue
	}
	value, error := strconv.ParseFloat(valueString, 64)
	if error != nil {
		return fallbackValue
	}
	return value
}
