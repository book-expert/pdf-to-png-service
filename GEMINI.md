# GEMINI.md - PDF to PNG Service

## Service Overview
This service is the **entry point** of the processing pipeline. It accepts PDF files and converts them into a sequence of PNG images (one per page).

## Architecture & Data Flow
1.  **Input**: Listens to NATS JetStream subject `pdfs.created`.
    -   Payload: JSON event containing `pdf_object_key`.
2.  **Processing**:
    -   Downloads the PDF from the Object Store.
    -   Renders each page as a PNG image.
    -   Uploads each PNG to the Object Store (`png_bucket`).
3.  **Output**: Publishes events to `pngs.created`.
    -   Payload: `PNGCreatedEvent` (contains `png_object_key`, `page_number`, `workflow_id`).

## Configuration
-   **Config File**: `project.toml`
-   **Concurrency**: Controlled via `workers` setting.

## Current Status (Dec 12, 2025)
-   **Health**: ✅ Healthy
-   **Performance**: High.
-   **Dependencies**: `mupdf` (or similar rendering lib), NATS.
