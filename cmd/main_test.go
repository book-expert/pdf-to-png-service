// Tests for the main pdf-to-png-service CLI.
//
// Focus areas:
// - mergeConfigAndFlags behavior: flags override config; config provides defaults.
// - Struct shapes for Options and config mappings.
package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"pdf-to-png-service/pdfrender"
)

func TestMergeConfigAndFlags(t *testing.T) {
	t.Parallel()

	testCases := getMergeConfigAndFlagsTestCases()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := mergeConfigAndFlags(
				&testCase.baseConfig,
				testCase.flags,
				testCase.projectRoot,
			)

			assert.Equal(t, testCase.expectedOptions, result)
		})
	}
}

// Defines the struct for our test cases.
type mergeConfigTestCase struct {
	name            string
	projectRoot     string
	flags           flags
	expectedOptions pdfrender.Options
	baseConfig      config
}

// Solves funlen: This function is now very short, composing the test suite
// from dedicated helper functions for each case.
func getMergeConfigAndFlagsTestCases() []mergeConfigTestCase {
	return []mergeConfigTestCase{
		getFlagsOverrideCase(),
		getConfigDefaultsCase(),
	}
}

// Solves funlen: The setup for the "flags override" test case is now in its own function.
func getFlagsOverrideCase() mergeConfigTestCase {
	return mergeConfigTestCase{
		name: "Flags should override all corresponding config values",
		baseConfig: config{
			Paths: configPaths{
				InputDir:  "/config/in",
				OutputDir: "/config/out",
			},
			LogsDir:  configLogsDir{PDFToPNG: "/logs/pdf_to_png"},
			Settings: configSettings{DPI: 200, Workers: 4},
			BlankDetection: configBlankDetection{
				FuzzPercent:       5,
				NonWhiteThreshold: 0.05,
			},
		},
		flags: flags{
			inputPath:  "/flag/in",
			outputPath: "/flag/out",
			dpi:        300,
			workers:    8,
		},
		projectRoot: "/root",
		expectedOptions: pdfrender.Options{
			ProgressBarOutput:      io.Writer(nil),
			ProjectRoot:            "/root",
			InputPath:              "/flag/in",
			OutputPath:             "/flag/out",
			DPI:                    300,
			Workers:                8,
			BlankFuzzPercent:       5,
			BlankNonWhiteThreshold: 0.05,
		},
	}
}

// Solves funlen: The setup for the "config defaults" test case is now in its own
// function.
func getConfigDefaultsCase() mergeConfigTestCase {
	return mergeConfigTestCase{
		name: "Config values should be used when flags are not provided",
		baseConfig: config{
			Paths: configPaths{
				InputDir:  "/config/in",
				OutputDir: "/config/out",
			},
			LogsDir:  configLogsDir{PDFToPNG: ""},
			Settings: configSettings{DPI: 150, Workers: 2},
			BlankDetection: configBlankDetection{
				FuzzPercent:       0,
				NonWhiteThreshold: 0,
			},
		},
		flags: flags{
			inputPath:  "",
			outputPath: "",
			dpi:        0,
			workers:    0,
		},
		projectRoot: "/root",
		expectedOptions: pdfrender.Options{
			ProgressBarOutput:      io.Writer(nil),
			ProjectRoot:            "/root",
			InputPath:              "/config/in",
			OutputPath:             "/config/out",
			DPI:                    150,
			Workers:                2,
			BlankFuzzPercent:       0,
			BlankNonWhiteThreshold: 0,
		},
	}
}
