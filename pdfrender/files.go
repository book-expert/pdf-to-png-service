// File: ./pdfrender/files.go
package pdfrender

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverPDFs finds all PDF files in a given directory.
// It performs a case-insensitive search and does not recurse into subdirectories.
func DiscoverPDFs(dirPath string) ([]string, error) {
	dirEntries, readErr := os.ReadDir(dirPath)
	if readErr != nil {
		return nil, fmt.Errorf(
			"could not read directory %s: %w",
			dirPath,
			readErr,
		)
	}

	var pdfPaths []string
	for _, entry := range dirEntries {
		// Ensure we only process files, not directories.
		if !entry.IsDir() &&
			strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") {

			pdfPaths = append(pdfPaths, filepath.Join(dirPath, entry.Name()))
		}
	}

	return pdfPaths, nil
}

// CountFiles counts all files with a given extension in a directory.
// It performs a case-insensitive search and does not recurse into subdirectories.
func CountFiles(dirPath, extension string) (int, error) {
	dirEntries, readErr := os.ReadDir(dirPath)
	if readErr != nil {
		return 0, fmt.Errorf("could not read directory %s: %w", dirPath, readErr)
	}

	count := 0
	lowerExt := strings.ToLower(extension)
	for _, entry := range dirEntries {
		if !entry.IsDir() &&
			strings.HasSuffix(strings.ToLower(entry.Name()), lowerExt) {

			count++
		}
	}

	return count, nil
}

// setupOutputDirectory creates a structured output folder for a given PDF's pages.
// For a PDF named 'mydoc.pdf', it creates '<baseOutputPath>/mydoc/png/'.
func setupOutputDirectory(baseOutputPath, pdfPath string) (string, error) {
	// Extract the PDF filename without the extension.
	pdfBaseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	outputDir := filepath.Join(baseOutputPath, pdfBaseName, "png")

	// Create all necessary parent directories.
	if mkdirErr := os.MkdirAll(outputDir, 0o750); mkdirErr != nil {
		return "", fmt.Errorf(
			"failed to create output directory %s: %w",
			outputDir,
			mkdirErr,
		)
	}

	return outputDir, nil
}
