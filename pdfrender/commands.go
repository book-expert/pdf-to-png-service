// File: ./pdfrender/commands.go
package pdfrender

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nnikolov3/logger"
)

// CommandExecutor defines an interface for running external commands.
// This abstraction is crucial for enabling unit tests to mock command execution.
type CommandExecutor interface {
	// Run executes a command and returns its standard output.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	// RunCombined executes a command and returns its combined standard output and
	// standard error.
	RunCombined(ctx context.Context, name string, args ...string) ([]byte, error)
}

// defaultExecutor implements the CommandExecutor interface using the standard os/exec
// package.
// This is the implementation used in the production application.
type defaultExecutor struct{}

// Run is the production implementation for executing a command.
func (executor *defaultExecutor) Run(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// RunCombined is the production implementation for executing a command and capturing all
// output.
func (executor *defaultExecutor) RunCombined(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// getPDFPages executes the `pdfinfo` command to determine the number of pages in a PDF.
func (processor *Processor) getPDFPages(
	ctx context.Context,
	pdfPath string,
) (int, error) {
	if pdfPath == "" {
		return 0, errors.New("pdf path cannot be empty")
	}

	// The `pdfinfo` command prints metadata, including the page count, to stdout.
	outputBytes, execErr := processor.executor.Run(ctx, "pdfinfo", pdfPath)
	if execErr != nil {
		// Include the command's output in the error for better debugging.
		return 0, fmt.Errorf(
			"pdfinfo execution failed: %w. Output: %s",
			execErr,
			string(outputBytes),
		)
	}

	return parsePdfInfoOutput(string(outputBytes))
}

// parsePdfInfoOutput scans the text output from the `pdfinfo` command to find and parse
// the page count.
func parsePdfInfoOutput(output string) (int, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Pages:") {
			parts := strings.Fields(line) // e.g., ["Pages:", "123"]
			if len(parts) >= 2 {
				pageCount, convErr := strconv.Atoi(parts[1])
				if convErr == nil {
					return pageCount, nil
				}
			}
		}
	}

	return 0, errors.New("could not parse 'Pages:' line from pdfinfo output")
}

// renderPage executes the Ghostscript command to convert a single PDF page to a PNG
// image.
func (processor *Processor) renderPage(
	ctx context.Context,
	pdfPath string,
	page int,
	outPath string,
) error {
	if page <= 0 {
		return errors.New("page number must be positive")
	}

	if pdfPath == "" || outPath == "" {
		return errors.New("pdf path and output path cannot be empty")
	}

	args := buildGhostscriptArgs(processor.config.DPI, page, outPath, pdfPath)

	outputBytes, execErr := processor.executor.RunCombined(
		ctx,
		"ghostscript",
		args...)
	if execErr != nil {
		return fmt.Errorf(
			"ghostscript execution failed: %w. Output: %s",
			execErr,
			string(outputBytes),
		)
	}

	return nil
}

// buildGhostscriptArgs constructs the list of command-line arguments for the Ghostscript
// process.
func buildGhostscriptArgs(dpi, page int, outPath, pdfPath string) []string {
	return []string{
		"-q", "-dNOPAUSE", "-dBATCH", // Quiet mode, non-interactive batch processing.
		"-sDEVICE=png16m",                   // Set the output device to a 24-bit color PNG.
		fmt.Sprintf("-r%d", dpi),            // Set the resolution in DPI.
		fmt.Sprintf("-dFirstPage=%d", page), // Specify the page number to render.
		fmt.Sprintf("-dLastPage=%d", page),  // Process only that single page.
		"-o", outPath,                       // Set the output file path.
		"-dTextAlphaBits=4",     // Enable anti-aliasing for text.
		"-dGraphicsAlphaBits=4", // Enable anti-aliasing for graphics.
		"-dDownScaleFactor=1",   // Disable downscaling.
		"-dPDFFitPage",          // Fit the PDF page to the output media.
		pdfPath,                 // The input PDF file.
	}
}

