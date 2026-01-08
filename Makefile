.PHONY: all build run install test test-v test-update clean lint mocks mocks-clean playground up down jaeger

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

# Builds and starts the playground
playground: build
	./perles playground

# Build and run the binary with the debug flag
debug: build
	./perles -d

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
		./internal/ui/coleditor/... \
		./internal/ui/commandpalette/... \
		./internal/ui/details/... \
		./internal/ui/modals/help/... \
		./internal/ui/modals/issueeditor/... \
		./internal/ui/nobeads/... \
		./internal/ui/outdated/... \
		./internal/ui/shared/colorpicker/... \
		./internal/ui/shared/diffviewer/... \
		./internal/ui/shared/issuebadge/... \
		./internal/ui/shared/logoverlay/... \
		./internal/ui/shared/modal/... \
		./internal/ui/shared/overlay/... \
		./internal/ui/shared/picker/... \
		./internal/ui/shared/toaster/... \
		./internal/ui/shared/vimtextarea/... \
		./internal/ui/styles/... \
		./internal/ui/tree/... \
		./internal/mode/search/... \
		./internal/mode/orchestration/... \
		./internal/mode/playground/... \
		-update

# Run linter (requires golangci-lint)
lint:
	golangci-lint run ./...

# Generate all mocks (clean first)
mocks: mocks-clean
	mockery

# Clean generated mocks
mocks-clean:
	@rm -rf ./internal/mocks

# Clean build artifacts
clean:
	rm -f perles
	go clean ./...

# Start docker-compose services (Jaeger for tracing)
up:
	docker-compose up -d

# Stop docker-compose services
down:
	docker-compose down

# Open Jaeger UI in browser
jaeger:
	open http://localhost:16686
