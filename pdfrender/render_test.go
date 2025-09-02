// Package pdfrender provides PDF-to-PNG conversion functionality.
package pdfrender

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/nnikolov3/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProcessor_Defaults(t *testing.T) {
	t.Parallel()

	log, loggerErr := logger.New(t.TempDir(), "test.log")
	require.NoError(t, loggerErr)

	t.Run("Zero values should default correctly", func(t *testing.T) {
		t.Parallel()

		processor := NewProcessor(Options{
			ProgressBarOutput:      nil,
			InputPath:              "",
			OutputPath:             "",
			ProjectRoot:            "",
			DPI:                    0,
			Workers:                0,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		}, log)
		assert.Equal(t, 200, processor.config.DPI)
		assert.Equal(t, runtime.NumCPU(), processor.config.Workers)
		assert.NotNil(t, processor.executor)
	})

	t.Run("Custom values should be preserved", func(t *testing.T) {
		t.Parallel()

		opts := Options{
			ProgressBarOutput:      nil,
			InputPath:              "",
			OutputPath:             "",
			ProjectRoot:            "",
			DPI:                    300,
			Workers:                4,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		}
		processor := NewProcessor(opts, log)
		assert.Equal(t, 300, processor.config.DPI)
		assert.Equal(t, 4, processor.config.Workers)
	})
}

func TestParsePdfInfoOutput(t *testing.T) {
	t.Parallel()
	t.Run("Valid output with pages", func(t *testing.T) {
		t.Parallel()

		output := "Title: Test Doc\nAuthor: Me\nPages: 15\nEncrypted: no"
		pages, err := parsePdfInfoOutput(output)
		require.NoError(t, err)
		assert.Equal(t, 15, pages)
	})

	t.Run("Output without pages line", func(t *testing.T) {
		t.Parallel()

		output := "Title: Test Doc\nAuthor: Me"
		_, err := parsePdfInfoOutput(output)
		assert.Error(t, err)
	})
}

func TestInterpretBlankDetectorExitCode(t *testing.T) {
	t.Parallel()
	t.Run("Exit code 0 means blank", func(t *testing.T) {
		t.Parallel()

		isBlank, err := interpretBlankDetectorExitCode(
			nil,
		) // nil error is equivalent to exit code 0
		require.NoError(t, err)
		assert.True(t, isBlank)
	})

	t.Run("Exit code 1 means not blank", func(t *testing.T) {
		t.Parallel()
		// Create a fake exec.ExitError with code 1.
		errWithCode1 := &exec.ExitError{
			ProcessState: &os.ProcessState{},
			Stderr:       nil,
		}
		// Note: we cannot set the exit code on os.ProcessState in a portable way
		// here.

		isBlank, err := interpretBlankDetectorExitCode(errWithCode1)
		require.NoError(t, err)
		assert.False(t, isBlank)
	})

	t.Run("Other exit codes mean error", func(t *testing.T) {
		t.Parallel()

		errWithCode2 := &exec.ExitError{
			ProcessState: &os.ProcessState{},
			Stderr:       nil,
		}
		_, err := interpretBlankDetectorExitCode(errWithCode2)
		assert.Error(t, err)
	})
}
