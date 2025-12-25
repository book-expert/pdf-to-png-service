# PDF-to-PNG Service

## Project Summary

A NATS-based microservice that converts PDF files to PNG images.

## Detailed Description

This service listens for `pdfs.created` messages on a NATS stream. When a message is received, it downloads the PDF file from a NATS object store, converts each page to a PNG image using Ghostscript, and uploads the images to another NATS object store. For each generated PNG, it publishes a `pngs.created` event.

This service is a key component in the document processing pipeline, enabling subsequent services to work with images instead of PDF files.

Core capabilities include:

-   **Document Analysis (LLM Integration)**: Analyzes the PDF content and user preferences (Style, Pace, etc.) using Gemini to generate a persistent "Master Narration Directive" that guides the tone of the entire audiobook.
-   **NATS Integration**: Seamlessly integrates with NATS for messaging and object storage.
-   **Concurrent Processing**: Utilizes concurrent workers to accelerate the conversion process.
-   **High-Quality Rendering**: Renders each PDF page as a PNG image with configurable DPI using Ghostscript.
-   **Integrated Blank Page Detection**: Automatically detects and skips blank pages during processing.
-   **Robust Error Handling**: Implements `ack`, `nak`, and `term` logic for handling NATS messages and DLQ support.

## Technology Stack

-   **Programming Language:** Go
-   **Messaging:** NATS JetStream
-   **Dependencies:**
    -   `ghostscript`: For PDF rendering.
    -   `poppler-utils` (specifically `pdfinfo`): For page counting.

## Configuration

The service is configured via a local `project.toml` file.

```toml
[service]
log_dir = "./logs"
workers = 4
dpi = 300
blank_fuzz_percent = 5
blank_non_white_threshold = 0.005

[nats]
url = "nats://192.168.122.102:4222" # Adjust to your NATS server
dlq_subject = "pdf-to-png.dlq"

[nats.consumer]
stream = "PDFS"
subject = "pdfs.created"
durable = "pdf-to-png-durable"

[nats.producer]
stream = "PNGS"
subject = "pngs.created"

[nats.object_store]
pdf_bucket = "PDF_FILES"
png_bucket = "PNG_FILES"
```

## Prerequisites

Ensure the following system dependencies are installed:

-   **Ghostscript** (`ghostscript`)
-   **Poppler Utils** (`poppler-utils` on Debian/Ubuntu, `poppler-utils` on Fedora/RHEL)

## Usage

To run the service:

```bash
make run
# OR
go run cmd/main.go
```

## Development

To build the service:
```bash
make build
```

To run linting:
```bash
make lint
```


