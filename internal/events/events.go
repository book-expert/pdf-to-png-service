/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */
package events

import (
	events "github.com/book-expert/common-events"
)

// EventHeader is an alias to the canonical definition in common-events.
// This makes the common type available to other packages in this service
// that import this local 'events' package.
type EventHeader = events.EventHeader

// PDFProcessingStartedEvent is an alias to the canonical definition in common-events.
type PDFProcessingStartedEvent = events.PDFProcessingStartedEvent

// LyriaGenerationConfig is an alias to the canonical definition in common-events.
type LyriaGenerationConfig = events.LyriaGenerationConfig

// AudioSessionConfig is an alias to the canonical definition in common-events.
type AudioSessionConfig = events.AudioSessionConfig

// JobSettings is an alias to the canonical definition in common-events.
type JobSettings = events.JobSettings

// PDFCreatedEvent is triggered when a PDF is uploaded.
type PDFCreatedEvent = events.PDFCreatedEvent

// PNGCreatedEvent is triggered when a PNG page is generated from a PDF.
type PNGCreatedEvent = events.PNGCreatedEvent

const NoSoundscapeDirective = events.NoSoundscapeDirective
