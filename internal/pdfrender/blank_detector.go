/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

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
func (processor *Processor) IsImageBlank(imageData []byte) (bool, error) {
	decodedImage, _, decodeError := image.Decode(bytes.NewReader(imageData))
	if decodeError != nil {
		return false, fmt.Errorf("could not decode image data: %w", decodeError)
	}

	bounds := decodedImage.Bounds()
	totalPixels := float64(bounds.Dx() * bounds.Dy())

	if totalPixels == 0 {
		return false, ErrImageZeroPixels
	}

	fuzzRatio := float64(processor.configuration.BlankFuzzPercent) / PercentageDivisor
	whiteThreshold := uint32((1.0 - fuzzRatio) * MaxColor8Bit)

	nonWhiteCount := countNonWhitePixels(decodedImage, whiteThreshold)
	nonWhiteRatio := nonWhiteCount / totalPixels

	return nonWhiteRatio < processor.configuration.BlankNonWhiteThreshold, nil
}

// countNonWhitePixels iterates through the image to count pixels that differ from white.
func countNonWhitePixels(decodedImage image.Image, whiteThreshold uint32) float64 {
	nonWhiteCount := 0.0
	bounds := decodedImage.Bounds()

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
func isPixelNonWhite(pixelColor color.Color, whiteThreshold uint32) bool {
	redComponent, greenComponent, blueComponent, _ := pixelColor.RGBA()

	red8Bit := redComponent >> BitsToShift16To8
	green8Bit := greenComponent >> BitsToShift16To8
	blue8Bit := blueComponent >> BitsToShift16To8

	return red8Bit < whiteThreshold || green8Bit < whiteThreshold || blue8Bit < whiteThreshold
}
