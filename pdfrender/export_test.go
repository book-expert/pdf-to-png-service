package pdfrender

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
