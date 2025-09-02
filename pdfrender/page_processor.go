// Package pdfrender provides PDF-to-PNG conversion functionality.
package pdfrender

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/cheggaaa/pb/v3"
)

// pageJob represents a single task for a worker to render one page of a PDF.
type pageJob struct {
	pdfPath    string
	outputPath string
	pageIndex  int
}

// pageProcessor manages the concurrent rendering of pages for a single PDF file.
type pageProcessor struct {
	parent    *Processor // A reference back to the main processor for config and logging.
	outputDir string
}

// newPageProcessor creates a new processor for handling the pages of one PDF.
func newPageProcessor(parent *Processor, outputDir string) *pageProcessor {
	return &pageProcessor{
		parent:    parent,
		outputDir: outputDir,
	}
}

// processPages orchestrates the rendering of all pages in a PDF.
// It sets up a worker pool, distributes jobs, and waits for completion.
func (pp *pageProcessor) processPages(
	ctx context.Context,
	pdfPath string,
	pageCount int,
) error {
	jobs := make(chan pageJob, pageCount)

	var waitGroup sync.WaitGroup

	// Start a pool of worker goroutines.
	for range pp.parent.config.Workers {
		waitGroup.Add(1)

		go pp.pageWorker(ctx, &waitGroup, jobs)
	}

	// Create a progress bar specifically for the pages of this PDF.
	pageProgressBar := pb.New(pageCount).
		SetTemplateString(`  {{ bar . " " "▸" "▹" " " " "}} {{percent .}} {{etime .}}`).
		SetWriter(pp.parent.config.ProgressBarOutput).
		Start()
	defer pageProgressBar.Finish()

	// Send a job to the workers for each page.
	for i := 1; i <= pageCount; i++ {
		pngPath := filepath.Join(pp.outputDir, fmt.Sprintf("page_%04d.png", i))
		jobs <- pageJob{
			pdfPath:    pdfPath,
			pageIndex:  i,
			outputPath: pngPath,
		}
	}

	close(jobs) // No more jobs will be sent.

	waitGroup.Wait() // Wait for all workers to finish.

	return nil
}

// pageWorker is a goroutine that pulls jobs from the channel and processes them.
// It runs until the jobs channel is closed and empty.
func (pp *pageProcessor) pageWorker(
	ctx context.Context,
	waitGroup *sync.WaitGroup,
	jobs <-chan pageJob,
) {
	defer waitGroup.Done()

	for job := range jobs {
		// Check if the context has been canceled (e.g., by Ctrl+C).
		if ctx.Err() != nil {
			pp.parent.log.Warn(
				"Context canceled, skipping job for page %d",
				job.pageIndex,
			)

			return
		}

		processErr := pp.processSinglePage(ctx, job)
		if processErr != nil {
			pp.parent.log.Warn(
				"Failed to process page %d of %s: %v",
				job.pageIndex,
				filepath.Base(job.pdfPath),
				processErr,
			)
		}
	}
}

// processSinglePage contains the logic for rendering and checking a single page.
func (pp *pageProcessor) processSinglePage(ctx context.Context, job pageJob) error {
	// Step 1: Render the PDF page to a PNG image using Ghostscript.
	renderErr := pp.parent.renderPage(ctx, job.pdfPath, job.pageIndex, job.outputPath)
	if renderErr != nil {
		return fmt.Errorf("rendering failed: %w", renderErr)
	}

	// Step 2: Check the resulting PNG for blankness and delete it if necessary.
	detectionErr := pp.parent.handleBlankDetection(ctx, job.outputPath)
	if detectionErr != nil {
		// Log this as a warning because the page was still rendered,
		// but we failed to determine if it was blank.
		pp.parent.log.Warn(
			"Blank detection failed for %s: %v",
			filepath.Base(job.outputPath),
			detectionErr,
		)
	}

	return nil
}
