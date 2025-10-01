package main

import (
	"testing"
	"time"

	"github.com/book-expert/events"
)

func TestBuildPNGCreatedEvent_PreservesAugmentation(t *testing.T) {
	t.Parallel()

	preferences := &events.AugmentationPreferences{
		Commentary: events.AugmentationCommentarySettings{Enabled: true, CustomInstructions: "Call out charts."},
		Summary: events.AugmentationSummarySettings{
			Enabled:            true,
			Placement:          events.SummaryPlacementTop,
			CustomInstructions: "Open with the main thesis.",
		},
	}

	header := events.EventHeader{
		Timestamp:  time.Now().UTC(),
		WorkflowID: "wf",
		UserID:     "user",
		TenantID:   "tenant",
		EventID:    "evt",
	}

	event := buildPNGCreatedEvent(header, "png-key", 10, 2, preferences)

	if event.Augmentation == nil {
		t.Fatalf("expected augmentation preferences to be set")
	}

	if event.Augmentation != preferences {
		t.Fatalf("augmentation pointer should be preserved; got %p want %p", event.Augmentation, preferences)
	}

	if event.PageNumber != 2 || event.TotalPages != 10 {
		t.Fatalf("unexpected pagination metadata: %+v", event)
	}

	if event.PNGKey != "png-key" {
		t.Fatalf("unexpected PNG key: %s", event.PNGKey)
	}

	if event.Header != header {
		t.Fatalf("header mismatch: got %+v want %+v", event.Header, header)
	}
}
