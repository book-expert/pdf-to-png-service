// File: ./cmd/main_test.go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"pdf-to-png-service/pdfrender"
)

// TestMergeConfigAndFlags verifies that command-line flags correctly override config file
// settings.
func TestMergeConfigAndFlags(t *testing.T) {
	testCases := []struct {
		expectedOptions pdfrender.Options
		baseConfig      config
		name            string
		projectRoot     string
		flags           flags
	}{
		{
			name: "Flags should override all corresponding config values",
			baseConfig: config{
				Paths: struct {
					InputDir  string `toml:"input_dir"`
					OutputDir string `toml:"output_dir"`
				}{InputDir: "/config/in", OutputDir: "/config/out"},
				Settings: struct {
					DPI     int `toml:"dpi"`
					Workers int `toml:"workers"`
				}{DPI: 200, Workers: 4},
			},
			flags: flags{
				inputPath:  "/flag/in",
				outputPath: "/flag/out",
				dpi:        300,
				workers:    8,
			},
			projectRoot: "/root",
			expectedOptions: pdfrender.Options{
				ProjectRoot: "/root",
				InputPath:   "/flag/in",
				OutputPath:  "/flag/out",
				DPI:         300,
				Workers:     8,
			},
		},
		{
			name: "Config values should be used when flags are not provided",
			baseConfig: config{
				Paths: struct {
					InputDir  string `toml:"input_dir"`
					OutputDir string `toml:"output_dir"`
				}{InputDir: "/config/in", OutputDir: "/config/out"},
				Settings: struct {
					DPI     int `toml:"dpi"`
					Workers int `toml:"workers"`
				}{DPI: 150, Workers: 2},
			},
			flags:       flags{}, // No flags provided.
			projectRoot: "/root",
			expectedOptions: pdfrender.Options{
				ProjectRoot: "/root",
				InputPath:   "/config/in",
				OutputPath:  "/config/out",
				DPI:         150,
				Workers:     2,
			},
		},
		{
			name: "Blank detection values from config should be preserved",
			baseConfig: config{
				BlankDetection: struct {
					FuzzPercent       int     `toml:"fast_fuzz_percent"`
					NonWhiteThreshold float64 `toml:"fast_non_white_threshold"`
				}{FuzzPercent: 10, NonWhiteThreshold: 0.1},
			},
			flags:       flags{},
			projectRoot: "/root",
			expectedOptions: pdfrender.Options{
				ProjectRoot:            "/root",
				BlankFuzzPercent:       10,
				BlankNonWhiteThreshold: 0.1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set default values for expected options so we don't have to
			// repeat them in every test case.
			if tc.expectedOptions.BlankFuzzPercent == 0 {
				tc.expectedOptions.BlankFuzzPercent = 5 // Default value
			}

			if tc.expectedOptions.BlankNonWhiteThreshold == 0 {
				tc.expectedOptions.BlankNonWhiteThreshold = 0.005 // Default value
			}

			result := mergeConfigAndFlags(
				tc.baseConfig,
				tc.flags,
				tc.projectRoot,
			)

			// We don't care about the progress bar output in this test.
			result.ProgressBarOutput = nil
			tc.expectedOptions.ProgressBarOutput = nil

			assert.Equal(t, tc.expectedOptions, result)
		})
	}
}
