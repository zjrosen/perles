.PHONY: all build build-go build-frontend run run-all install test test-v test-update clean lint mocks mocks-clean playground up down jaeger daemon docs index-docs

# Version from git (tag or commit hash)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X 'main.version=$(VERSION)'
BUILD_LDFLAGS := -s -w $(LDFLAGS)

# Default target
all: build test

# Build frontend (requires Node.js)
build-frontend:
	@echo "Building frontend..."
	cd frontend && npm install && npm run build

# Go-only build (assumes frontend is pre-built)
build-go:
	go build -trimpath -ldflags "$(BUILD_LDFLAGS)" -o perles .

build: build-frontend build-go

# Build the frontend and back and run the binary
run-all: build
	./perles

# Build and run the binary
run: build-go
	./perles

# Build dameon 
daemon: build-go
	./perles daemon -p 19999

# Builds and starts the playground
playground: build-go
	./perles playground

# Build and run the binary with the debug flag
debug: build-go
	./perles -d

# Install the binary to $GOPATH/bin with version info
install: build-frontend
	go install -trimpath -ldflags "$(BUILD_LDFLAGS)" .

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
		./internal/ui/embeddedmode/... \
		./internal/ui/modals/help/... \
		./internal/ui/modals/issueeditor/... \
		./internal/ui/nobeads/... \
		./internal/ui/outdated/... \
		./internal/ui/shared/chatpanel/... \
		./internal/ui/shared/colorpicker/... \
		./internal/ui/shared/diffviewer/... \
		./internal/ui/shared/issuebadge/... \
		./internal/ui/shared/logoverlay/... \
		./internal/ui/shared/modal/... \
		./internal/ui/shared/overlay/... \
		./internal/ui/shared/picker/... \
		./internal/ui/shared/table/... \
		./internal/ui/shared/selection/... \
		./internal/ui/shared/toaster/... \
		./internal/ui/shared/vimtextarea/... \
		./internal/ui/styles/... \
		./internal/ui/tree/... \
		./internal/mode/kanban/... \
		./internal/mode/dashboard/... \
		./internal/mode/search/... \
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

docs:
	zensical serve -a 127.0.0.1:8001

index-docs:
	qmd --index perles update
	qmd --index perles embed