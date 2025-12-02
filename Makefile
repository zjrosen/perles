.PHONY: all build run install test test-v test-update clean lint

# Version from git (tag or commit hash)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X 'main.version=$(VERSION)'

# Default target
all: build test

# Build the binary with version info
build:
	go build -ldflags "$(LDFLAGS)" -o perles .

# Build and run the binary
run: build
	./perles

# Install the binary to $GOPATH/bin with version info
install:
	go install -ldflags "$(LDFLAGS)" .

# Run all tests
test:
	go test ./...

# Run tests with verbose output
test-v:
	go test -v ./...

# Update golden test files (for teatest snapshot tests)
test-update:
	@echo "Updating golden files in packages with teatest..."
	@go test ./internal/ui/board/... \
		./internal/ui/bqlinput/... \
		./internal/ui/coleditor/... \
		./internal/ui/colorpicker/... \
		./internal/ui/details/... \
		./internal/ui/help/... \
		./internal/ui/labeleditor/... \
		./internal/ui/modal/... \
		./internal/ui/overlay/... \
		./internal/ui/picker/... \
		./internal/ui/saveactionpicker/... \
		./internal/ui/toaster/... \
		./internal/ui/viewselector/... \
		./internal/mode/search/... \
		-update

# Run linter (requires golangci-lint)
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -f perles
	go clean ./...
