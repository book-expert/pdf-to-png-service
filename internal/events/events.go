/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */
package events

import (
	common "github.com/book-expert/common-events"
)

// EventHeader is an alias to the canonical definition in common-events.
// This makes the common type available to other packages in this service
// that import this local 'events' package.
type EventHeader = common.EventHeader

// PDFProcessingStartedEvent is an alias to the canonical definition in common-events.
type PDFProcessingStartedEvent = common.PDFProcessingStartedEvent

const NoSoundscapeDirective = "[DO NOT GENERATE SOUNDSCAPE]"

type LyriaGenerationConfig struct {
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

type AudioSessionConfig struct {
	SessionID        string                 `json:"SessionID"`
	SourceDocumentID string                 `json:"SourceDocumentID"`
	VoiceID          string                 `json:"VoiceID"`    // The parsed voice name, e.g., "niko"
	VoiceStyle       string                 `json:"VoiceStyle"` // The parsed voice style, e.g., "calm, deep, mature"
	MusicPrompt      string                 `json:"MusicPrompt"`
	GenerationConfig *LyriaGenerationConfig `json:"GenerationConfig,omitempty"`
	TextDirective    string                 `json:"TextDirective"`
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
