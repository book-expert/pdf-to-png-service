// File: ./pdfrender/render.go
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

// Options holds all configurable parameters for a Processor.
// This struct is used to initialize a new Processor with user-defined settings.
type Options struct {
	// InputPath is the source directory containing PDF files to process.
	InputPath string
	// OutputPath is the destination directory where PNGs will be saved.
	OutputPath string
	// ProjectRoot is the root directory of the application. It's used to
	// locate and build the 'detect-blank' helper binary.
	ProjectRoot string
	// DPI specifies the resolution (dots per inch) for the rendered PNGs.
	// A higher DPI results in a higher-resolution image. Defaults to 200.
	DPI int
	// Workers specifies the number of concurrent goroutines to use for rendering
	// pages. Defaults to the number of available CPU cores.
	Workers int
	// BlankFuzzPercent determines the tolerance for a pixel to be considered "white".
	// A value of 5 means any color channel within 5% of pure white (255) is
	// treated as white. This helps with off-white pages. Defaults to 5.
	BlankFuzzPercent int
	// BlankNonWhiteThreshold is the minimum ratio of non-white pixels required
	// for a page to be considered "not blank". A small value like 0.001 means
	// even a tiny speck will save the page from being deleted. Defaults to 0.005.
	BlankNonWhiteThreshold float64
	// ProgressBarOutput is the writer where the progress bar will be rendered.
	// Defaults to os.Stdout. Can be set to io.Discard to disable it for tests.
	ProgressBarOutput io.Writer
}

// Processor encapsulates the logic for processing a batch of PDF files.
type Processor struct {
	config   Options
	log      *logger.Logger
	executor CommandExecutor
}

// NewProcessor creates and initializes a new Processor with the given options and logger.
// It sets sensible defaults for any zero-value fields in the Options struct.
func NewProcessor(opts Options, log *logger.Logger) *Processor {
	if opts.DPI <= 0 {
		opts.DPI = 200
	}
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}
	if opts.BlankFuzzPercent <= 0 {
		opts.BlankFuzzPercent = 5
	}
	if opts.BlankNonWhiteThreshold <= 0 {
		opts.BlankNonWhiteThreshold = 0.005
	}
	if opts.ProgressBarOutput == nil {
		opts.ProgressBarOutput = os.Stdout
	}

	return &Processor{
		config:   opts,
		log:      log,
		executor: &defaultExecutor{}, // Use the real command executor by default.
	}
}

// Process is the main entry point for starting the PDF-to-PNG conversion job.
// It discovers PDFs, builds the helper binary, and processes each file sequentially.
func (processor *Processor) Process(ctx context.Context) error {
	// Step 1: Validate the configuration before starting any work.
	if validationErr := processor.validateConfig(); validationErr != nil {
		return validationErr
	}

	// Step 2: Ensure the `detect-blank` helper binary is built and available.
	if buildErr := ensureDetectBlankBinary(ctx, processor.config.ProjectRoot, processor.log); buildErr != nil {
		return fmt.Errorf("could not prepare blank detection tool: %w", buildErr)
	}

	// Step 3: Discover all PDF files in the input directory.
	pdfPaths, discoveryErr := DiscoverPDFs(processor.config.InputPath)
	if discoveryErr != nil {
		return fmt.Errorf("failed to discover PDFs: %w", discoveryErr)
	}
	if len(pdfPaths) == 0 {
		return fmt.Errorf("no PDF files found in %s", processor.config.InputPath)
	}

	// Step 4: Process each discovered PDF file.
	processor.log.Info("Found %d PDF(s) to process.", len(pdfPaths))

	return processor.processAllPDFs(ctx, pdfPaths)
}

// validateConfig checks if the essential configuration options have been provided.
func (processor *Processor) validateConfig() error {
	if processor.config.InputPath == "" {
		return errors.New("input path is required")
	}
	if processor.config.OutputPath == "" {
		return errors.New("output path is required")
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

		if processErr := processor.processOnePDF(ctx, pdfPath); processErr != nil {
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
		return errors.New("pdf has zero or a negative number of pages")
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
