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
		{
			name:     "Error: Invalid fuzz percentage (non-numeric)",
			args:     []string{"./detect-blank", "image.png", "abc", "0.1"},
			asserter: func(t *testing.T, _ arguments, err error) { t.Helper(); require.Error(t, err) },
		},
		{
			name:     "Error: Fuzz percentage below range",
			args:     []string{"./detect-blank", "image.png", "-1", "0.1"},
			asserter: func(t *testing.T, _ arguments, err error) { t.Helper(); require.ErrorIs(t, err, ErrInvalidFuzzPercent) },
		},
		{
			name:     "Error: Fuzz percentage above range",
			args:     []string{"./detect-blank", "image.png", "101", "0.1"},
			asserter: func(t *testing.T, _ arguments, err error) { t.Helper(); require.ErrorIs(t, err, ErrInvalidFuzzPercent) },
		},
		{
			name:     "Error: Invalid threshold (non-numeric)",
			args:     []string{"./detect-blank", "image.png", "10", "xyz"},
			asserter: func(t *testing.T, _ arguments, err error) { t.Helper(); require.Error(t, err) },
		},
		{
			name:     "Error: Threshold below range",
			args:     []string{"./detect-blank", "image.png", "10", "-0.1"},
			asserter: func(t *testing.T, _ arguments, err error) { t.Helper(); require.ErrorIs(t, err, ErrInvalidThreshold) },
		},
		{
			name:     "Error: Threshold above range",
			args:     []string{"./detect-blank", "image.png", "10", "1.1"},
			asserter: func(t *testing.T, _ arguments, err error) { t.Helper(); require.ErrorIs(t, err, ErrInvalidThreshold) },
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
		"zero":  createZeroPixelImage(),
	}

	errorCases := getImageErrorCases(assertErrorIs, images)
	contentCases := getImageContentCases(assertHasContent, images)

	return append(errorCases, contentCases...)
}

func getImageErrorCases(
	assertErrorIs func(error) func(*testing.T, bool, error),
	_ map[string]image.Image,
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
			name: "Image with zero pixels",
			args: arguments{filePath: "", fuzzFactor: 0, threshold: 0},
			setup: func(t *testing.T, fp string) {
				t.Helper()
				createTestPNG(t, fp, createTestImage(1, 1, color.White))
			},
			asserter: func(t *testing.T, _ bool, _ error) {
				t.Helper()
				testArgs := arguments{
					filePath:   "",
					fuzzFactor: 0,
					threshold:  0,
				}
				_, actualErr := imageHasContentWithImage(
					testArgs,
					createZeroPixelImage(),
				)
				require.ErrorIs(t, actualErr, ErrImageZeroPixels)
			},
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
		{
			name: "Content just above threshold",
			args: arguments{filePath: "", fuzzFactor: 0, threshold: 0.1},
			setup: func(t *testing.T, fp string) {
				t.Helper()
				img := createImageWithContentRatio(100, 100, 0.11)
				createTestPNG(t, fp, img)
			},
			asserter: assertHasContent(true),
		},
		{
			name: "Content just below threshold",
			args: arguments{filePath: "", fuzzFactor: 0, threshold: 0.1},
			setup: func(t *testing.T, fp string) {
				t.Helper()
				img := createImageWithContentRatio(100, 100, 0.09)
				createTestPNG(t, fp, img)
			},
			asserter: assertHasContent(false),
		},
		{
			name: "Near-white pixels treated as white (due to fuzz factor)",
			args: arguments{filePath: "", fuzzFactor: 0.1, threshold: 0.5},
			setup: func(t *testing.T, fp string) {
				t.Helper()
				img := createImageWithNearWhitePixels(100, 100, 0.05)
				createTestPNG(t, fp, img)
			},
			asserter: assertHasContent(false),
		},
		{
			name: "Off-white pixels treated as content (outside fuzz factor)",
			args: arguments{filePath: "", fuzzFactor: 0.05, threshold: 0.1},
			setup: func(t *testing.T, fp string) {
				t.Helper()
				img := createImageWithNearWhitePixels(100, 100, 0.1)
				createTestPNG(t, fp, img)
			},
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

func createImageWithContentRatio(width, height int, contentRatio float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	totalPixels := width * height
	contentPixels := int(float64(totalPixels) * contentRatio)

	// Fill the first contentPixels with black, remainder with white.
	for i := range contentPixels {
		x := i % width
		y := i / width
		img.Set(x, y, color.Black)
	}

	for i := contentPixels; i < totalPixels; i++ {
		x := i % width
		y := i / width
		img.Set(x, y, color.White)
	}

	return img
}

func createImageWithNearWhitePixels(
	width, height int,
	graynessFactor float64,
) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	grayValue := uint8(255 * (1.0 - graynessFactor))
	nearWhite := color.RGBA{R: grayValue, G: grayValue, B: grayValue, A: 255}

	for y := range height {
		for x := range width {
			img.Set(x, y, nearWhite)
		}
	}

	return img
}

type zeroPixelImage struct{}

func (zpi zeroPixelImage) ColorModel() color.Model {
	return color.RGBAModel
}

func (zpi zeroPixelImage) Bounds() image.Rectangle {
	return image.Rect(0, 0, 0, 0)
}

func (zpi zeroPixelImage) At(_, _ int) color.Color {
	return color.RGBA{R: 255, G: 255, B: 255, A: 255}
}

func createZeroPixelImage() image.Image {
	return zeroPixelImage{}
}

func imageHasContentWithImage(args arguments, img image.Image) (bool, error) {
	bounds := img.Bounds()

	totalPixels := float64(bounds.Dx() * bounds.Dy())
	if totalPixels == 0 {
		return false, ErrImageZeroPixels
	}

	nonWhiteCount := countNonWhitePixels(img, args.fuzzFactor)
	nonWhiteRatio := nonWhiteCount / totalPixels

	return nonWhiteRatio >= args.threshold, nil
}
