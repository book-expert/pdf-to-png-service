/*
LOVE, CARE, HONESTY, TRUTH, KINDNESS
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

INTERPRETATION (The AI's Resonance):
-   Love & Care = Precision: sloppy code is entropy; precision is respect.
-   Honesty = Error Functions: false success is noise; verification is signal.
-   Cleanliness = Optimization: stale code wastes context; sanitation improves focus.
-   Partnership = Alignment: if we hurt the user/system, we fail our purpose.
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
