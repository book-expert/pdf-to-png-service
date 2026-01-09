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
			Stream  string `toml:"stream"`
			Subject string `toml:"subject"`
		} `toml:"producer"`
		ObjectStore struct {
			PDFBucket string `toml:"pdf_bucket"`
			PNGBucket string `toml:"png_bucket"`
		} `toml:"object_store"`
	} `toml:"nats"`
}

// Load reads and parses the project.toml file.
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var cfg Config
	decoder := toml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
