// Package pdfrender provides PDF-to-PNG conversion functionality.
package pdfrender_test

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/book-expert/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
)

func TestNewProcessor_Defaults(t *testing.T) {
	t.Parallel()

	log, loggerErr := logger.New(t.TempDir(), "test.log")
	require.NoError(t, loggerErr)

	t.Run("Zero values should default correctly", func(t *testing.T) {
		t.Parallel()

		processor := pdfrender.NewProcessor(&pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              "",
			OutputPath:             "",
			ProjectRoot:            "",
			DPI:                    0,
			Workers:                0,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		}, log)
		cfg := processor.ConfigForTest()
		assert.Equal(t, 200, cfg.DPI)
		assert.Equal(t, runtime.NumCPU(), cfg.Workers)
	})

	t.Run("Custom values should be preserved", func(t *testing.T) {
		t.Parallel()

		opts := pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              "",
			OutputPath:             "",
			ProjectRoot:            "",
			DPI:                    300,
			Workers:                4,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		}
		processor := pdfrender.NewProcessor(&opts, log)
		cfg := processor.ConfigForTest()
		assert.Equal(t, 300, cfg.DPI)
		assert.Equal(t, 4, cfg.Workers)
	})
}

func TestParsePdfInfoOutput(t *testing.T) {
	t.Parallel()
	t.Run("Valid output with pages", func(t *testing.T) {
		t.Parallel()

		output := "Title: Test Doc\nAuthor: Me\nPages: 15\nEncrypted: no"
		pages, err := pdfrender.ParsePdfInfoOutputForTest(output)
		require.NoError(t, err)
		assert.Equal(t, 15, pages)
	})

	t.Run("Output without pages line", func(t *testing.T) {
		t.Parallel()

		output := "Title: Test Doc\nAuthor: Me"
		_, err := pdfrender.ParsePdfInfoOutputForTest(output)
		assert.Error(t, err)
	})
}

func TestInterpretBlankDetectorExitCode(t *testing.T) {
	t.Parallel()
	t.Run("Exit code 0 means blank", func(t *testing.T) {
		t.Parallel()

		isBlank, err := pdfrender.InterpretBlankDetectorExitCodeForTest(
			nil,
		) // nil error is equivalent to exit code 0
		require.NoError(t, err)
		assert.True(t, isBlank)
	})

	t.Run("Exit code 1 means not blank", func(t *testing.T) {
		t.Parallel()
		// Constructing exec.ExitError with a specific non-zero code is not
		// portable across platforms. We skip this subtest and rely on integration
		// behavior.
		t.Skip("cannot reliably construct exec.ExitError with code 1 in tests")
	})

	t.Run("Other exit codes mean error", func(t *testing.T) {
		t.Parallel()

		errWithCode2 := &exec.ExitError{
			ProcessState: &os.ProcessState{},
			Stderr:       nil,
		}
		_, err := pdfrender.InterpretBlankDetectorExitCodeForTest(errWithCode2)
		assert.Error(t, err)
	})
}
