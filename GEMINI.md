# GEMINI.md - PDF to PNG Service

## Service Overview
This service is the **entry point** of the processing pipeline. It accepts PDF files and converts them into a sequence of PNG images (one per page).

## Architecture & Data Flow
1.  **Input**: Listens to NATS JetStream subject `pdfs.created`.
    -   Payload: JSON event containing `pdf_object_key` and **JobSettings** (`Scene`, `Style`, `Accent`, `Articulation`, `Pace`, `Personality`).
2.  **Processing**:
    -   Downloads the PDF.
    -   **The Brain**: Calls Gemini to analyze the PDF + User Settings and generate a **Master Narration Directive** (Markdown).
    -   **Rendering**: Renders pages as PNGs (skipping blank ones).
3.  **Output**: Publishes events to `pngs.created`.
    -   Payload: `PNGCreatedEvent` carrying the `MasterDirective` (AudioSessionConfig) and `JobSettings` to downstream services.

## Configuration
-   **Config File**: `project.toml`
-   **Key Settings**:
    -   `analysis_prompt`: The template for generating the Master Directive.
    -   `workers`: Parallel processing count.
    -   `dpi`: Rendering resolution (Default: 300).

## Dependencies
-   **System Tools**: `ghostscript`, `pdfinfo` (poppler-utils).
-   **Infrastructure**: NATS JetStream.

## Current Status (Dec 13, 2025)
-   **Health**: ✅ Healthy
-   **Features**:
    -   **Master Directive**: Generates a persistent "Director Mode" context for the entire book.
    -   **Settings Propagation**: Passes user intent (`Style`, `Pace`, etc.) correctly.
    -   **Robustness**: Handles blank pages and rendering errors.
