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

package events

import "time"

// EventHeader contains metadata common to all events.
type EventHeader struct {
	Timestamp  time.Time `json:"Timestamp"`
	WorkflowID string    `json:"WorkflowID"`
	UserID     string    `json:"UserID"`
	TenantID   string    `json:"TenantID"`
	EventID    string    `json:"EventID"`
}

type AudioSessionConfig struct {
	SessionID        string `json:"SessionID"`
	SourceDocumentID string `json:"SourceDocumentID"`
	VoiceID          string `json:"VoiceID"`    // The parsed voice name, e.g., "niko"
	VoiceStyle       string `json:"VoiceStyle"` // The parsed voice style, e.g., "calm, deep, mature"
	MusicPrompt      string `json:"MusicPrompt"`
	TextDirective    string `json:"TextDirective"`
}

type JobSettings struct {
	SoundscapePrompt   string              `json:"SoundscapePrompt,omitempty"`
	AugmentationPrompt string              `json:"AugmentationPrompt,omitempty"`
	Exclusions         string              `json:"Exclusions,omitempty"`
	Voice              string              `json:"Voice,omitempty"`
	AudioSessionConfig *AudioSessionConfig `json:"AudioSessionConfig,omitempty"`
}

// PDFCreatedEvent is triggered when a PDF is uploaded.
type PDFCreatedEvent struct {
	Header   EventHeader  `json:"Header"`
	PDFKey   string       `json:"PDFKey"`
	Settings *JobSettings `json:"Settings,omitempty"`
}

// PNGCreatedEvent is triggered when a PNG page is generated from a PDF.
type PNGCreatedEvent struct {
	Header     EventHeader  `json:"Header"`
	PNGKey     string       `json:"PNGKey"`
	PageNumber int          `json:"PageNumber"`
	TotalPages int          `json:"TotalPages"`
	Settings   *JobSettings `json:"Settings,omitempty"`
}
