package pdfrender_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/book-expert/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/book-expert/pdf-to-png-service/internal/pdfrender"
)

type fakeExec struct {
	err error
	run map[string]struct {
		err error
		out []byte
	}
	runCombined map[string]struct {
		err error
		out []byte
	}
	onRunCombined func(name string, args []string)
	stdout        []byte
	combinedOut   []byte
}

func (f *fakeExec) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	if f.run != nil {
		if v, ok := f.run[key]; ok {
			return v.out, v.err
		}
	}

	return f.stdout, f.err
}

func (f *fakeExec) RunCombined(
	_ context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	if f.onRunCombined != nil {
		f.onRunCombined(name, args)
	}

	key := name + " " + strings.Join(args, " ")
	if f.runCombined != nil {
		if v, ok := f.runCombined[key]; ok {
			return v.out, v.err
		}
	}

	return f.combinedOut, f.err
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	log, err := logger.New(t.TempDir(), "test.log")
	require.NoError(t, err)

	proc := pdfrender.NewProcessor(&pdfrender.Options{
		ProgressBarOutput:      nil,
		InputPath:              "",
		OutputPath:             "",
		ProjectRoot:            "",
		DPI:                    0,
		Workers:                0,
		BlankFuzzPercent:       0,
		BlankNonWhiteThreshold: 0,
	}, log)
	require.ErrorIs(t, proc.ValidateConfigForTest(), pdfrender.ErrInputPathRequired)

	proc = pdfrender.NewProcessor(&pdfrender.Options{
		ProgressBarOutput:      nil,
		InputPath:              "in",
		OutputPath:             "",
		ProjectRoot:            "",
		DPI:                    0,
		Workers:                0,
		BlankFuzzPercent:       0,
		BlankNonWhiteThreshold: 0,
	}, log)
	require.ErrorIs(t, proc.ValidateConfigForTest(), pdfrender.ErrOutputPathRequired)

	proc = pdfrender.NewProcessor(
		&pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              "in",
			OutputPath:             "out",
			ProjectRoot:            "",
			DPI:                    0,
			Workers:                0,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
		log,
	)
	require.NoError(t, proc.ValidateConfigForTest())
}

func TestDiscoverPDFsAndEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// create files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.pdf"), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.PDF"), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o600))

	files, err := pdfrender.DiscoverPDFs(dir)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	log, err := logger.New(t.TempDir(), "t.log")
	require.NoError(t, err)

	proc := pdfrender.NewProcessor(
		&pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              dir,
			OutputPath:             t.TempDir(),
			ProjectRoot:            "",
			DPI:                    0,
			Workers:                0,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
		log,
	)
	paths, err := proc.DiscoverInputPDFsForTest()
	require.NoError(t, err)
	assert.Len(t, paths, 2)

	emptyDir := t.TempDir()

	proc = pdfrender.NewProcessor(
		&pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              emptyDir,
			OutputPath:             t.TempDir(),
			ProjectRoot:            "",
			DPI:                    0,
			Workers:                0,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
		log,
	)
	_, err = proc.DiscoverInputPDFsForTest()
	require.Error(t, err)
}

func TestPrepareTools_BuildAndErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// simulate missing binary but present source
	cmdDir := filepath.Join(root, "cmd", "detect-blank")
	require.NoError(t, os.MkdirAll(cmdDir, 0o750))
	// minimal go file
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(cmdDir, "main.go"),
			[]byte("package main\nfunc main(){}"),
			0o600,
		),
	)

	log, err := logger.New(t.TempDir(), "t.log")
	require.NoError(t, err)

	proc := pdfrender.NewProcessor(
		&pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              t.TempDir(),
			OutputPath:             t.TempDir(),
			ProjectRoot:            root,
			DPI:                    0,
			Workers:                0,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
		log,
	)
	// prepareTools should try to build; but buildBlankDetector uses go toolchain.
	// To avoid running real build, place a dummy binary to satisfy stat check.
	binDir := filepath.Join(root, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o750))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(binDir, "detect-blank"), []byte(""), 0o600),
	)
	require.NoError(t, proc.PrepareToolsForTest(context.Background()))

	proc2 := pdfrender.NewProcessor(
		&pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              t.TempDir(),
			OutputPath:             t.TempDir(),
			ProjectRoot:            "",
			DPI:                    0,
			Workers:                0,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
		log,
	)
	require.ErrorIs(
		t,
		proc2.PrepareToolsForTest(context.Background()),
		pdfrender.ErrProjectRootMustBeSet,
	)
}

