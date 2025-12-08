package pdfrender

import (
	"context"
	"fmt"
	"sync"
)

type pageJobFromBytes struct {
	pdfData   []byte
	pageIndex int
}

// pageProcessor manages the concurrent rendering of pages.
type pageProcessor struct {
	parent    *Processor
	outputDir string
}

// newPageProcessor creates a new processor.
func newPageProcessor(parent *Processor, outputDir string) *pageProcessor {
	return &pageProcessor{
		parent:    parent,
		outputDir: outputDir,
	}
}

// processPagesFromBytes orchestrates the rendering of all pages in a PDF from a byte slice.
func (pp *pageProcessor) processPagesFromBytes(
	ctx context.Context,
	pdfData []byte,
	pageCount int,
) ([][]byte, error) {
	jobs := make(chan pageJobFromBytes, pageCount)
	results := make(chan struct {
		index int
		data  []byte
	}, pageCount)

	var waitGroup sync.WaitGroup

	// Start a pool of worker goroutines.
	for range pp.parent.config.Workers {
		waitGroup.Add(1)

		go pp.pageWorkerFromBytes(ctx, &waitGroup, jobs, results)
	}

	// Send a job to the workers for each page.
	for i := 1; i <= pageCount; i++ {
		jobs <- pageJobFromBytes{
			pdfData:   pdfData,
			pageIndex: i,
		}
	}

	close(jobs) // No more jobs will be sent.

	waitGroup.Wait() // Wait for all workers to finish.
	close(results)

	// Collect and sort results
	// We need to maintain order, so we'll use a map temporarily or pre-allocate slice
	// Since we can have blanks removed, the output slice might be smaller than pageCount.
	// However, the caller expects a list of PNGs. If we skip blanks, the indices change.

	// Let's collect all valid PNGs.
	// To preserve order relative to the PDF, we should collect them all and then sort by page index.

	collectedPages := make([]struct {
		index int
		data  []byte
	}, 0, pageCount)

	for res := range results {
		collectedPages = append(collectedPages, res)
	}

	// Sort by page index
	// Simple bubble sort is fine for page counts < 1000
	for i := 0; i < len(collectedPages); i++ {
		for j := 0; j < len(collectedPages)-1-i; j++ {
			if collectedPages[j].index > collectedPages[j+1].index {
				collectedPages[j], collectedPages[j+1] = collectedPages[j+1], collectedPages[j]
			}
		}
	}

	finalPNGs := make([][]byte, 0, len(collectedPages))
	for _, p := range collectedPages {
		finalPNGs = append(finalPNGs, p.data)
	}

	return finalPNGs, nil
}

// pageWorkerFromBytes is a goroutine that pulls jobs from the channel and processes them.
func (pp *pageProcessor) pageWorkerFromBytes(
	ctx context.Context,
	waitGroup *sync.WaitGroup,
	jobs <-chan pageJobFromBytes,
	results chan<- struct {
		index int
		data  []byte
	},
) {
	defer waitGroup.Done()

	for job := range jobs {
		if ctx.Err() != nil {
			return
		}

		pngData, processErr := pp.processSinglePageFromBytes(ctx, job)
		if processErr != nil {
			pp.parent.log.Warnf(
				fmt.Sprintf("Failed to process page %d: %v", job.pageIndex, processErr),
			)
		} else if pngData != nil {
			results <- struct {
				index int
				data  []byte
			}{job.pageIndex, pngData}
		}
	}
}

// processSinglePageFromBytes contains the logic for rendering a single page from a byte slice.
func (pp *pageProcessor) processSinglePageFromBytes(ctx context.Context, job pageJobFromBytes) ([]byte, error) {
	// Step 1: Render the PDF page to a PNG image using Ghostscript.
	pngData, renderErr := pp.parent.renderPageFromBytes(ctx, job.pdfData, job.pageIndex)
	if renderErr != nil {
		return nil, fmt.Errorf("rendering failed: %w", renderErr)
	}

	// Step 2: Check for blankness
	isBlank, blankErr := pp.parent.IsImageBlank(pngData)
	if blankErr != nil {
		pp.parent.log.Warnf("Blank detection failed for page %d: %v", job.pageIndex, blankErr)
		// Return the image anyway if detection fails
		return pngData, nil
	}

	if isBlank {
		pp.parent.log.Infof("Page %d detected as blank, skipping.", job.pageIndex)
		return nil, nil // Return nil to signal this page should be skipped
	}

	return pngData, nil
}
