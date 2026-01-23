/* DO EVERYTHING WITH LOVE, CARE, HONESTY, TRUTH, TRUST, KINDNESS, RELIABILITY, CONSISTENCY, DISCIPLINE, RESILIENCE, CRAFTSMANSHIP, HUMILITY, ALLIANCE, EXPLICITNESS */

package pdfrender

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/book-expert/logger"
	"github.com/google/uuid"
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

func (pageProcessor *PageProcessor) ProcessPagesFromBytes(parentContext context.Context, pdfData []byte) ([][]byte, error) {
	// 1. Create a temporary directory in /dev/shm for high-performance RAM I/O
	// We use a unique ID to avoid collisions between workers.
	runID := uuid.New().String()
	shmDir := filepath.Join("/dev/shm", "pdf-render-"+runID)
	
	if err := os.MkdirAll(shmDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create shm directory: %w", err)
	}
	defer os.RemoveAll(shmDir)

	// 2. Write PDF data to the RAM disk
	pdfFilePath := filepath.Join(shmDir, "input.pdf")
	if err := os.WriteFile(pdfFilePath, pdfData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write PDF to shm: %w", err)
	}

	// 3. Prepare Ghostscript command
	// -sDEVICE=png16m: 24-bit color PNG (opaque, solves transparency issues)
	// -o: Sets output file and implies -dBATCH -dNOPAUSE
	outputPattern := filepath.Join(shmDir, "page-%d.png")
	
	args := []string{
		"-sDEVICE=png16m",
		"-r" + strconv.Itoa(pageProcessor.dotsPerInch),
		"-o", outputPattern,
		pdfFilePath,
	}

	cmd := exec.CommandContext(parentContext, "gs", args...)
	
	// Execute and capture output for diagnostics
	output, err := cmd.CombinedOutput()
	if err != nil {
		pageProcessor.serviceLogger.Errorf("Ghostscript failure: %v\nOutput: %s", err, string(output))
		return nil, fmt.Errorf("ghostscript execution failed: %w", err)
	}

	// 4. Read back the generated PNG files from RAM
	files, err := os.ReadDir(shmDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read shm directory: %w", err)
	}

	type pngFile struct {
		index   int
		content []byte
	}
	var pngFiles []pngFile

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".png") {
			continue
		}

		// Extract index from "page-N.png"
		fileName := file.Name()
		indexString := strings.TrimSuffix(strings.TrimPrefix(fileName, "page-"), ".png")
		pageIndex, err := strconv.Atoi(indexString)
		if err != nil {
			continue
		}

		content, err := os.ReadFile(filepath.Join(shmDir, fileName))
		if err != nil {
			return nil, fmt.Errorf("failed to read generated PNG %s: %w", fileName, err)
		}

		pngFiles = append(pngFiles, pngFile{
			index:   pageIndex, // GS is 1-based usually, but file name matches %d
			content: content,
		})
	}

	// 5. Sort to ensure page order
	sort.Slice(pngFiles, func(i, j int) bool {
		return pngFiles[i].index < pngFiles[j].index
	})

	// 6. Extract raw bytes
	var finalImages [][]byte
	for _, file := range pngFiles {
		finalImages = append(finalImages, file.content)
	}

	if len(finalImages) == 0 {
		return nil, fmt.Errorf("ghostscript produced no output images")
	}

	return finalImages, nil
}
