# GEMINI.md - PDF to PNG Service

## Service Overview
This service is the **entry point** of the processing pipeline. It accepts PDF files and converts them into a sequence of PNG images (one per page).

## Architecture & Data Flow
1.  **Input**: Listens to NATS JetStream subject `pdfs.created`.
    -   Payload: JSON event containing `pdf_object_key` and **JobSettings** (Style, Voice, Language, etc.).
2.  **Processing**:
    -   Downloads the PDF from the Object Store (`PDF_FILES`).
    -   **Blank Detection**: Scans pages for content. Skips blank pages.
    -   **Rendering**: Uses Ghostscript to render each page as a high-quality PNG.
    -   Uploads each PNG to the Object Store (`PNG_FILES`).
3.  **Output**: Publishes events to `pngs.created`.
    -   Payload: `PNGCreatedEvent` (propagates **JobSettings** to downstream services).

## Configuration
-   **Config File**: `project.toml`
-   **Key Settings**:
    -   `workers`: Parallel processing count (Default: 4).
    -   `dpi`: Rendering resolution (Default: 300).
    -   `blank_fuzz_percent`: Sensitivity for blank page detection.

## Dependencies
-   **System Tools**: `ghostscript`, `pdfinfo` (poppler-utils).
-   **Infrastructure**: NATS JetStream (Messaging & Object Store).

## Current Status (Dec 12, 2025)
-   **Health**: ✅ Healthy
-   **Features**:
    -   **Settings Propagation**: Successfully passes `JobSettings` (Voice, Style, Language) to downstream events.
    -   **Robustness**: Handles blank pages and rendering errors gracefully.
