// File: ./cmd/main.go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/nnikolov3/configurator"
	"github.com/nnikolov3/logger"

	"pdf-to-png-service/pdfrender"
)

// Define named types for each section of the configuration.
type configPaths struct {
	InputDir  string `toml:"input_dir"`
	OutputDir string `toml:"output_dir"`
}

type configLogsDir struct {
	PDFToPNG string `toml:"pdf_to_png"`
}

type configSettings struct {
	DPI     int `toml:"dpi"`
	Workers int `toml:"workers"`
}

type configBlankDetection struct {
	FuzzPercent       int     `toml:"fast_fuzz_percent"`
	NonWhiteThreshold float64 `toml:"fast_non_white_threshold"`
}

// config represents the structure of the project.toml file, now using named types.
type config struct {
	Paths          configPaths          `toml:"paths"`
	LogsDir        configLogsDir        `toml:"logs_dir"`
	Settings       configSettings       `toml:"settings"`
	BlankDetection configBlankDetection `toml:"blank_detection"`
}

func main() {
	ctx := context.Background()
	// The `run` function contains the core application logic.
	// We call it and then os.Exit to ensure deferred functions are run correctly.
	err := run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run is the main logic function, separated from main to allow for easier testing and
// clean exit handling.
func run(ctx context.Context) error {
	projectRoot, configPath, err := configurator.FindProjectRoot(".")
	if err != nil {
		return fmt.Errorf("could not find project root: %w", err)
	}

	cfg, err := safeLoadConfig(configPath)
	if err != nil {
		return err
	}

	flgs := parseFlags()
	options := mergeConfigAndFlags(&cfg, flgs, projectRoot)

	return processWithLogger(ctx, &options, cfg.LogsDir.PDFToPNG)
}

// safeLoadConfig loads the TOML config, allowing missing file without error.
func safeLoadConfig(path string) (config, error) {
	cfg, err := loadConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			var emptyCfg config

			return emptyCfg, nil
		}

		return config{}, fmt.Errorf("error loading config file: %w", err)
	}

	return cfg, nil
}

// loadConfig reads and parses the project.toml file.
func loadConfig(path string) (config, error) {
	var cfg config

	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		var zero config

		return zero, fmt.Errorf("failed to decode config file: %w", err)
	}

	return cfg, nil
}

// flags represents the command-line arguments.
type flags struct {
	inputPath  string
	outputPath string
	dpi        int
	workers    int
}

// parseFlags defines and parses command-line flags.
func parseFlags() flags {
	var flagsVar flags
	flag.StringVar(
		&flagsVar.inputPath,
		"input",
		"",
		"Input directory for PDF files (required).",
	)
	flag.StringVar(
		&flagsVar.outputPath,
		"output",
		"",
		"Output directory for PNG files (required).",
	)
	flag.IntVar(&flagsVar.dpi, "dpi", 0, "Resolution in DPI for the output images.")
	flag.IntVar(&flagsVar.workers, "workers", 0, "Number of concurrent workers.")
	flag.Parse()

	return flagsVar
}

// mergeConfigAndFlags combines settings from the config file and command-line flags.
// Flags take precedence over the config file settings.
func mergeConfigAndFlags(cfg *config, flgs flags, projectRoot string) pdfrender.Options {
	opts := pdfrender.Options{
		ProgressBarOutput:      nil,
		ProjectRoot:            projectRoot,
		InputPath:              cfg.Paths.InputDir,
		OutputPath:             cfg.Paths.OutputDir,
		DPI:                    cfg.Settings.DPI,
		Workers:                cfg.Settings.Workers,
		BlankFuzzPercent:       cfg.BlankDetection.FuzzPercent,
		BlankNonWhiteThreshold: cfg.BlankDetection.NonWhiteThreshold,
	}

	// Command-line flags override config file values.
	if flgs.inputPath != "" {
		opts.InputPath = flgs.inputPath
	}

	if flgs.outputPath != "" {
		opts.OutputPath = flgs.outputPath
	}

	if flgs.dpi > 0 {
		opts.DPI = flgs.dpi
	}

	if flgs.workers > 0 {
		opts.Workers = flgs.workers
	}

	return opts
}

// processWithLogger sets up the logger and runs the processor.
func processWithLogger(
	ctx context.Context,
	options *pdfrender.Options,
	logDir string,
) error {
	log, err := setupLogger(options.ProjectRoot, logDir)
	if err != nil {
		return fmt.Errorf("could not set up logger: %w", err)
	}

	defer func() {
		cerr := log.Close()
		if cerr != nil {
			_, _ = fmt.Fprintf(
				os.Stderr,
				"failed to close logger: %v\n",
				cerr,
			)
		}
	}()

	processor := pdfrender.NewProcessor(options, log)

	procErr := processor.Process(ctx)
	if procErr != nil {
		return fmt.Errorf("PDF processing failed: %w", procErr)
	}

	return nil
}

// setupLogger initializes the logger, creating the log directory if needed.
func setupLogger(projectRoot, logDirConfig string) (*logger.Logger, error) {
	logDir := logDirConfig
	if logDir == "" {
		logDir = filepath.Join(projectRoot, "logs", "pdf_to_png")
	}

	logFileName := fmt.Sprintf("log_%s.log", time.Now().Format("20060102_150405"))

	log, err := logger.New(logDir, logFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	return log, nil
}
