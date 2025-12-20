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
	ctx context.Context,
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
	for i := 0; i < workerCount; i++ {
		waitGroup.Add(1)
		go processor.pageWorker(ctx, &waitGroup, jobs, results)
	}

	// 2. Queue Jobs: Send work to workers
	for i := 1; i <= pageCount; i++ {
		jobs <- RenderJob{
			PDFData:   pdfData,
			PageIndex: i,
		}
	}
	close(jobs) // Signal workers that no more jobs are coming

	// 3. Wait: Ensure all processing is complete before closing results
	waitGroup.Wait()
	close(results)

	// 4. Fan-In: Collect results
	// Why: We must collect all concurrent results before we can order them.
	collectedPages := make([]RenderResult, 0, pageCount)
	for res := range results {
		collectedPages = append(collectedPages, res)
	}

	// 5. Sort: Restore page order
	// Why: Concurrency disrupts order; we must restore it based on PageIndex.
	// O(n log n) is significantly faster than Bubble Sort for large docs.
	sort.Slice(collectedPages, func(i, j int) bool {
		return collectedPages[i].PageIndex < collectedPages[j].PageIndex
	})

	// 6. Extract: Create final slice
	finalPNGs := make([][]byte, 0, len(collectedPages))
	for _, p := range collectedPages {
		finalPNGs = append(finalPNGs, p.ImageData)
	}

	return finalPNGs, nil
}

// pageWorker processes jobs from the channel until closed or context cancelled.
func (processor *PageProcessor) pageWorker(
	ctx context.Context,
	waitGroup *sync.WaitGroup,
	jobs <-chan RenderJob,
	results chan<- RenderResult,
) {
	defer waitGroup.Done()

	for job := range jobs {
		// Fail fast if context is cancelled
		if ctx.Err() != nil {
			return
		}

		pngData, err := processor.processSinglePage(ctx, job)
		// Log errors but do not crash the batch; individual page failures are tolerable.
		if err != nil {
			processor.parent.log.Warnf("Failed to process page %d: %v", job.PageIndex, err)
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
func (processor *PageProcessor) processSinglePage(ctx context.Context, job RenderJob) ([]byte, error) {
	// Step 1: Render PDF -> PNG
	pngData, err := processor.parent.renderPageFromBytes(ctx, job.PDFData, job.PageIndex)
	if err != nil {
		return nil, fmt.Errorf("render error: %w", err)
	}

	// Step 2: Blank Detection
	isBlank, err := processor.parent.IsImageBlank(pngData)
	if err != nil {
		processor.parent.log.Warnf("Blank detection failed for page %d: %v", job.PageIndex, err)
		// Fallback: Return image if detection fails to avoid data loss
		return pngData, nil
	}

	if isBlank {
		processor.parent.log.Infof("Page %d detected as blank, skipping.", job.PageIndex)
		return nil, nil // Signal to skip
	}

	return pngData, nil
}
