/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package pdfrender

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"sort"
	"sync"

	"github.com/book-expert/logger"
	"github.com/gen2brain/go-fitz"
)

type PageProcessor struct {
	serviceLogger *logger.Logger
	dotsPerInch   int
}

func NewPageProcessor(serviceLogger *logger.Logger, dotsPerInch int) *PageProcessor {
	return &PageProcessor{
		serviceLogger: serviceLogger,
		dotsPerInch:   dotsPerInch,
	}
}

type renderResult struct {
	index   int
	content []byte
	error   error
}

func (pageProcessor *PageProcessor) ProcessPagesFromBytes(parentContext context.Context, pdfData []byte, pageCount int) ([][]byte, error) {
	results := make(chan renderResult, pageCount)
	var waitGroup sync.WaitGroup

	// Concurrency control: Limit to 4 parallel renders
	semaphore := make(chan struct{}, 4)

	for index := 0; index < pageCount; index++ {
		waitGroup.Add(1)
		go func(pageIndex int) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			select {
			case <-parentContext.Done():
				results <- renderResult{error: parentContext.Err()}
				return
			default:
				localDocument, fitzError := fitz.NewFromMemory(pdfData)
				if fitzError != nil {
					results <- renderResult{error: fitzError}
					return
				}
				defer func() {
					if closeError := localDocument.Close(); closeError != nil {
						pageProcessor.serviceLogger.Warnf("failed to close local fitz document: %v", closeError)
					}
				}()

				image, renderError := localDocument.ImageDPI(pageIndex, float64(pageProcessor.dotsPerInch))
				if renderError != nil {
					results <- renderResult{error: renderError}
					return
				}

				var buffer bytes.Buffer
				encoder := png.Encoder{CompressionLevel: png.BestCompression}
				if encodeError := encoder.Encode(&buffer, image); encodeError != nil {
					results <- renderResult{error: fmt.Errorf("png encode failed: %w", encodeError)}
					return
				}

				results <- renderResult{index: pageIndex, content: buffer.Bytes()}
			}
		}(index)
	}

	go func() {
		waitGroup.Wait()
		close(results)
	}()

	finalPages := make([]renderResult, 0, pageCount)
	for result := range results {
		if result.error != nil {
			return nil, result.error
		}
		finalPages = append(finalPages, result)
	}

	// Sort: Restore page order
	sort.Slice(finalPages, func(index, nextIndex int) bool {
		return finalPages[index].index < finalPages[nextIndex].index
	})

	output := make([][]byte, len(finalPages))
	for index, page := range finalPages {
		output[index] = page.content
	}

	return output, nil
}
