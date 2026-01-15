/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package config

import (
	"os"
	"strconv"
)

// Config represents the configuration structure.
type Config struct {
	Service struct {
		LogDir                 string
		Workers                int
		DPI                    int
		BlankFuzzPercent       int
		BlankNonWhiteThreshold float64
	}
	LLM struct {
		APIKeyVariable                string
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
	configuration.Service.LogDir = getEnv("PDF_TO_PNG_LOG_DIR", "/home/niko/development/logs/tts-logs")
	configuration.Service.Workers = getEnvInt("PDF_TO_PNG_WORKERS", 4)
	configuration.Service.DPI = getEnvInt("PDF_TO_PNG_DPI", 300)
	configuration.Service.BlankFuzzPercent = getEnvInt("PDF_TO_PNG_BLANK_FUZZ_PERCENT", 5)
	configuration.Service.BlankNonWhiteThreshold = getEnvFloat("PDF_TO_PNG_BLANK_NON_WHITE_THRESHOLD", 0.005)

	// LLM
	configuration.LLM.APIKeyVariable = "GEMINI_API_KEY"
	configuration.LLM.Model = getEnv("PDF_TO_PNG_LLM_MODEL", "gemini-2.5-flash")
	configuration.LLM.MaxRetries = getEnvInt("PDF_TO_PNG_MAX_RETRIES", 3)
	configuration.LLM.TimeoutSeconds = getEnvInt("PDF_TO_PNG_TIMEOUT_SECONDS", 60)
	configuration.LLM.Temperature = getEnvFloat("PDF_TO_PNG_TEMPERATURE", 0.5)
	configuration.LLM.TextDirectiveGenerationPrompt = os.Getenv("PDF_TO_PNG_TEXT_DIRECTIVE_GENERATION_PROMPT")
	configuration.LLM.MusicConfigGenerationPrompt = os.Getenv("PDF_TO_PNG_MUSIC_CONFIG_GENERATION_PROMPT")

	// NATS
	configuration.NATS.Address = getEnv("NATS_ADDRESS", "nats://localhost:4222")

	return &configuration, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return fallback
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvFloat(key string, fallback float64) float64 {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return fallback
	}
	return value
}