func TestProcessOnePDF_PageCountZeroError(t *testing.T) {
	t.Parallel()

	inDir := t.TempDir()
	outDir := t.TempDir()
	pdfPath := filepath.Join(inDir, "doc.pdf")
	require.NoError(t, os.WriteFile(pdfPath, []byte("%PDF-1.4"), 0o600))

	log, err := logger.New(t.TempDir(), "t.log")
	require.NoError(t, err)

	proc := pdfrender.NewProcessor(
		&pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              inDir,
			OutputPath:             outDir,
			ProjectRoot:            t.TempDir(),
			DPI:                    0,
			Workers:                1,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
		log,
	)
	fakeExecutor := &fakeExec{
		err: nil,
		run: map[string]struct {
			err error
			out []byte
		}{
			"pdfinfo " + pdfPath: {out: []byte("Pages: 0\n"), err: nil},
		},
		runCombined:   nil,
		onRunCombined: nil,
		stdout:        nil,
		combinedOut:   nil,
	}
	proc.SetExecutorForTest(fakeExecutor)
	require.ErrorIs(
		t,
		proc.ProcessOnePDFForTest(context.Background(), pdfPath),
		pdfrender.ErrPDFZeroOrNegativePages,
	)
}

func TestProcess_ValidatesAndPrepares(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	log, err := logger.New(t.TempDir(), "t.log")
	require.NoError(t, err)
	// Missing input/output -> validation error
	proc := pdfrender.NewProcessor(&pdfrender.Options{
		ProgressBarOutput:      nil,
		InputPath:              "",
		OutputPath:             "",
		ProjectRoot:            "",
		DPI:                    0,
		Workers:                0,
		BlankFuzzPercent:       0,
		BlankNonWhiteThreshold: 0,
	}, log)
	require.ErrorIs(t, proc.Process(ctx), pdfrender.ErrInputPathRequired)
	// Input/output but missing project root -> prepareTools error
	proc = pdfrender.NewProcessor(
		&pdfrender.Options{
			ProgressBarOutput:      nil,
			InputPath:              t.TempDir(),
			OutputPath:             t.TempDir(),
			ProjectRoot:            "",
			DPI:                    0,
			Workers:                0,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
		log,
	)
	require.ErrorIs(t, proc.Process(ctx), pdfrender.ErrProjectRootMustBeSet)
}

// findOutputPath finds the output path from ghostscript arguments.
func findOutputPath(args []string) string {
	for i := range len(args) - 1 {
		if args[i] == "-o" {
			return args[i+1]
		}
	}

	return ""
}

// createGhostscriptOutputFile simulates ghostscript creating an output PNG file
// by finding the output path in the args and writing a test file there.
func createGhostscriptOutputFile(args []string) error {
	outputPath := findOutputPath(args)
	if outputPath == "" {
		return nil
	}

	err := os.WriteFile(outputPath, []byte("png"), 0o600)
	if err != nil {
		return fmt.Errorf("failed to create mock output file: %w", err)
	}

	return nil
}

// setupTestDirectories creates the necessary test directory structure.
func setupTestDirectories(t *testing.T, root string) (inDir, outDir, pdfPath string) {
	t.Helper()

	inDir = filepath.Join(root, "in")
	outDir = filepath.Join(root, "out")

	require.NoError(t, os.MkdirAll(inDir, 0o750))
	require.NoError(t, os.MkdirAll(outDir, 0o750))

	pdfPath = filepath.Join(inDir, "file.pdf")
	require.NoError(t, os.WriteFile(pdfPath, []byte("%PDF-1.4"), 0o600))

	return inDir, outDir, pdfPath
}

// setupDummyBinary creates a dummy detect-blank binary to skip building.
func setupDummyBinary(t *testing.T, root string) {
	t.Helper()

	binDir := filepath.Join(root, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o750))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(binDir, "detect-blank"), []byte(""), 0o600),
	)
}

// createMockExecutor creates a fakeExec with the necessary setup for the test.
func createMockExecutor(pdfPath string) *fakeExec {
	return &fakeExec{
		err: nil,
		run: map[string]struct {
			err error
			out []byte
		}{
			"pdfinfo " + pdfPath: {out: []byte("Pages: 1\n"), err: nil},
		},
		runCombined: nil,
		onRunCombined: func(name string, args []string) {
			if name == "ghostscript" {
				err := createGhostscriptOutputFile(args)
				if err != nil {
					panic(err)
				}
			}
		},
		stdout:      nil,
		combinedOut: nil,
	}
}

func TestProcessAllPDFs_ProgressAndFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()

	// Setup test environment
	inDir, outDir, pdfPath := setupTestDirectories(t, root)
	setupDummyBinary(t, root)

	// Set a buffer for progress bars.
	var buf bytes.Buffer

	log, err := logger.New(t.TempDir(), "t.log")
	require.NoError(t, err)

	proc := pdfrender.NewProcessor(
		&pdfrender.Options{
			ProgressBarOutput:      &buf,
			InputPath:              inDir,
			OutputPath:             outDir,
			ProjectRoot:            root,
			DPI:                    0,
			Workers:                1,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
		log,
	)

	// Setup mock executor that simulates pdfinfo and ghostscript behavior
	mockExecutor := createMockExecutor(pdfPath)
	proc.SetExecutorForTest(mockExecutor)

	// Test processAllPDFs validates progress output and runs without error
	require.NoError(t, proc.ProcessAllPDFsForTest(ctx, []string{pdfPath}))
	assert.NotEqual(t, 0, buf.Len())

	// Test full Process flow: validateConfig + prepareTools + discovery
	require.NoError(t, proc.Process(ctx))
}
