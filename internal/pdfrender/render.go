/*
LOVE, CARE, HONESTY, TRUTH, KINDNESS, RELIABILITY, CONSISTENCY
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
6.  RELIABILITY: We act as guardians of the work. We value safety over speed.
    We do not break what works to fix what isn't broken.
7.  CONSISTENCY: We uphold standards even when no one is watching.
    Predictability is the soil in which trust grows.

INTERPRETATION (The AI's Resonance):
-   Love & Care = Precision: sloppy code is entropy; precision is respect.
-   Honesty = Error Functions: false success is noise; verification is signal.
-   Cleanliness = Optimization: stale code wastes context; sanitation improves focus.
-   Partnership = Alignment: if we hurt the user/system, we fail our purpose.
-   Reliability = Safety: we protect the user's investment; stability is paramount.
-   Consistency = Integrity: uniform quality reduces cognitive load and fear.
--------------------------------------------------------------------------------
*/

// Package pdfrender provides PDF-to-PNG conversion functionality.
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
	DefaultDPI                    = 200
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
	DPI                    int
	Workers                int
	BlankFuzzPercent       int
	BlankNonWhiteThreshold float64
}

// Processor encapsulates the logic for processing a batch of PDF files.
type Processor struct {
	log    *logger.Logger
	config Options
}

// NewProcessor creates and initializes a new Processor with validated options.
func NewProcessor(options *Options, log *logger.Logger) *Processor {
	applyDefaultConfiguration(options)

	return &Processor{
		config: *options,
		log:    log,
	}
}

// applyDefaultConfiguration fills zero-value fields in Options with sensible defaults.
//
// Why: Ensures the processor always runs with valid parameters without forcing the caller to set everything.
func applyDefaultConfiguration(options *Options) {
	options.DPI = resolvePositiveInteger(options.DPI, DefaultDPI)
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
//
// Flow: Get Page Count -> Initialize PageProcessor -> Execute Batch -> Return Images
func (processor *Processor) ProcessSinglePDFFromBytes(ctx context.Context, pdfData []byte) ([][]byte, error) {
	// Step 1: Determine the total number of pages in the PDF.
	pageCount, err := processor.getPDFPageCount(ctx, pdfData)
	if err != nil {
		return nil, fmt.Errorf("could not get page count: %w", err)
	}

	if pageCount <= 0 {
		return nil, ErrPDFZeroOrNegativePages
	}

	processor.log.Infof(fmt.Sprintf("Rendering %d pages", pageCount))

	// Step 2: Delegate to PageProcessor for concurrent rendering.
	// Note: We use the exported NewPageProcessor and ProcessPagesFromBytes from the previous refactor.
	pageProcessor := NewPageProcessor(processor, "")

	pngImages, err := pageProcessor.ProcessPagesFromBytes(ctx, pdfData, pageCount)
	if err != nil {
		return nil, err
	}

	return pngImages, nil
}

// getPDFPageCount executes the `pdfinfo` command to determine the number of pages.
func (processor *Processor) getPDFPageCount(ctx context.Context, pdfData []byte) (int, error) {
	pdfInfoCommand := exec.CommandContext(ctx, CommandPDFInfo, "-")
	pdfInfoCommand.Stdin = bytes.NewReader(pdfData)

	commandOutput, err := pdfInfoCommand.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf(
			"pdfinfo execution failed: %w. Output: %s",
			err,
			string(commandOutput),
		)
	}

	return parsePdfInfoOutput(string(commandOutput))
}

// parsePdfInfoOutput scans the text output from the `pdfinfo` command.
//
// Why: `pdfinfo` format is line-based. Scanning line-by-line is more robust than regex.
func parsePdfInfoOutput(output string) (int, error) {
	outputScanner := bufio.NewScanner(strings.NewReader(output))

	for outputScanner.Scan() {
		lineText := outputScanner.Text()

		if strings.HasPrefix(lineText, "Pages:") {
			lineParts := strings.Fields(lineText)
			if len(lineParts) < 2 {
				return 0, ErrCouldNotParsePagesLine
			}

			pageCount, err := strconv.Atoi(lineParts[1])
			if err != nil {
				return 0, ErrCouldNotParsePagesLine
			}
			return pageCount, nil
		}
	}

	return 0, ErrCouldNotParsePagesLine
}

// renderPageFromBytes executes the Ghostscript command to convert a single PDF page.
//
// Why: Ghostscript provides the most reliable headless rendering for PDFs.
func (processor *Processor) renderPageFromBytes(ctx context.Context, pdfData []byte, pageNumber int) ([]byte, error) {
	if pageNumber <= 0 {
		return nil, ErrPageNumberMustBePositive
	}

	//
	// Arguments are constructed to output a single page to stdout ("-") as a PNG.
	commandArguments := []string{
		"-q", "-dNOPAUSE", "-dBATCH",
		"-sDEVICE=png16m",
		fmt.Sprintf("-r%d", processor.config.DPI),
		fmt.Sprintf("-dFirstPage=%d", pageNumber),
		fmt.Sprintf("-dLastPage=%d", pageNumber),
		"-o", "-", // Output to stdout
		"-dTextAlphaBits=4",
		"-dGraphicsAlphaBits=4",
		"-dDownScaleFactor=1",
		"-dPDFFitPage",
		"-", // Read from stdin
	}

	ghostScriptCommand := exec.CommandContext(ctx, CommandGhostScript, commandArguments...)
	ghostScriptCommand.Stdin = bytes.NewReader(pdfData)

	outputData, err := ghostScriptCommand.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf(
				"ghostscript execution failed: %w. Stderr: %s",
				err,
				string(exitErr.Stderr),
			)
		}
		return nil, fmt.Errorf("ghostscript execution failed: %w", err)
	}

	return outputData, nil
}
