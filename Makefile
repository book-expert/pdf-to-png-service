# PDF-to-PNG Service Makefile

.PHONY: build install clean test lint fmt help

# Build configuration
BINARY := pdf-to-png-service
INSTALL_DIR := $(HOME)/bin

# Go build flags
LDFLAGS := -w -s
BUILD_FLAGS := -ldflags="$(LDFLAGS)"

# Default target
all: build test lint

# Build the service
build:
	@echo "Building $(BINARY)..."
	go build -o ./$(BINARY) ./cmd

# Install binary to ~/bin
install: build
	@echo "Installing $(BINARY) to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	cp ./$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "✅ $(BINARY) installed to $(INSTALL_DIR)/$(BINARY)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f ./$(BINARY)
	rm -f logs/*.log

# Run tests
test:
	@echo "Running tests..."
	go test -v ./cmd

# Run linter
lint:
	@echo "Running golangci-lint..."
	@gofumpt -l -w .
	@golangci-lint run --fix
	@go vet ./...
	@go test -race ./...
	@staticcheck ./...
	@golangci-lint cache clean
	@go clean -cache

# Format Go code
fmt:
	@echo "Formatting Go code..."
	go fmt ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  build   - Build the PDF-to-PNG service into project root"
	@echo "  install - Install service binary to ~/bin"
	@echo "  clean   - Clean build artifacts"
	@echo "  test    - Run tests"
	@echo "  lint    - Run golangci-lint"
	@echo "  fmt     - Format Go code"
	@echo "  help    - Show this help"