// handleBlankDetection checks a newly rendered PNG and removes it if it's determined to
// be blank.
func (processor *Processor) handleBlankDetection(
	ctx context.Context,
	pngPath string,
) error {
	isBlank, detectionErr := processor.isPngBlank(ctx, pngPath)
	if detectionErr != nil {
		return detectionErr
	}

	if isBlank {
		removeErr := os.Remove(pngPath)
		if removeErr != nil {
			return fmt.Errorf(
				"failed to remove blank file %s: %w",
				pngPath,
				removeErr,
			)
		}

		processor.log.Info("Removed blank: %s", filepath.Base(pngPath))
	}

	return nil
}

// isPngBlank executes the `detect-blank` helper binary to analyze a PNG image.
func (processor *Processor) isPngBlank(
	ctx context.Context,
	pngPath string,
) (bool, error) {
	binaryPath := filepath.Join(processor.config.ProjectRoot, "bin", "detect-blank")
	args := []string{
		pngPath,
		strconv.Itoa(processor.config.BlankFuzzPercent),
		fmt.Sprintf("%.6f", processor.config.BlankNonWhiteThreshold),
	}

	_, execErr := processor.executor.RunCombined(ctx, binaryPath, args...)

	return interpretBlankDetectorExitCode(execErr)
}

// interpretBlankDetectorExitCode translates the exit code from the blank detector binary
// into a boolean result (isBlank) or an error.
func interpretBlankDetectorExitCode(execErr error) (bool, error) {
	if execErr == nil {
		// Exit code 0 means the image is blank.
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(execErr, &exitErr) {
		switch exitErr.ExitCode() {
		case 1:
			// Exit code 1 means the image is not blank.
			return false, nil
		default:
			// Any other exit code indicates an unexpected error in the
			// detector tool.
			return false, fmt.Errorf(
				"blank detector exited with unexpected code %d",
				exitErr.ExitCode(),
			)
		}
	}

	// This handles cases where the command couldn't even be executed (e.g.,
	// permission denied).
	return false, fmt.Errorf("failed to run blank detector command: %w", execErr)
}

// ensureDetectBlankBinary checks for the existence of the `detect-blank` binary
// and compiles it from source if it is not found.
func ensureDetectBlankBinary(
	ctx context.Context,
	projectRoot string,
	log *logger.Logger,
) error {
	if projectRoot == "" {
		return errors.New("projectRoot must be set to auto-build helper binaries")
	}

	binaryPath := filepath.Join(projectRoot, "bin", "detect-blank")
	sourcePath := filepath.Join(projectRoot, "cmd", "detect-blank", "main.go")

	// If the binary already exists, we don't need to do anything.
	if _, statErr := os.Stat(binaryPath); statErr == nil {
		return nil
	}

	// If the source code is missing, we can't build it.
	if _, statErr := os.Stat(sourcePath); os.IsNotExist(statErr) {
		return fmt.Errorf(
			"cannot build detect-blank: source file not found at %s",
			sourcePath,
		)
	}

	// Proceed with building the binary.
	return buildBlankDetector(ctx, sourcePath, binaryPath, log)
}

// buildBlankDetector runs `go build` to compile the helper binary from its source file.
func buildBlankDetector(
	ctx context.Context,
	sourcePath, binaryPath string,
	log *logger.Logger,
) error {
	log.Info("Detect-blank binary not found. Building from source...")

	binDir := filepath.Dir(binaryPath)

	mkdirErr := os.MkdirAll(binDir, 0o755)
	if mkdirErr != nil {
		return fmt.Errorf(
			"could not create bin directory at %s: %w",
			binDir,
			mkdirErr,
		)
	}

	// The `-o` flag specifies the output path for the compiled binary.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, sourcePath)

	output, buildErr := cmd.CombinedOutput()
	if buildErr != nil {
		return fmt.Errorf(
			"failed to build detect-blank binary: %w. Output: %s",
			buildErr,
			string(output),
		)
	}

	log.Success("Successfully built detect-blank binary at %s", binaryPath)

	return nil
}
