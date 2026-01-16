# PDF-to-PNG Service

The **PDF-to-PNG Service** is a high-performance Go microservice that serves as the entry point and intelligence engine for the document processing pipeline. It handles the conversion of PDF documents into high-quality PNG images while simultaneously analyzing document structure to guide the entire book-to-audio workflow.

## Overview

This service integrates with Google Gemini to perform "Document Analysis" upon upload. It generates a persistent **Master Narration Directive** that instructs subsequent services on how to handle text extraction and synthesis, ensuring a consistent tone, style, and musical atmosphere throughout the process.

## Key Features

- **High-Quality Rendering**: Uses Ghostscript to render PDF pages into PNG images at 300 DPI for maximum OCR accuracy.
- **Document Intelligence (Gemini Integration)**:
    - Analyzes document context to create tailored "Text Directives" (e.g., "Ignore citations," "Focus on main body").
    - Generates complex music configurations based on the document's mood and tone.
- **Advanced Processing**:
    - **Blank Page Detection**: Automatically skips pages with insufficient content to optimize cost.
    - **Metadata Extraction**: Uses `pdfinfo` to accurately determine document properties and page counts.
    - **Event-Driven Workflow**: Powered by the `common-worker` library for reliable, asynchronous processing.
    - **Robust Storage**: Integrates with NATS Object Store for high-integrity asset management.

## 🛡️ Alignment with Project Standards

This service adheres to the **Manifesto of Truth** and project engineering standards:
- **Whole Words Only**: Naming conventions avoid abbreviations (e.g., `directive`, `configuration`, `context`).
- **Care**: Implements blank page detection and DPI optimizations to ensure the highest quality input for the OCR stage.
- **Craftsmanship**: Gemini prompts are carefully designed to provide actionable intelligence to downstream services.

## Requirements

- Go 1.25.5+
- NATS Server with JetStream enabled
- **Ghostscript** (`gs`): Required for PDF-to-Image conversion.
- **Poppler Utils** (`pdfinfo`): Required for document analysis.
- **Gemini API Key**: Required for document intelligence features.

## Configuration

The service is configured via `project.toml`. Key areas include:

- `[service]`: Worker count, DPI settings, and blank page detection thresholds.
- `[llm]`: Model settings and prompts for generating directives and music configurations.
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
- `internal/analyzer`: Gemini-powered logic for generating directives.
- `internal/pdfrender`: Ghostscript wrapper for high-fidelity rendering.
- `internal/worker`: Core orchestration logic for the document workflow.

## Events

### Consumes
- `pdfs.created`: Triggered when a new PDF is uploaded to the system.

### Produces
- `pngs.created`: Triggered for every successfully rendered PNG page. Includes the Master Narration Directive and Music Configuration.

---
*Built with ❤️, Craftsmanship, and Discipline.*
