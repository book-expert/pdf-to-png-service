/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */
package config

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Config represents the configuration structure.
type Config struct {
	Service struct {
		LogDir                 string  `toml:"log_dir"`
		Workers                int     `toml:"workers"`
		DPI                    int     `toml:"dpi"`
		BlankFuzzPercent       int     `toml:"blank_fuzz_percent"`
		BlankNonWhiteThreshold float64 `toml:"blank_non_white_threshold"`
	} `toml:"service"`
	Voices map[string]string `toml:"voices"`
	LLM    struct {
		APIKeyVariable                string  `toml:"api_key_variable"`
		Model                         string  `toml:"model"`
		TextDirectiveGenerationPrompt string  `toml:"text_directive_generation_prompt"`
		MusicConfigGenerationPrompt   string  `toml:"music_config_generation_prompt"`
		TimeoutSeconds                int     `toml:"timeout_seconds"`
		Temperature                   float64 `toml:"temperature"`
	} `toml:"llm"`
	NATS struct {
		URL        string `toml:"url"`
		DLQSubject string `toml:"dlq_subject"`
		Consumer   struct {
			Stream  string `toml:"stream"`
			Subject string `toml:"subject"`
			Durable string `toml:"durable"`
		} `toml:"consumer"`
		Producer struct {
			Stream                      string `toml:"stream"`
			Subject                     string `toml:"subject"`
			PDFProcessingStartedSubject string `toml:"pdf_processing_started_subject"`
		} `toml:"producer"`
		ObjectStore struct {
			PDFBucket string `toml:"pdf_bucket"`
			PNGBucket string `toml:"png_bucket"`
		} `toml:"object_store"`
	} `toml:"nats"`
}

// Load reads and parses the project.toml file.
func Load(path string) (*Config, error) {
	file, error := os.Open(path)
	if error != nil {
		return nil, error
	}
	defer func() { _ = file.Close() }()

	var configuration Config
	decoder := toml.NewDecoder(file)
	if error := decoder.Decode(&configuration); error != nil {
		return nil, error
	}

	// Apply Environment Overrides
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		configuration.NATS.URL = natsURL
	}

	return &configuration, nil
}
