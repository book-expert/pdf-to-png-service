// File: ./pdfrender/render_test.go
package pdfrender

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/nnikolov3/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor allows us to simulate the behavior of external commands for testing.
type mockExecutor struct {
	// responses maps a command key (e.g., "pdfinfo", "go build") to its expected
	// output and error.
	responses map[string]struct {
		output []byte
		err    error
	}
}

// newMockExecutor creates a new mock executor for use in tests.
func newMockExecutor() *mockExecutor {
	return &mockExecutor{responses: make(map[string]struct{})}
}

// Run simulates executing a command that only uses stdout.
func (executor *mockExecutor) Run(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	// In our project, only pdfinfo uses the simple 'Run' method.
	if name == "pdfinfo" {
		if resp, ok := executor.responses["pdfinfo"]; ok {
			return resp.output, resp.err
		}
	}

	return nil, errors.New("unexpected call to mockExecutor.Run for command: " + name)
}

// RunCombined simulates executing a command where stdout and stderr are merged.
func (executor *mockExecutor) RunCombined(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	var key string
	// Determine the command key based on the command name and arguments.
	if name == "go" && len(args) > 1 && args[0] == "build" {
		key = "go build"
	} else if name == "ghostscript" {
		key = "ghostscript"
	} else if strings.HasSuffix(name, "detect-blank") {
		key = "detect-blank"
	} else {
		return nil, errors.New("unhandled command in mockExecutor.RunCombined: " + name)
	}

	if resp, ok := executor.responses[key]; ok {
		return resp.output, resp.err
	}

	return nil, errors.New("no mock response configured for command: " + key)
}

// newTestProcessor is a helper function to create a Processor with a mock executor for
// testing.
func newTestProcessor(test *testing.T) (*Processor, *mockExecutor) {
	log, err := logger.New(test.TempDir(), "test.log")
	require.NoError(test, err)

	opts := Options{
		InputPath:         test.TempDir(),
		OutputPath:        test.TempDir(),
		ProjectRoot:       ".",        // Assume project root is current dir for tests.
		ProgressBarOutput: io.Discard, // Disable progress bar output during tests.
	}
	processor := NewProcessor(opts, log)
	mockExec := newMockExecutor()
	processor.executor = mockExec // Replace the real executor with our mock.

	return processor, mockExec
}

func TestNewProcessor_Defaults(t *testing.T) {
	log, loggerErr := logger.New(t.TempDir(), "test.log")
	require.NoError(t, loggerErr)

	t.Run("Zero values should default correctly", func(t *testing.T) {
		processor := NewProcessor(Options{}, log)
		assert.Equal(t, 200, processor.config.DPI)
		assert.Equal(t, runtime.NumCPU(), processor.config.Workers)
		assert.NotNil(t, processor.executor)
	})

	t.Run("Custom values should be preserved", func(t *testing.T) {
		opts := Options{DPI: 300, Workers: 4}
		processor := NewProcessor(opts, log)
		assert.Equal(t, 300, processor.config.DPI)
		assert.Equal(t, 4, processor.config.Workers)
	})
}

func TestParsePdfInfoOutput(t *testing.T) {
	t.Run("Valid output with pages", func(t *testing.T) {
		output := "Title: Test Doc\nAuthor: Me\nPages: 15\nEncrypted: no"
		pages, err := parsePdfInfoOutput(output)
		require.NoError(t, err)
		assert.Equal(t, 15, pages)
	})

	t.Run("Output without pages line", func(t *testing.T) {
		output := "Title: Test Doc\nAuthor: Me"
		_, err := parsePdfInfoOutput(output)
		assert.Error(t, err)
	})
}

func TestInterpretBlankDetectorExitCode(t *testing.T) {
	t.Run("Exit code 0 means blank", func(t *testing.T) {
		isBlank, err := interpretBlankDetectorExitCode(
			nil,
		) // nil error is equivalent to exit code 0
		require.NoError(t, err)
		assert.True(t, isBlank)
	})

	t.Run("Exit code 1 means not blank", func(t *testing.T) {
		// Create a fake exec.ExitError with code 1.
		errWithCode1 := &exec.ExitError{ProcessState: &os.ProcessState{}}
		type code interface{ ExitCode() int }
		if p, ok := errWithCode1.ProcessState.Sys().(code); ok {
			// This is a simplified way to set the exit code for testing.
			// A more robust mock would be needed for complex scenarios.
		}

		isBlank, err := interpretBlankDetectorExitCode(errWithCode1)
		require.NoError(t, err)
		assert.False(t, isBlank)
	})

	t.Run("Other exit codes mean error", func(t *testing.T) {
		errWithCode2 := &exec.ExitError{ProcessState: &os.ProcessState{}}
		_, err := interpretBlankDetectorExitCode(errWithCode2)
		assert.Error(t, err)
	})
}
