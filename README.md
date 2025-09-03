# PDF to PNG Service

A small Go service/CLI that converts PDF files to PNG images. It discovers PDFs in an input directory, renders each page to PNG using Ghostscript, and optionally removes pages considered "blank" using a fast helper tool. Progress is displayed via a progress bar, and structured logs are written to a logs directory.

This repository contains:
- cmd/main.go: the main CLI entrypoint.
- pdfrender: the library that performs discovery, rendering, and blank detection.
- cmd/detect-blank: a tiny helper binary used to quickly determine if a PNG page is blank (based on pixel content).

## Features
- Discover all PDFs in a directory (non-recursive) and process each file.
- Render each page as a PNG with configurable DPI.
- Concurrent worker support for faster processing.
- Optional blank-page detection and removal using a compiled helper binary.
- Progress bar output to stdout.
- File-based logging with time‑stamped log files.

## Requirements
- Go (see go.mod for the exact version; Go 1.21+ recommended).
- Ghostscript (binary `ghostscript`).
- pdfinfo (from Poppler utilities, binary `pdfinfo`).
- A POSIX‑like environment is recommended for best compatibility.

Make sure `ghostscript` and `pdfinfo` are available in your PATH.

## Installation
You can build the main tool with:

```
go build -o bin/pdf-to-png ./cmd
```

On first run, the service will automatically build the helper binary `detect-blank` (from `./cmd/detect-blank`) into `./bin/detect-blank` if it does not already exist. Ensure you have a working Go toolchain for that step as well.

Alternatively, you can run via `go run`:

```
go run ./cmd --input /path/to/pdfs --output /path/to/output
```

## Configuration
The service can read configuration from a TOML file discovered via `github.com/nnikolov3/configurator` starting from the current directory upwards. The resolved config path is passed to the loader (`loadConfig`). If the file is missing, defaults/flags are used.

Configuration structure (TOML keys):

```toml
[paths]
input_dir = "/path/to/input"
output_dir = "/path/to/output"

[logs_dir]
pdf_to_png = "/path/to/logs/pdf_to_png"  # optional; default is <projectRoot>/logs/pdf_to_png

[settings]
dpi = 200         # default 200 if <= 0
workers = 0       # default = runtime.NumCPU() if <= 0

[blank_detection]
fast_fuzz_percent = 5           # default 5 if <= 0
fast_non_white_threshold = 0.005 # default 0.005 if <= 0
```

Notes:
- If a value is 0 or empty, sensible defaults are applied by the code.
- Command‑line flags override configuration values.

## Command‑line Usage
Flags for the main command:
- --input string   Input directory for PDF files (required)
- --output string  Output directory for PNG files (required)
- --dpi int        Resolution in DPI for output images (optional)
- --workers int    Number of concurrent workers (optional)

Examples:

Render using only flags:
```
go run ./cmd \
  --input /data/pdfs \
  --output /data/output \
  --dpi 300 \
  --workers 8
```

Render using config (flags override):
```
# project.toml located at or above the working directory with desired values

go run ./cmd --dpi 150
```

### Output layout
For each PDF named `somefile.pdf`, images are written as PNGs under:
```
<output_dir>/somefile/png/
```

### Logs
Logs are written to a directory named by:
- The `logs_dir.pdf_to_png` configuration key, if set; otherwise
- `<projectRoot>/logs/pdf_to_png`.

Each run creates a timestamped log file, e.g. `log_20250101_123000.log`.

## Blank Detection
After each page is rendered, the tool executes the helper binary `detect-blank` with:
```
<path-to-png> <fuzzPercent> <nonWhiteThreshold>
```
- Exit code 0: the image is blank; the PNG file is removed.
- Exit code 1: the image has content.
- Other exit codes: treated as an error.

The parameters correspond to:
- fast_fuzz_percent: integer percent (0–100). Larger values treat near‑white pixels as white.
- fast_non_white_threshold: ratio (0.0–1.0). Minimum fraction of non‑white pixels to consider the page non‑blank.

## Development
Run tests:
```
make test
# or
go test ./...
```

Run lint (golangci-lint must be installed):
```
make lint
```

Project structure:
- cmd/main.go: CLI entrypoint that parses flags, loads config, sets up logging, and runs the processor.
- pdfrender: library with processing logic, rendering via Ghostscript, discovery utilities, and building/running the blank detection helper when needed.
- cmd/detect-blank: helper CLI that evaluates whether a PNG is blank.

Key options and defaults (from code):
- DPI: default 200
- Workers: default runtime.NumCPU()
- Blank fuzz percent: default 5
- Blank non‑white threshold: default 0.005

## Troubleshooting
- Error: "could not find project root": Make sure you run the tool from within the project (or adjust accordingly) so `configurator.FindProjectRoot(".")` can resolve the root.
- Error building `detect-blank`: Ensure the Go toolchain is installed and `go build` works; check permissions for the `bin/` directory.
- "pdfinfo execution failed": Verify that `pdfinfo` is installed and visible in PATH.
- "ghostscript execution failed": Verify that Ghostscript is installed and visible in PATH.
- No PDFs found: Ensure the `--input` directory exists and contains `.pdf` files (non‑recursive search).

## License
Add your preferred license here (e.g., MIT).