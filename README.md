# PDF-to-PNG Service

The **PDF-to-PNG Service** is a high-performance Go microservice that serves as the entry point and intelligence engine for the document processing pipeline. It handles the initial stage of converting PDF documents into a series of high-quality PNG images while simultaneously analyzing the document structure to guide the entire book-to-audio workflow.

## Overview

This service is more than a simple converter; it integrates with Google Gemini to perform "Document Analysis" upon upload. It generates a persistent **Master Narration Directive** that instructs subsequent services on how to handle text extraction and synthesis, ensuring a consistent tone and style throughout the process.

## Key Features

- **High-Quality Rendering**: Uses Ghostscript to render PDF pages into PNG images at configurable DPI (default: 300).
- **Document Intelligence (Gemini Integration)**:
    - Analyzes document context to create tailored "Text Directives" (e.g., "Ignore citations," "Focus on main body").
    - Generates complex music generation configurations for background soundscapes based on the document's mood and tone.
- **Advanced Processing**:
    - **Blank Page Detection**: Automatically skips pages with insufficient content to save processing time and cost.
    - **Metadata Extraction**: Uses `pdfinfo` to accurately determine document properties and page counts.
    - **Event-Driven Workflow**: Consumes `pdfs.created` events and produces `pngs.created` events for each valid page, utilizing the `common-worker` library for reliable processing.
    - **Robust Storage**: Integrates with NATS Object Store for retrieving source PDFs and storing resulting PNGs.
    
    ## Requirements
    - Go 1.25.5+
- NATS Server with JetStream enabled
- **Ghostscript** (`gs`): Required for PDF-to-Image conversion.
- **Poppler Utils** (`pdfinfo`): Required for document analysis.
- **Gemini API Key**: Required for document intelligence features.

## Configuration

The service is configured via `project.toml`. Key areas include:

- `[service]`: Worker count, DPI settings, and blank page detection thresholds.
- `[llm]`: Model settings (e.g., `gemini-2.5-flash`) and prompts for generating directives and music configurations.
- `[nats]`: Stream, subject, and object store bucket configurations.

## Getting Started

### Installation

```bash
make install
```

### Building

```bash
make build
```

### Running

```bash
make run
```

## Internal Architecture

- `cmd/pdf-to-png-service`: Application initialization and NATS worker startup.
- `internal/analyzer`: Gemini-powered logic for generating text and music directives.
- `internal/pdfrender`: Ghostscript wrapper for high-fidelity PDF rendering.
- `internal/worker`: Core orchestration logic for the document processing workflow.
- `internal/config`: Configuration loading and validation.

## Events

### Consumes
- `pdfs.created`: Triggered when a new PDF is uploaded to the system.

### Produces
- `pngs.created`: Triggered for every successfully rendered PNG page. Includes the Master Narration Directive and Music Configuration in the metadata.