# gsc-indexer Makefile
# Common development tasks

.PHONY: help test test-race test-cover lint build install release clean fmt vet staticcheck gosec deps tools

# Default target
help:
	@echo "gsc-indexer - Make targets:"
	@echo ""
	@echo "  test          Run tests with race detector"
	@echo "  test-race     Run tests with race detector (alias)"
	@echo "  test-cover    Run tests with coverage report"
	@echo "  lint          Run golangci-lint"
	@echo "  staticcheck   Run staticcheck"
	@echo "  gosec         Run gosec security scanner"
	@echo "  vet           Run go vet"
	@echo "  fmt           Format code with gofmt/goimports"
	@echo "  build         Build binary to ./gsc-indexer"
	@echo "  install       Install binary to $$GOBIN (default ~/go/bin)"
	@echo "  release       Create a release using goreleaser (requires tag)"
	@echo "  release-snap  Create a snapshot release (no tag)"
	@echo "  clean         Remove build artifacts"
	@echo "  deps          Download and tidy dependencies"
	@echo "  tools         Install development tools"
	@echo ""

# Run all tests with race detector
test: test-race

test-race:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

# Run tests with coverage and open HTML report
test-cover:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run golangci-lint
lint:
	golangci-lint run --timeout=5m

# Run staticcheck
staticcheck:
	staticcheck ./...

# Run gosec security scanner
gosec:
	gosec -quiet ./...

# Run go vet
vet:
	go vet ./...

# Format code
fmt:
	gofmt -w .
	goimports -w .

# Build binary
build:
	go build -v -o gsc-indexer .

# Install to GOBIN
install:
	go install -v .

# Create release (requires git tag)
release:
	goreleaser release --clean

# Create snapshot release (no tag, for testing)
release-snap:
	goreleaser release --snapshot --clean

# Clean build artifacts
clean:
	rm -f gsc-indexer
	rm -f coverage.out coverage.html
	rm -rf dist/

# Download and tidy dependencies
deps:
	go mod download
	go mod tidy

# Install development tools
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/goreleaser/goreleaser@latest

# Run all checks locally before push
check: fmt vet staticcheck gosec lint test

# Verify CI would pass locally
ci-local: deps check build
	@echo "All CI checks passed!"