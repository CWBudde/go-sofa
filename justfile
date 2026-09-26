# go-sofa justfile
# Development automation for SOFA (Spatially Oriented Format for Acoustics)

set shell := ["bash", "-uc"]

# Default recipe - show available commands
default:
    @just --list

# Note: Install dependencies manually or use the GitHub Actions workflow
# treefmt: Download from https://github.com/numtide/treefmt/releases
# Go tools (versions pinned as in CI): go install mvdan.cc/gofumpt@v0.12.0 && go install github.com/daixiang0/gci@v0.14.0 && go install mvdan.cc/sh/v3/cmd/shfmt@v3.14.1
# golangci-lint: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
# prettier: npm install -g prettier@3
# shellcheck: brew install shellcheck / apt-get install shellcheck

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

# Download third-party reference SOFA files into testdata/ (see testdata/PROVENANCE.md)
fetch-testdata:
    ./scripts/fetch-testdata.sh

# Run all tests (needs `just fetch-testdata` once)
test:
    go test -race -v -timeout 300s ./...

# Write one file per DataType with Save and read it back with h5py and netCDF4
# (needs Python with h5py and netCDF4: pip install h5py netCDF4)
interop DIR="":
    #!/usr/bin/env bash
    set -euo pipefail
    dir="{{ DIR }}"
    if [[ -z "$dir" ]]; then dir="$(mktemp -d)"; fi
    go run ./internal/interop/gen "$dir"
    python3 scripts/interop_check.py "$dir"

# Fail when the root package's statement coverage is below MIN percent
coverage-check MIN="85":
    #!/usr/bin/env bash
    set -euo pipefail
    profile="$(mktemp)"
    trap 'rm -f "$profile"' EXIT
    go test -timeout 300s -coverprofile="$profile" . >/dev/null
    total="$(go tool cover -func="$profile" | awk '/^total:/ { sub("%", "", $3); print $3 }')"
    echo "root package coverage: ${total}% (floor {{ MIN }}%)"
    awk -v t="$total" -v m="{{ MIN }}" 'BEGIN { exit !(t >= m) }'

# Run tests with coverage
test-coverage:
    go test -v -timeout 120s -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

# Run all checks (formatting, linting, tests, tidiness)
check: check-formatted lint test check-tidy

# Command-line tools under cmd/
tools := "sofainfo sofa2json sofaprobe"

# Build all CLI tools into bin/
build:
    for t in {{ tools }}; do go build -o "bin/$t" "./cmd/$t" || exit; done

# Install all CLI tools to $GOPATH/bin
install:
    for t in {{ tools }}; do go install "./cmd/$t" || exit; done

# Clean build artifacts
clean:
    rm -rf bin/
    rm -f coverage.out coverage.html
    rm -f sofainfo sofa2json sofaprobe

# Run sofaprobe on sample files
test-sample FILE="testdata/tester.sofa":
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
