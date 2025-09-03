package pdfrender

import "context"

// Exported test-only accessors for unexported functions and fields.
// This file is compiled only during tests and does not affect the public API.

// ParsePdfInfoOutputForTest exposes parsePdfInfoOutput for tests in external package.
func ParsePdfInfoOutputForTest(s string) (int, error) { return parsePdfInfoOutput(s) }

// InterpretBlankDetectorExitCodeForTest exposes interpretBlankDetectorExitCode for tests
// in external package.
func InterpretBlankDetectorExitCodeForTest(err error) (bool, error) {
	return interpretBlankDetectorExitCode(err)
}

// ConfigForTest returns a copy of the processor configuration for assertions in tests.
func (processor *Processor) ConfigForTest() Options { return processor.config }

// Test-only helpers to access unexported methods for white-box tests from external
// package.
func (processor *Processor) ValidateConfigForTest() error { return processor.validateConfig() }

func (processor *Processor) DiscoverInputPDFsForTest() ([]string, error) {
	return processor.discoverInputPDFs()
}

func (processor *Processor) PrepareToolsForTest(ctx context.Context) error {
	return processor.prepareTools(ctx)
}

// Allow tests to inject a fake executor.
func (processor *Processor) SetExecutorForTest(
	exec CommandExecutor,
) {
	processor.executor = exec
}

// Call internal processing functions for focused tests.
func (processor *Processor) ProcessAllPDFsForTest(
	ctx context.Context,
	paths []string,
) error {
	return processor.processAllPDFs(ctx, paths)
}

func (processor *Processor) ProcessOnePDFForTest(ctx context.Context, path string) error {
	return processor.processOnePDF(ctx, path)
}
