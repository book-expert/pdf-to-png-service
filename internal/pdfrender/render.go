// Package pdfrender provides PDF-to-PNG conversion functionality.
package pdfrender

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cheggaaa/pb/v3"
	"github.com/nnikolov3/logger"
)

var (
	// ErrInputPathRequired is returned when input path is not provided.
	ErrInputPathRequired = errors.New("input path is required")
	// ErrOutputPathRequired is returned when output path is not provided.
	ErrOutputPathRequired = errors.New("output path is required")
	// ErrPDFZeroOrNegativePages is returned when a PDF has invalid page count.
	ErrPDFZeroOrNegativePages = errors.New(
		"pdf has zero or a negative number of pages",
	)
)

// Options holds all configurable parameters for a Processor.
// This struct is used to initialize a new Processor with user-defined settings.
type Options struct {
	ProgressBarOutput      io.Writer
	InputPath              string
	OutputPath             string
	ProjectRoot            string
	DPI                    int
	Workers                int
	BlankFuzzPercent       int
	BlankNonWhiteThreshold float64
}

// Processor encapsulates the logic for processing a batch of PDF files.
type Processor struct {
	executor CommandExecutor
	log      *logger.Logger
	config   Options
}

// NewProcessor creates and initializes a new Processor with the given options and logger.
// It sets sensible defaults for any zero-value fields in the Options struct.
func NewProcessor(opts *Options, log *logger.Logger) *Processor {
	applyDefaultOptions(opts)

	return &Processor{
		config:   *opts,
		log:      log,
		executor: &defaultExecutor{}, // Use the real command executor by default.
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

// Process is the main entry point for starting the PDF-to-PNG conversion job.
// It discovers PDFs, builds the helper binary, and processes each file sequentially.
func (processor *Processor) Process(ctx context.Context) error {
	// Step 1: Validate the configuration before starting any work.
	err := processor.validateConfig()
	if err != nil {
		return err
	}

	// Step 2: Prepare helper tooling required for processing.
	err = processor.prepareTools(ctx)
	if err != nil {
		return err
	}

	// Step 3: Discover all PDF files in the input directory.
	pdfPaths, err := processor.discoverInputPDFs()
	if err != nil {
		return err
	}

	// Step 4: Process each discovered PDF file.
	processor.log.Info("Found %d PDF(s) to process.", len(pdfPaths))

	return processor.processAllPDFs(ctx, pdfPaths)
}

// prepareTools ensures required helper binaries are available before processing.
func (processor *Processor) prepareTools(ctx context.Context) error {
	buildErr := ensureDetectBlankBinary(
		ctx,
		processor.config.ProjectRoot,
		processor.log,
	)
	if buildErr != nil {
		return fmt.Errorf("could not prepare blank detection tool: %w", buildErr)
	}

	return nil
}

// discoverInputPDFs discovers input PDFs and validates non-empty result.
func (processor *Processor) discoverInputPDFs() ([]string, error) {
	pdfPaths, discoveryErr := DiscoverPDFs(processor.config.InputPath)
	if discoveryErr != nil {
		return nil, fmt.Errorf("failed to discover PDFs: %w", discoveryErr)
	}

	if len(pdfPaths) == 0 {
		return nil, fmt.Errorf(
			"no PDF files found in %s: %w",
			processor.config.InputPath,
			os.ErrNotExist,
		)
	}

	return pdfPaths, nil
}

// validateConfig checks if the essential configuration options have been provided.
func (processor *Processor) validateConfig() error {
	if processor.config.InputPath == "" {
		return ErrInputPathRequired
	}

	if processor.config.OutputPath == "" {
		return ErrOutputPathRequired
	}

	return nil
}

// processAllPDFs iterates through a list of PDF file paths and processes each one.
// It uses a progress bar to show the overall progress.
func (processor *Processor) processAllPDFs(ctx context.Context, pdfPaths []string) error {
	mainProgressBar := pb.New(len(pdfPaths)).
		SetTemplateString(`{{ bar . " " "━" "━" " " " "}} {{percent .}} {{rtime .}}`).
		SetWriter(processor.config.ProgressBarOutput).
		Start()
	defer mainProgressBar.Finish()

	for _, pdfPath := range pdfPaths {
		mainProgressBar.Increment()
		processor.log.Info("Starting processing for: %s", filepath.Base(pdfPath))

		processErr := processor.processOnePDF(ctx, pdfPath)
		if processErr != nil {
			processor.log.Error(
				"Failed to process %s: %v",
				filepath.Base(pdfPath),
				processErr,
			)
			// Continue to the next file even if one fails.
		} else {
			processor.log.Success("Successfully processed %s", filepath.Base(pdfPath))
		}
	}

	return nil
}

// processOnePDF handles the conversion of a single PDF file.
func (processor *Processor) processOnePDF(ctx context.Context, pdfPath string) error {
	// Determine the total number of pages in the PDF.
	pageCount, pageCountErr := processor.getPDFPages(ctx, pdfPath)
	if pageCountErr != nil {
		return fmt.Errorf("could not get page count: %w", pageCountErr)
	}

	if pageCount <= 0 {
		return ErrPDFZeroOrNegativePages
	}

	// Create the specific output directory for this PDF's PNGs.
	outputDir, setupErr := setupOutputDirectory(processor.config.OutputPath, pdfPath)
	if setupErr != nil {
		return fmt.Errorf("could not set up output directory: %w", setupErr)
	}

	processor.log.Info("Rendering %d pages into %s", pageCount, outputDir)

	// Create and run a PageProcessor to handle the concurrent rendering.
	pageProc := newPageProcessor(processor, outputDir)

	return pageProc.processPages(ctx, pdfPath, pageCount)
}
