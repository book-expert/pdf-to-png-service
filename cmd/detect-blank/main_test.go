// File: ./cmd/detect-blank/main_test.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Suite for Argument Parsing ---

type argTestCase struct {
	asserter func(t *testing.T, result arguments, err error)
	name     string
	args     []string
}

func TestParseAndValidateArguments(t *testing.T) {
	t.Parallel()

	testCases := getParseAndValidateArgumentsTestCases()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, err := parseAndValidateArguments(testCase.args)
			testCase.asserter(t, result, err)
		})
	}
}

func getParseAndValidateArgumentsTestCases() []argTestCase {
	happyPathCases := []argTestCase{
		{
			name: "Happy Path: Valid arguments",
			args: []string{"./detect-blank", "image.png", "10", "0.1"},
			asserter: func(t *testing.T, result arguments, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Equal(
					t,
					arguments{
						filePath:   "image.png",
						fuzzFactor: 0.1,
						threshold:  0.1,
					},
					result,
				)
			},
		},
	}

	return append(happyPathCases, getArgErrorCases()...)
}

func getArgErrorCases() []argTestCase {
	return []argTestCase{
		{
			name:     "Error: Too few arguments",
			args:     []string{"./detect-blank", "image.png"},
			asserter: func(t *testing.T, _ arguments, err error) { t.Helper(); require.ErrorIs(t, err, ErrInvalidArguments) },
		},
		{
			name:     "Error: Too many arguments",
			args:     []string{"./detect-blank", "a", "b", "c", "d"},
			asserter: func(t *testing.T, _ arguments, err error) { t.Helper(); require.ErrorIs(t, err, ErrInvalidArguments) },
		},
	}
}

// --- Test Suite for Image Content Analysis ---

type imageTestCase struct {
	setup    func(t *testing.T, filePath string)
	asserter func(t *testing.T, hasContent bool, err error)
	name     string
	args     arguments
}

func TestImageHasContent(t *testing.T) {
	t.Parallel()

	testCases := getImageHasContentTestCases()
	for _, testCase := range testCases {
		// Solves syntax error: The function signature is now correct.
		t.Run(testCase.name, func(t *testing.T) {
			t.Helper() // t.Helper() is now correctly placed inside the function body.
			t.Parallel()

			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, "test.png")
			args := testCase.args

			args.filePath = filePath
			testCase.setup(t, filePath)

			hasContent, err := imageHasContent(args)
			testCase.asserter(t, hasContent, err)
		})
	}
}

func getImageHasContentTestCases() []imageTestCase {
	// Define shared assertion helpers
	assertErrorIs := func(expectedErr error) func(t *testing.T, hc bool, err error) {
		return func(t *testing.T, _ bool, err error) {
			t.Helper()
			require.ErrorIs(t, err, expectedErr)
		}
	}
	assertHasContent := func(expected bool) func(t *testing.T, hc bool, err error) {
		return func(t *testing.T, hc bool, err error) {
			t.Helper()
			require.NoError(t, err)
			assert.Equal(t, expected, hc)
		}
	}

	// Define shared test images
	images := map[string]image.Image{
		"white": createTestImage(100, 100, color.White),
		"black": createTestImage(100, 100, color.Black),
		"zero":  createTestImage(0, 0, color.White),
	}

	errorCases := getImageErrorCases(assertErrorIs, images)
	contentCases := getImageContentCases(assertHasContent, images)

	return append(errorCases, contentCases...)
}

func getImageErrorCases(
	assertErrorIs func(error) func(*testing.T, bool, error),
	images map[string]image.Image,
) []imageTestCase {
	return []imageTestCase{
		{
			name:     "File not found",
			args:     arguments{filePath: "", fuzzFactor: 0, threshold: 0},
			setup:    func(_ *testing.T, _ string) {},
			asserter: assertErrorIs(os.ErrNotExist),
		},
		{
			name: "Invalid image file",
			args: arguments{filePath: "", fuzzFactor: 0, threshold: 0},
			setup: func(t *testing.T, filePath string) {
				t.Helper()
				require.NoError(
					t,
					os.WriteFile(
						filePath,
						[]byte("not an image"),
						0o600,
					),
				)
			},
			asserter: assertErrorIs(image.ErrFormat),
		},
		{
			name:     "Image with zero pixels",
			args:     arguments{filePath: "", fuzzFactor: 0, threshold: 0},
			setup:    func(t *testing.T, fp string) { t.Helper(); createTestPNG(t, fp, images["zero"]) },
			asserter: assertErrorIs(ErrImageZeroPixels),
		},
	}
}

func getImageContentCases(
	assertHasContent func(bool) func(*testing.T, bool, error),
	images map[string]image.Image,
) []imageTestCase {
	return []imageTestCase{
		{
			name:     "Completely white image is blank",
			args:     arguments{filePath: "", fuzzFactor: 0, threshold: 0.01},
			setup:    func(t *testing.T, fp string) { t.Helper(); createTestPNG(t, fp, images["white"]) },
			asserter: assertHasContent(false),
		},
		{
			name:     "Completely black image has content",
			args:     arguments{filePath: "", fuzzFactor: 0, threshold: 0.01},
			setup:    func(t *testing.T, fp string) { t.Helper(); createTestPNG(t, fp, images["black"]) },
			asserter: assertHasContent(true),
		},
	}
}

// --- General Test Helper Functions ---

func createTestImage(width, height int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, c)
		}
	}

	return img
}

func createTestPNG(t *testing.T, filePath string, img image.Image) {
	t.Helper()

	file, err := os.Create(filePath)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, file.Close()) })

	err = png.Encode(file, img)
	require.NoError(t, err)
}
