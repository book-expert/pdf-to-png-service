/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */
package pdfrender

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// RenderJob represents a unit of work for the page worker.
// Why: Explicit typing makes channel signatures easier to read and modify.
type RenderJob struct {
	PDFData   []byte
	PageIndex int
}

// RenderResult holds the outcome of a processed page.
// Why: Decouples the result data from the transmission mechanism.
type RenderResult struct {
	PageIndex int
	ImageData []byte
}

// PageProcessor manages the concurrent rendering of pages.
type PageProcessor struct {
	parent    *Processor
	outputDir string
}

// NewPageProcessor creates a new processor.
func NewPageProcessor(parent *Processor, outputDir string) *PageProcessor {
	return &PageProcessor{
		parent:    parent,
		outputDir: outputDir,
	}
}

// ProcessPagesFromBytes orchestrates the rendering of all pages in a PDF from a byte slice.
//
// Graph: Fan-Out (Jobs) -> Workers -> Fan-In (Results) -> Sort -> Filter
func (processor *PageProcessor) ProcessPagesFromBytes(
	parentContext context.Context,
	pdfData []byte,
	pageCount int,
) ([][]byte, error) {
	// Buffered channels prevent blocking on send/receive for small to medium PDFs.
	jobs := make(chan RenderJob, pageCount)
	results := make(chan RenderResult, pageCount)

	var waitGroup sync.WaitGroup
	workerCount := processor.parent.config.Workers

	//

	// 1. Fan-Out: Start worker pool
	for index := 0; index < workerCount; index++ {
		waitGroup.Add(1)
		go processor.pageWorker(parentContext, &waitGroup, jobs, results)
	}

	// 2. Queue Jobs: Send work to workers
	for index := 1; index <= pageCount; index++ {
		jobs <- RenderJob{
			PDFData:   pdfData,
			PageIndex: index,
		}
	}
	close(jobs) // Signal workers that no more jobs are coming

	// 3. Wait: Ensure all processing is complete before closing results
	waitGroup.Wait()
	close(results)

	// 4. Fan-In: Collect results
	// Why: We must collect all concurrent results before we can order them.
	collectedPages := make([]RenderResult, 0, pageCount)
	for result := range results {
		collectedPages = append(collectedPages, result)
	}

	// 5. Sort: Restore page order
	// Why: Concurrency disrupts order; we must restore it based on PageIndex.
	// O(n log n) is significantly faster than Bubble Sort for large docs.
	sort.Slice(collectedPages, func(indexI, indexJ int) bool {
		return collectedPages[indexI].PageIndex < collectedPages[indexJ].PageIndex
	})

	// 6. Extract: Create final slice
	finalPNGs := make([][]byte, 0, len(collectedPages))
	for _, page := range collectedPages {
		finalPNGs = append(finalPNGs, page.ImageData)
	}

	return finalPNGs, nil
}

// pageWorker processes jobs from the channel until closed or context cancelled.
func (processor *PageProcessor) pageWorker(
	parentContext context.Context,
	waitGroup *sync.WaitGroup,
	jobs <-chan RenderJob,
	results chan<- RenderResult,
) {
	defer waitGroup.Done()

	for job := range jobs {
		// Fail fast if context is cancelled
		if parentContext.Err() != nil {
			return
		}

		pngData, renderError := processor.processSinglePage(parentContext, job)
		// Log errors but do not crash the batch; individual page failures are tolerable.
		if renderError != nil {
			processor.parent.log.Warnf("Failed to process page %d: %v", job.PageIndex, renderError)
			continue
		}

		// Only send non-nil data (nil indicates a blank page was skipped)
		if pngData != nil {
			results <- RenderResult{
				PageIndex: job.PageIndex,
				ImageData: pngData,
			}
		}
	}
}

// processSinglePage renders a specific page and checks for blankness.
func (processor *PageProcessor) processSinglePage(parentContext context.Context, job RenderJob) ([]byte, error) {
	// Step 1: Render PDF -> PNG
	pngData, renderError := processor.parent.renderPageFromBytes(parentContext, job.PDFData, job.PageIndex)
	if renderError != nil {
		return nil, fmt.Errorf("render error: %w", renderError)
	}

	// Step 2: Blank Detection
	isBlank, detectionError := processor.parent.IsImageBlank(pngData)
	if detectionError != nil {
		processor.parent.log.Warnf("Blank detection failed for page %d: %v", job.PageIndex, detectionError)
		// Fallback: Return image if detection fails to avoid data loss
		return pngData, nil
	}

	if isBlank {
		processor.parent.log.Infof("Page %d detected as blank, skipping.", job.PageIndex)
		return nil, nil // Signal to skip
	}

	return pngData, nil
}