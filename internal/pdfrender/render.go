/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package pdfrender

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"

	"github.com/book-expert/logger"
)

// Define command constants.
const (
	// Default configuration values.
	DefaultDotsPerInch            = 200
	DefaultBlankFuzzPercent       = 5
	DefaultBlankNonWhiteThreshold = 0.005
)

// ErrPDFZeroOrNegativePages is returned when a PDF has invalid page count.
var ErrPDFZeroOrNegativePages = errors.New("pdf has zero or a negative number of pages")

// Options holds all configurable parameters for a Processor.
type Options struct {
	ProgressBarOutput      io.Writer
	DotsPerInch            int
	Workers                int
	BlankFuzzPercent       int
	BlankNonWhiteThreshold float64
}

// Processor encapsulates the logic for processing a batch of PDF files using Ghostscript.
type Processor struct {
	serviceLogger *logger.Logger
	configuration Options
}

// NewProcessor creates and initializes a new Processor with validated options.
func NewProcessor(options *Options, serviceLogger *logger.Logger) *Processor {
	applyDefaultConfiguration(options)

	return &Processor{
		configuration: *options,
		serviceLogger: serviceLogger,
	}
}

// applyDefaultConfiguration fills zero-value fields in Options with sensible defaults.
func applyDefaultConfiguration(options *Options) {
	options.DotsPerInch = resolvePositiveInteger(options.DotsPerInch, DefaultDotsPerInch)
	options.Workers = resolvePositiveInteger(options.Workers, runtime.NumCPU())

	options.BlankFuzzPercent = resolvePositiveInteger(
		options.BlankFuzzPercent,
		DefaultBlankFuzzPercent,
	)

	options.BlankNonWhiteThreshold = resolvePositiveFloat(
		options.BlankNonWhiteThreshold,
		DefaultBlankNonWhiteThreshold,
	)

	if options.ProgressBarOutput == nil {
		options.ProgressBarOutput = os.Stdout
	}
}

// resolvePositiveInteger returns the default value if the provided value is non-positive.
func resolvePositiveInteger(value, defaultValue int) int {
	if value <= 0 {
		return defaultValue
	}
	return value
}

// resolvePositiveFloat returns the default value if the provided value is non-positive.
func resolvePositiveFloat(value, defaultValue float64) float64 {
	if value <= 0 {
		return defaultValue
	}
	return value
}

// ProcessSinglePDFFromBytes converts a single PDF file from a byte slice to PNGs using Ghostscript.
func (processor *Processor) ProcessSinglePDFFromBytes(parentContext context.Context, pdfData []byte) ([][]byte, error) {
	pageProcessor := NewPageProcessor(processor.serviceLogger, processor.configuration.DotsPerInch)

	pngImages, processingError := pageProcessor.ProcessPagesFromBytes(parentContext, pdfData)
	if processingError != nil {
		return nil, processingError
	}

	return pngImages, nil
}
