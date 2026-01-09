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

package events

import (
	common "github.com/niko/common-events"
)

// EventHeader is an alias to the canonical definition in common-events.
// This makes the common type available to other packages in this service
// that import this local 'events' package.
type EventHeader = common.EventHeader

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
