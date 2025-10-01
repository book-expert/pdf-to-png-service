package main

import "github.com/book-expert/events"

// buildPNGCreatedEvent assembles the PNGCreatedEvent payload, threading through
// the end-user augmentation preferences unmodified.
func buildPNGCreatedEvent(
	header events.EventHeader,
	pngKey string,
	totalPages int,
	pageNumber int,
	augmentation *events.AugmentationPreferences,
) events.PNGCreatedEvent {
	return events.PNGCreatedEvent{
		Header:       header,
		PNGKey:       pngKey,
		PageNumber:   pageNumber,
		TotalPages:   totalPages,
		Augmentation: augmentation,
	}
}
