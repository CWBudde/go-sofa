# go-sofa justfile
# Development automation for SOFA (Spatially Oriented Format for Acoustics)

set shell := ["bash", "-uc"]

export GOPRIVATE := "github.com/MeKo-Christian"

# Default recipe - show available commands
default:
    @just --list

# Note: Install dependencies manually or use the GitHub Actions workflow
# treefmt: Download from https://github.com/numtide/treefmt/releases
# Go tools: go install mvdan.cc/gofumpt@latest && go install github.com/daixiang0/gci@latest && go install mvdan.cc/sh/v3/cmd/shfmt@latest
# golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
# prettier: npm install -g prettier

# Format all code using treefmt
fmt:
    treefmt --allow-missing-formatter

# Check if code is formatted correctly
check-formatted:
    treefmt --allow-missing-formatter --fail-on-change

# Run linters
lint:
    golangci-lint run --timeout=2m

# Run linters with auto-fix
lint-fix:
    golangci-lint run --fix --timeout=2m

# Ensure go.mod is tidy
check-tidy:
    go mod tidy
    git diff --exit-code go.mod go.sum

# Run all tests
test:
    go test -v -timeout 120s ./...

# Run tests with coverage
test-coverage:
    go test -v -timeout 120s -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

# Run all checks (formatting, linting, tests, tidiness)
check: check-formatted lint test check-tidy

# Build sofaprobe CLI tool
build-sofaprobe:
    go build -o bin/sofaprobe ./cmd/sofaprobe

# Build all CLI tools
build: build-sofaprobe

# Install sofaprobe to $GOPATH/bin
install-sofaprobe:
    go install ./cmd/sofaprobe

# Install all CLI tools
install: install-sofaprobe

# Clean build artifacts
clean:
    rm -rf bin/
    rm -f coverage.out coverage.html
    rm -f sofaprobe

# Run sofaprobe on sample files
test-sample FILE="testdata/sample.sofa":
    go run ./cmd/sofaprobe "{{ FILE }}"

# Show version information
version:
    go version

# Run go mod download
deps:
    go mod download

# Verify dependencies
verify:
    go mod verify

fix:
    just lint-fix
    just fmt
