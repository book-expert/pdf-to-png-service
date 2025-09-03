// Command detect-blank analyzes a PNG image and exits with a code indicating
// whether the image is blank (mostly white) or contains content.
//
// Usage: detect-blank <filepath> <fuzz_percent> <non_white_threshold>
// - fuzz_percent: 0..100 tolerated deviation from pure white (higher = more tolerant)
// - non_white_threshold: 0.0..1.0 minimum ratio of non-white pixels to consider content
//
// Exit codes:
//
//	0 = blank image
//	1 = image has content
//	2 = error (bad args, cannot open/parse image, etc.)
package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // Import the PNG decoder.
	"os"
	"strconv"
)

var (
	ErrInvalidArguments   = errors.New("invalid number of arguments")
	ErrInvalidFuzzPercent = errors.New("fuzz percentage must be between 0 and 100")
	ErrInvalidThreshold   = errors.New(
		"non-white threshold must be between 0.0 and 1.0",
	)
	ErrImageZeroPixels = errors.New("image has zero pixels")
)

// arguments holds the parsed and validated command-line arguments.
type arguments struct {
	filePath   string
	fuzzFactor float64
	threshold  float64
}

// Exit codes used by this tool to communicate with the main application.
const (
	exitCodeBlank    = 0 // The image is blank.
	exitCodeNotBlank = 1 // The image has content.
	exitCodeError    = 2 // An error occurred (e.g., bad arguments, file not found).

	// Command line argument constants.
	expectedArgCount = 4
	percentToRatio   = 100.0
	maxColorValue    = 255.0
)

func main() {
	// Step 1: Parse and validate the command-line arguments.
	args, err := parseAndValidateArguments(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Argument error: %v\n", err)
		os.Exit(exitCodeError)
	}

	// Step 2: Analyze the image file.
	hasContent, err := imageHasContent(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Image analysis error: %v\n", err)
		os.Exit(exitCodeError)
	}

	// Step 3: Exit with the appropriate code based on the analysis.
	if hasContent {
		os.Exit(exitCodeNotBlank)
	}

	os.Exit(exitCodeBlank)
}

// --- Argument Parsing ---

// parseAndValidateArguments processes the raw command-line arguments.
func parseAndValidateArguments(args []string) (arguments, error) {
	if len(args) != expectedArgCount {
		return arguments{}, fmt.Errorf(
			"expected 3 arguments, but got %d. Usage: <program> <filepath> <fuzz_percent> <threshold>: %w",
			len(args)-1,
			ErrInvalidArguments,
		)
	}

	fuzzFactor, err := parseFuzz(args[2])
	if err != nil {
		return arguments{}, err
	}

	threshold, err := parseThreshold(args[3])
	if err != nil {
		return arguments{}, err
	}

	return arguments{
		filePath:   args[1],
		fuzzFactor: fuzzFactor,
		threshold:  threshold,
	}, nil
}

// parseFuzz parses and validates the fuzz percentage string.
func parseFuzz(fuzzStr string) (float64, error) {
	fuzzPercent, err := strconv.Atoi(fuzzStr)
	if err != nil {
		return 0, fmt.Errorf("invalid fuzz percentage '%s': %w", fuzzStr, err)
	}

	if fuzzPercent < 0 || fuzzPercent > 100 {
		return 0, fmt.Errorf(
			"fuzz percentage must be between 0 and 100, got %d: %w",
			fuzzPercent,
			ErrInvalidFuzzPercent,
		)
	}

	return float64(fuzzPercent) / percentToRatio, nil
}

// parseThreshold parses and validates the non-white threshold string.
func parseThreshold(thresholdStr string) (float64, error) {
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"invalid non-white threshold '%s': %w",
			thresholdStr,
			err,
		)
	}

	if threshold < 0 || threshold > 1.0 {
		return 0, fmt.Errorf(
			"non-white threshold must be between 0.0 and 1.0, got %f: %w",
			threshold,
			ErrInvalidThreshold,
		)
	}

	return threshold, nil
}

// --- Image Analysis ---

// imageHasContent orchestrates the image analysis process.
func imageHasContent(args arguments) (bool, error) {
	img, err := loadImage(args.filePath)
	if err != nil {
		return false, err
	}

	bounds := img.Bounds()

	totalPixels := float64(bounds.Dx() * bounds.Dy())
	if totalPixels == 0 {
		return false, ErrImageZeroPixels
	}

	nonWhiteCount := countNonWhitePixels(img, args.fuzzFactor)
	nonWhiteRatio := nonWhiteCount / totalPixels

	return nonWhiteRatio >= args.threshold, nil
}

// loadImage opens and decodes an image file.
func loadImage(filePath string) (image.Image, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open file %s: %w", filePath, err)
	}

	defer func() {
		cerr := file.Close()
		if cerr != nil {
			_, _ = fmt.Fprintf(
				os.Stderr,
				"failed to close file %s: %v\n",
				filePath,
				cerr,
			)
		}
	}()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf(
			"could not decode image file %s: %w",
			filePath,
			err,
		)
	}

	return img, nil
}

// ** countNonWhitePixels now has a cognitive complexity of 1 **
// It defines the logic to apply to each pixel and passes it to the iterator.
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

// ** visitPixels now contains the complex logic (nested loops) **
// Its cognitive complexity is 3, which is below the threshold.
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
