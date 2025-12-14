package events

import "time"

// EventHeader contains metadata common to all events.
type EventHeader struct {
	Timestamp  time.Time `json:"timestamp"`
	WorkflowID string    `json:"workflow_id"`
	UserID     string    `json:"user_id"`
	TenantID   string    `json:"tenant_id"`
	EventID    string    `json:"event_id"`
}

type AudioSessionConfig struct {
	SessionID        string `json:"session_id"`
	SourceDocumentID string `json:"source_document_id"`
	VoiceID          string `json:"voice_id"`
	MasterDirective  string `json:"master_directive"` // Raw Markdown (Scene + Director's Notes)
}

type JobSettings struct {
	TranscriptionMode  string              `json:"transcription_mode,omitempty"`
	StyleProfile       string              `json:"style_profile,omitempty"`
	CustomInstructions string              `json:"custom_instructions,omitempty"`
	Exclusions         string              `json:"exclusions,omitempty"`
	Voice              string              `json:"voice,omitempty"`
	Language           string              `json:"language,omitempty"`
	Scene              string              `json:"scene,omitempty"`
	Style              string              `json:"style,omitempty"`
	Accent             string              `json:"accent,omitempty"`
	Articulation       string              `json:"articulation,omitempty"`
	Pace               string              `json:"pace,omitempty"`
	Personality        string              `json:"personality,omitempty"`
	AudioSessionConfig *AudioSessionConfig `json:"audio_session_config,omitempty"`
}

// PDFCreatedEvent is triggered when a PDF is uploaded.
type PDFCreatedEvent struct {
	Header   EventHeader  `json:"header"`
	PDFKey   string       `json:"pdf_key"`
	Settings *JobSettings `json:"settings,omitempty"`
}

// PNGCreatedEvent is triggered when a PNG page is generated from a PDF.
type PNGCreatedEvent struct {
	Header     EventHeader  `json:"header"`
	PNGKey     string       `json:"png_key"`
	PageNumber int          `json:"page_number"`
	TotalPages int          `json:"total_pages"`
	Settings   *JobSettings `json:"settings,omitempty"`
}
