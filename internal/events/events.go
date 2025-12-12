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

// PDFCreatedEvent is triggered when a PDF is uploaded.
type PDFCreatedEvent struct {
	Header EventHeader `json:"header"`
	PDFKey string      `json:"pdf_key"`
}

// PNGCreatedEvent is triggered when a PNG page is generated from a PDF.
type PNGCreatedEvent struct {
	Header     EventHeader `json:"header"`
	PNGKey     string      `json:"png_key"`
	PageNumber int         `json:"page_number"`
	TotalPages int         `json:"total_pages"`
}
