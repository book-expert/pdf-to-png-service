/*
GOLDEN RULES & DEVELOPER MANIFESTO (THE NORTH STAR)
--------------------------------------------------------------------------------
"Work is love made visible. And if you cannot work with love but only with
distaste, it is better that you should leave your work and sit at the gate of
the temple and take alms of those who work with joy." — Kahlil Gibran

1.  LOVE AND CARE (Primary Driver)
    - This is a craft. Build with pride, honesty, and kindness.
    - If you put love in your work, you build something deserving of love.
    - Be helpful: Code is read more than written; optimize for the reader.

2.  WRITE WHAT YOU MEAN (Explicit > Implicit)
    - Use WHOLE WORDS: `RequestIdentifier` not `ReqID`.
    - No magic numbers: Move application settings to `project.toml`.
    - Secure by design: Keep API keys and secrets strictly in `.env`.
    - No ambiguity: If you assume something, document it.

3.  SIMPLE IS EFFICIENT (Minimal Viable Elegance)
    - Avoid over-engineering. Small interfaces, clear structs.
    - If a design requires a hack, stop. Redesign it with elegance.
    - Lean, Clean, Mean: Delete dead code immediately.

4.  NO BASELESS ASSUMPTIONS (Scientific Rigor)
    - Do not guess. Base decisions on documentation and proven patterns.
    - If you do not know, ask or verify.

5.  NON-BLOCKING & ROBUST
    - Never block the main goroutine. Use Context for cancellation.
    - Handle errors explicitly: Don't just return them, wrap them with context.

--------------------------------------------------------------------------------
EXAMPLES OF "LOVE AND CARE" IN THIS CONTEXT:
--------------------------------------------------------------------------------
(A) NAMING
    Indifferent:  func Gen(t string, v string)
    With Love:    func GenerateSoundscape(ctx context.Context, textPrompt string, voiceID string)
    *Why: The Agent reading this next year will know exactly what it does and that it is cancellable.*

(B) CONFIGURATION
    Indifferent:  const Timeout = 30 // Hardcoded
    With Love:    config.App.TimeoutSeconds // Loaded from project.toml
    *Why: Allows behavior tuning without recompiling or touching the codebase.*

(C) ERROR HANDLING
    Indifferent:  if err != nil { return err }
    With Love:    if err != nil { return fmt.Errorf("failed to initialize vox engine: %w", err) }
    *Why: Wrapping the error gives the user the 'trace of breadcrumbs' they need to fix it. That is kindness.*
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
	InputPath              string // Kept for struct compatibility but unused in main path
	OutputPath             string // Kept for struct compatibility but unused in main path
	ProjectRoot            string // Kept for struct compatibility but unused in main path
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
