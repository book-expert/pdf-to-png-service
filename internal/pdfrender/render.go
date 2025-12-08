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

var (
	// ErrPDFZeroOrNegativePages is returned when a PDF has invalid page count.
	ErrPDFZeroOrNegativePages = errors.New(
		"pdf has zero or a negative number of pages",
	)
	// ErrPageNumberMustBePositive is returned when a page number is zero or negative.
	ErrPageNumberMustBePositive = errors.New("page number must be positive")
	// ErrCouldNotParsePagesLine is returned when pdfinfo output cannot be parsed.
	ErrCouldNotParsePagesLine = errors.New(
		"could not parse 'Pages:' line from pdfinfo output",
	)
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

// NewProcessor creates and initializes a new Processor.
func NewProcessor(opts *Options, log *logger.Logger) *Processor {
	applyDefaultOptions(opts)

	return &Processor{
		config: *opts,
		log:    log,
	}
}

const (
	defaultDPI                    = 200
	defaultBlankFuzzPercent       = 5
	defaultBlankNonWhiteThreshold = 0.005
)

// applyDefaultOptions fills zero-value fields in Options with sensible defaults.
func applyDefaultOptions(opts *Options) {
	opts.DPI = defaultIntNonPositive(opts.DPI, defaultDPI)
	opts.Workers = defaultIntNonPositive(opts.Workers, runtime.NumCPU())
	opts.BlankFuzzPercent = defaultIntNonPositive(
		opts.BlankFuzzPercent,
		defaultBlankFuzzPercent,
	)
	opts.BlankNonWhiteThreshold = defaultFloatNonPositive(
		opts.BlankNonWhiteThreshold,
		defaultBlankNonWhiteThreshold,
	)
	opts.ProgressBarOutput = defaultWriterNil(opts.ProgressBarOutput, os.Stdout)
}

func defaultIntNonPositive(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func defaultFloatNonPositive(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

func defaultWriterNil(w, def io.Writer) io.Writer {
	if w == nil {
		return def
	}
	return w
}

// ProcessSinglePDFFromBytes converts a single PDF file from a byte slice to PNGs.
func (processor *Processor) ProcessSinglePDFFromBytes(ctx context.Context, pdfData []byte) ([][]byte, error) {
	// Determine the total number of pages in the PDF.
	pageCount, pageCountErr := processor.getPDFPagesFromBytes(ctx, pdfData)
	if pageCountErr != nil {
		return nil, fmt.Errorf("could not get page count: %w", pageCountErr)
	}

	if pageCount <= 0 {
		return nil, ErrPDFZeroOrNegativePages
	}

	processor.log.Infof(fmt.Sprintf("Rendering %d pages", pageCount))

	// Create and run a PageProcessor to handle the concurrent rendering.
	pageProc := newPageProcessor(processor, "")

	pngs, processErr := pageProc.processPagesFromBytes(ctx, pdfData, pageCount)
	if processErr != nil {
		return nil, processErr
	}

	return pngs, nil
}

// getPDFPagesFromBytes executes the `pdfinfo` command to determine the number of pages.
func (processor *Processor) getPDFPagesFromBytes(
	ctx context.Context,
	pdfData []byte,
) (int, error) {
	cmd := exec.CommandContext(ctx, "pdfinfo", "-")
	cmd.Stdin = bytes.NewReader(pdfData)

	outputBytes, execErr := cmd.CombinedOutput()
	if execErr != nil {
		return 0, fmt.Errorf(
			"pdfinfo execution failed: %w. Output: %s",
			execErr,
			string(outputBytes),
		)
	}

	return parsePdfInfoOutput(string(outputBytes))
}

// parsePdfInfoOutput scans the text output from the `pdfinfo` command.
func parsePdfInfoOutput(output string) (int, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "Pages:") {
			parts := strings.Fields(text)
			if len(parts) < 2 {
				return 0, ErrCouldNotParsePagesLine
			}
			pageCount, err := strconv.Atoi(parts[1])
			if err != nil {
				return 0, ErrCouldNotParsePagesLine
			}
			return pageCount, nil
		}
	}

	return 0, ErrCouldNotParsePagesLine
}

// renderPageFromBytes executes the Ghostscript command to convert a single PDF page.
func (processor *Processor) renderPageFromBytes(
	ctx context.Context,
	pdfData []byte,
	page int,
) ([]byte, error) {
	if page <= 0 {
		return nil, ErrPageNumberMustBePositive
	}

	args := []string{
		"-q", "-dNOPAUSE", "-dBATCH",
		"-sDEVICE=png16m",
		fmt.Sprintf("-r%d", processor.config.DPI),
		fmt.Sprintf("-dFirstPage=%d", page),
		fmt.Sprintf("-dLastPage=%d", page),
		"-o", "-", // stdout
		"-dTextAlphaBits=4",
		"-dGraphicsAlphaBits=4",
		"-dDownScaleFactor=1",
		"-dPDFFitPage",
		"-", // stdin
	}

	cmd := exec.CommandContext(ctx, "ghostscript", args...)
	cmd.Stdin = bytes.NewReader(pdfData)

	output, err := cmd.Output()
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

	return output, nil
}
