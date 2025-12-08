#!/bin/bash
set -euo pipefail

# Configuration
NATS_SERVER="nats://192.168.122.102:4222"
PDF_FILE="test.pdf"
PDF_KEY="integration-test-doc"
WORKFLOW_ID="test-workflow-$(date +%s)"

echo "----------------------------------------------------------------"
echo "Integration Test Script for PDF-to-PNG Service"
echo "----------------------------------------------------------------"

# 1. Check prerequisites
if ! command -v nats &> /dev/null; then
    echo "Error: 'nats' CLI tool is required but not installed."
    exit 1
fi

if [ ! -f "$PDF_FILE" ]; then
    echo "Error: '$PDF_FILE' not found. Please ensure it exists."
    exit 1
fi

# 2. Setup NATS Context
echo "-> Setting up NATS context..."
nats context save local --server "$NATS_SERVER" > /dev/null || true
nats context select local

# 3. Ensure Streams and Buckets exist
echo "-> Ensuring NATS resources exist..."
nats object add PDF_FILES > /dev/null 2>&1 || true
nats object add PNG_FILES > /dev/null 2>&1 || true
nats stream add PDFS --subjects "pdfs.created" --storage file --retention limits --max-msgs 10000 > /dev/null 2>&1 || true
nats stream add PNGS --subjects "pngs.created" --storage file --retention limits --max-msgs 50000 > /dev/null 2>&1 || true

# 4. Upload PDF
echo "-> Uploading '$PDF_FILE' to PDF_FILES bucket as key '$PDF_KEY' நான"
nats object put PDF_FILES "$PDF_FILE" --name "$PDF_KEY" --force

# 5. Subscribe in background to catch the result
echo "-> Listening for completion events (timeout in 30s)..."
# We run this in background and kill it after we trigger
(nats sub "pngs.created" --count 1 --timeout 30s && echo -e "\n✅ Success! Received 'pngs.created' event.") &
SUB_PID=$!

# Give the subscriber a moment to connect
sleep 1

# 6. Trigger Event
echo "-> Publishing 'pdfs.created' event..."
JSON_PAYLOAD=$(cat <<EOF
{
  "header": {
    "workflow_id": "$WORKFLOW_ID",
    "user_id": "integration-tester",
    "tenant_id": "default",
    "event_id": "evt-$(date +%s)",
    "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  },
  "pdf_key": "$PDF_KEY",
  "augmentation": null
}
EOF
)

nats pub "pdfs.created" "$JSON_PAYLOAD"

# 7. Wait for subscriber
wait $SUB_PID || { echo "❌ Failed: Timed out waiting for response event."; exit 1; }

# 8. List and Download generated files
echo "-> Verifying output files in PNG_FILES bucket..."
nats object ls PNG_FILES

echo "-> Downloading generated PNGs to ./output directory..."
mkdir -p output
# Get the list of files matching our key pattern and download them
# We use 'nats object ls' and parse the output to find filenames starting with our key
# Note: 'nats object ls' output format might vary, so we'll try a simple loop for the first few pages 
# which is safer for a test script than parsing CLI output.

for i in {1..5}; do
    FILENAME="${PDF_KEY}-${i}.png"
    if nats object info PNG_FILES "$FILENAME" >/dev/null 2>&1; then
        echo "   Downloading $FILENAME..."
        nats object get PNG_FILES "$FILENAME" --output "output/$FILENAME" --force
    fi
done

echo "-> Check ./output directory for the images."

echo "----------------------------------------------------------------"
echo "Test Complete."
