.PHONY: build clean test run install build-mlx-server

# Build variables
BINARY_NAME=toke
BUILD_DIR=build

# Read current version from VERSION file
CURRENT_VERSION=$(shell cat VERSION 2>/dev/null || echo "0.4201")
# Auto-increment version for builds (adds 1 to last part)
VERSION=v$(shell echo $(CURRENT_VERSION) | awk -F. '{printf "%d.%d", $$1, $$2+1}')
# Keep current version without incrementing (for reference)
BASE_VERSION=v$(CURRENT_VERSION)

GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X github.com/chasedut/toke/internal/version.Version=${VERSION} \
	-X github.com/chasedut/toke/internal/version.BuildTime=${BUILD_TIME} \
	-X github.com/chasedut/toke/internal/version.GitCommit=${GIT_COMMIT}"

# Default target
all: build

# Build the main binary
build:
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Updating VERSION file to $(shell echo $(VERSION) | sed 's/^v//')..."
	@echo $(shell echo $(VERSION) | sed 's/^v//') > VERSION
	@echo "Build complete: $(BINARY_NAME) $(VERSION)"

# Build for all platforms
build-all: build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-windows-amd64

build-darwin-arm64:
	@echo "Building for macOS ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .

build-darwin-amd64:
	@echo "Building for macOS AMD64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .

build-linux-amd64:
	@echo "Building for Linux AMD64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

build-windows-amd64:
	@echo "Building for Windows AMD64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

# Build the MLX server bundle (requires Python environment)
build-mlx-server:
	@echo "Building MLX server bundle..."
	@chmod +x scripts/build-mlx-server.sh
	@scripts/build-mlx-server.sh

# Build the llama.cpp server (if needed locally)
build-llama-server:
	@echo "Building llama.cpp server..."
	@chmod +x scripts/build-llama-server.sh
	@scripts/build-llama-server.sh

# Install the binary to /usr/local/bin
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Installation complete!"

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -cover ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -rf build-mlx-server/
	@rm -rf build-llama-server/
	@echo "Clean complete!"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "Format complete!"

# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint &> /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: brew install golangci-lint"; \
	fi

# Check for vulnerabilities
vuln:
	@echo "Checking for vulnerabilities..."
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Update dependencies
deps:
	@echo "Updating dependencies..."
	go mod tidy
	go mod download
	@echo "Dependencies updated!"

# Development setup
dev-setup: deps
	@echo "Setting up development environment..."
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Development setup complete!"

# Set a specific version
set-version:
	@if [ -z "$(V)" ]; then \
		echo "Usage: make set-version V=0.4300"; \
		exit 1; \
	fi
	@echo "Setting version to $(V)..."
	@echo $(V) > VERSION
	@echo "Version set to $(V)"

# Show current version
show-version:
	@echo "Current version: $(BASE_VERSION)"
	@echo "Next build will be: $(VERSION)"

# Help target
help:
	@echo "Available targets:"
	@echo "  build           - Build the binary for current platform"
	@echo "  build-all       - Build for all supported platforms"
	@echo "  build-mlx-server - Build the MLX server bundle (macOS only)"
	@echo "  install         - Install binary to /usr/local/bin"
	@echo "  run             - Build and run the application"
	@echo "  test            - Run tests"
	@echo "  test-coverage   - Run tests with coverage"
	@echo "  clean           - Remove build artifacts"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linter"
	@echo "  vuln            - Check for vulnerabilities"
	@echo "  deps            - Update dependencies"
	@echo "  dev-setup       - Setup development environment"
	@echo "  help            - Show this help message"