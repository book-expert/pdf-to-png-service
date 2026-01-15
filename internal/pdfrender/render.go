/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package pdfrender

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/book-expert/logger"
)

// Define command constants to avoid magic strings and allow easy updates.
const (
	CommandPDFInfo     = "pdfinfo"
	CommandGhostScript = "ghostscript"

	// Default configuration values.
	DefaultDotsPerInch            = 200
	DefaultBlankFuzzPercent       = 5
	DefaultBlankNonWhiteThreshold = 0.005
)

var (
	// ErrPDFZeroOrNegativePages is returned when a PDF has invalid page count.
	ErrPDFZeroOrNegativePages = errors.New("pdf has zero or a negative number of pages")
	// ErrPageNumberMustBePositive is returned when a page number is zero or negative.
	ErrPageNumberMustBePositive = errors.New("page number must be positive")
	// ErrCouldNotParsePagesLine is returned when pdfinfo output cannot be parsed.
	ErrCouldNotParsePagesLine = errors.New("could not parse 'Pages:' line from pdfinfo output")
)

// Options holds all configurable parameters for a Processor.
type Options struct {
	ProgressBarOutput      io.Writer
	DotsPerInch            int
	Workers                int
	BlankFuzzPercent       int
	BlankNonWhiteThreshold float64
}

// Processor encapsulates the logic for processing a batch of PDF files.
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

// ProcessSinglePDFFromBytes converts a single PDF file from a byte slice to PNGs.
func (processor *Processor) ProcessSinglePDFFromBytes(parentContext context.Context, pdfData []byte) ([][]byte, error) {
	pageCount, lookupError := processor.getPDFPageCount(parentContext, pdfData)
	if lookupError != nil {
		return nil, fmt.Errorf("could not get page count: %w", lookupError)
	}

	if pageCount <= 0 {
		return nil, ErrPDFZeroOrNegativePages
	}

	processor.serviceLogger.Infof("Rendering %d pages", pageCount)

	pageProcessor := NewPageProcessor(processor.serviceLogger, "")

	pngImages, processingError := pageProcessor.ProcessPagesFromBytes(parentContext, pdfData, pageCount)
	if processingError != nil {
		return nil, processingError
	}

	return pngImages, nil
}

// getPDFPageCount executes the `pdfinfo` command to determine the number of pages.
func (processor *Processor) getPDFPageCount(parentContext context.Context, pdfData []byte) (int, error) {
	pdfInfoCommand := exec.CommandContext(parentContext, CommandPDFInfo, "-")
	pdfInfoCommand.Stdin = bytes.NewReader(pdfData)

	commandOutput, executionError := pdfInfoCommand.CombinedOutput()
	if executionError != nil {
		return 0, fmt.Errorf(
			"pdfinfo execution failed: %w. Output: %s",
			executionError,
			string(commandOutput),
		)
	}

	return parsePdfInfoOutput(string(commandOutput))
}

// parsePdfInfoOutput scans the text output from the `pdfinfo` command.
func parsePdfInfoOutput(output string) (int, error) {
	outputScanner := bufio.NewScanner(strings.NewReader(output))

	for outputScanner.Scan() {
		lineText := outputScanner.Text()

		if strings.HasPrefix(lineText, "Pages:") {
			lineParts := strings.Fields(lineText)
			if len(lineParts) < 2 {
				return 0, ErrCouldNotParsePagesLine
			}

			pageCount, conversionError := strconv.Atoi(lineParts[1])
			if conversionError != nil {
				return 0, ErrCouldNotParsePagesLine
			}
			return pageCount, nil
		}
	}

	return 0, ErrCouldNotParsePagesLine
}
