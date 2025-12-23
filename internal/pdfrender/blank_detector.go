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

package pdfrender

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // Import the PNG decoder for side-effect registration.
)

const (
	// MaxColor8Bit represents the maximum value for an 8-bit color channel (0-255).
	MaxColor8Bit = 255.0
	// PercentageDivisor is used to convert an integer percentage (0-100) to a ratio (0.0-1.0).
	PercentageDivisor = 100.0
	// BitsToShift16To8 is the shift amount to convert Go's 16-bit color to 8-bit.
	BitsToShift16To8 = 8
)

// ErrImageZeroPixels is returned when an image has no pixels (zero width or height).
var ErrImageZeroPixels = errors.New("image has zero pixels")

// IsImageBlank checks if the provided image data represents a blank (mostly white) image.
//
// Why: Detecting blank pages allows us to filter them out of the final output, saving storage
// and clean up the resulting document set.
func (processor *Processor) IsImageBlank(imageData []byte) (bool, error) {
	decodedImage, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return false, fmt.Errorf("could not decode image data: %w", err)
	}

	bounds := decodedImage.Bounds()
	totalPixels := float64(bounds.Dx() * bounds.Dy())

	// Guard clause against division by zero.
	if totalPixels == 0 {
		return false, ErrImageZeroPixels
	}

	// Calculate thresholds.
	// Why: We convert the fuzz percent (e.g., 5%) into a pixel value threshold (e.g., 242).
	// Any pixel channel below this value is considered "non-white".
	fuzzRatio := float64(processor.config.BlankFuzzPercent) / PercentageDivisor
	whiteThreshold := uint32((1.0 - fuzzRatio) * MaxColor8Bit)

	nonWhiteCount := countNonWhitePixels(decodedImage, whiteThreshold)
	nonWhiteRatio := nonWhiteCount / totalPixels

	return nonWhiteRatio < processor.config.BlankNonWhiteThreshold, nil
}

// countNonWhitePixels iterates through the image to count pixels that differ from white.
//
// Why: We simplified this from a "Visitor" pattern to a direct nested loop.
// The overhead of function calls per pixel in Go is significant for large images;
// a direct loop is simpler (KISS) and more performant.
func countNonWhitePixels(decodedImage image.Image, whiteThreshold uint32) float64 {
	nonWhiteCount := 0.0
	bounds := decodedImage.Bounds()

	// Iterate over every pixel.
	for yAxis := bounds.Min.Y; yAxis < bounds.Max.Y; yAxis++ {
		for xAxis := bounds.Min.X; xAxis < bounds.Max.X; xAxis++ {
			pixelColor := decodedImage.At(xAxis, yAxis)

			if isPixelNonWhite(pixelColor, whiteThreshold) {
				nonWhiteCount++
			}
		}
	}

	return nonWhiteCount
}

// isPixelNonWhite determines if a single pixel is dark enough to matter.
//
// Why: Go's image library returns 16-bit color components (0-65535).
// We shift right by 8 to get standard 8-bit values (0-255) for intuitive comparison.
func isPixelNonWhite(pixelColor color.Color, whiteThreshold uint32) bool {
	redComponent, greenComponent, blueComponent, _ := pixelColor.RGBA()

	red8Bit := redComponent >> BitsToShift16To8
	green8Bit := greenComponent >> BitsToShift16To8
	blue8Bit := blueComponent >> BitsToShift16To8

	// If any channel is darker than the threshold, the pixel is "non-white" (has content).
	return red8Bit < whiteThreshold || green8Bit < whiteThreshold || blue8Bit < whiteThreshold
}
