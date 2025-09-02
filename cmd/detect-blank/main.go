// File: ./cmd/detect-blank/main.go
package main

import (
	"errors"
	"fmt"
	"image"
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
	maxColorValue    = 255
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

// parseAndValidateArguments processes the raw command-line arguments.
func parseAndValidateArguments(args []string) (arguments, error) {
	if len(args) != expectedArgCount {
		return arguments{}, fmt.Errorf(
			"expected 3 arguments, but got %d. Usage: <program> <filepath> <fuzz_percent> <threshold>: %w",
			len(args)-1,
			ErrInvalidArguments,
		)
	}

	fuzzPercent, err := strconv.Atoi(args[2])
	if err != nil {
		return arguments{}, fmt.Errorf(
			"invalid fuzz percentage '%s': %w",
			args[2],
			err,
		)
	}

	if fuzzPercent < 0 || fuzzPercent > 100 {
		return arguments{}, fmt.Errorf(
			"fuzz percentage must be between 0 and 100, got %d: %w",
			fuzzPercent,
			ErrInvalidFuzzPercent,
		)
	}

	threshold, err := strconv.ParseFloat(args[3], 64)
	if err != nil {
		return arguments{}, fmt.Errorf(
			"invalid non-white threshold '%s': %w",
			args[3],
			err,
		)
	}

	if threshold < 0 || threshold > 1.0 {
		return arguments{}, fmt.Errorf(
			"non-white threshold must be between 0.0 and 1.0, got %f: %w",
			threshold,
			ErrInvalidThreshold,
		)
	}

	return arguments{
		filePath:   args[1],
		fuzzFactor: float64(fuzzPercent) / percentToRatio,
		threshold:  threshold,
	}, nil
}

// imageHasContent opens an image file and determines if it contains non-white pixels
// above the specified threshold.
func imageHasContent(args arguments) (bool, error) {
	file, err := os.Open(args.filePath)
	if err != nil {
		return false, fmt.Errorf("could not open file %s: %w", args.filePath, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return false, fmt.Errorf(
			"could not decode image file %s: %w",
			args.filePath,
			err,
		)
	}

	bounds := img.Bounds()

	totalPixels := float64(bounds.Dx() * bounds.Dy())
	if totalPixels == 0 {
		return false, ErrImageZeroPixels
	}

	nonWhiteCount := 0.0
	// The fuzz threshold determines how close to pure white a color can be.
	// 255 is pure white for a color channel.
	whiteThreshold := uint32((1.0 - args.fuzzFactor) * maxColorValue)

	// Iterate over every pixel in the image.
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// The color.Color interface returns RGBA values as 16-bit
			// pre-multiplied alpha.
			// We need to scale them down to 8-bit for our comparison.
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := uint32(r>>8), uint32(g>>8), uint32(b>>8)

			if r8 < whiteThreshold || g8 < whiteThreshold ||
				b8 < whiteThreshold {

				nonWhiteCount++
			}
		}
	}

	// Calculate the ratio of non-white pixels.
	nonWhiteRatio := nonWhiteCount / totalPixels

	return nonWhiteRatio >= args.threshold, nil
}
