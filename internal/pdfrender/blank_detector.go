package pdfrender

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // Import the PNG decoder.
)

const (
	maxColorValue  = 255.0
	percentToRatio = 100.0
)

// ErrImageZeroPixels is returned when an image has no pixels.
var ErrImageZeroPixels = errors.New("image has zero pixels")

// IsImageBlank checks if the provided image data represents a blank (mostly white) image.
// It uses the configured fuzz percent and threshold options.
func (processor *Processor) IsImageBlank(imageData []byte) (bool, error) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return false, fmt.Errorf("could not decode image data: %w", err)
	}

	bounds := img.Bounds()
	totalPixels := float64(bounds.Dx() * bounds.Dy())
	if totalPixels == 0 {
		return false, ErrImageZeroPixels
	}

	fuzzFactor := float64(processor.config.BlankFuzzPercent) / percentToRatio
	nonWhiteCount := countNonWhitePixels(img, fuzzFactor)
	nonWhiteRatio := nonWhiteCount / totalPixels

	return nonWhiteRatio < processor.config.BlankNonWhiteThreshold, nil
}

// countNonWhitePixels counts the number of pixels that are considered non-white.
func countNonWhitePixels(img image.Image, fuzzFactor float64) float64 {
	nonWhiteCount := 0.0
	whiteThreshold := uint32((1.0 - fuzzFactor) * maxColorValue)

	// This function (a closure) is the "visitor".
	// It will be executed for each pixel.
	pixelVisitor := func(c color.Color) {
		if isNonWhite(c, whiteThreshold) {
			nonWhiteCount++
		}
	}

	visitPixels(img, pixelVisitor)

	return nonWhiteCount
}

// visitPixels iterates over every pixel in the image and calls the visitor function.
func visitPixels(img image.Image, visitor func(c color.Color)) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			visitor(img.At(x, y))
		}
	}
}

// isNonWhite checks if a single pixel's color is considered non-white.
func isNonWhite(c color.Color, whiteThreshold uint32) bool {
	// The color.Color interface returns RGBA values as 16-bit pre-multiplied alpha.
	// We scale them down to 8-bit for our comparison.
	r, g, b, _ := c.RGBA()

	const bitsToShift = 8

	r8, g8, b8 := r>>bitsToShift, g>>bitsToShift, b>>bitsToShift

	return r8 < whiteThreshold || g8 < whiteThreshold || b8 < whiteThreshold
}
