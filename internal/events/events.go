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
