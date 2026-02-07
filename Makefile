.PHONY: help test test-verbose coverage coverage-html build clean all install-tools

# Default target
.DEFAULT_GOAL := help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary output directory
BIN_DIR=bin

# Binary names and paths
BINARIES=multisim sim latency_arb
MULTISIM_BINARY=$(BIN_DIR)/multisim
SIM_BINARY=$(BIN_DIR)/sim
LATENCY_ARB_BINARY=$(BIN_DIR)/latency_arb

# Coverage output
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

## help: Display this help message
help:
	@echo "Exchange Simulation - Build & Test Targets"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | grep -v '^##@' | sed 's/^## /  /' | column -t -s ':'

##@ Building

## build: Build all binaries
build: $(MULTISIM_BINARY) $(SIM_BINARY) $(LATENCY_ARB_BINARY)
	@echo "✓ All binaries built successfully"

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

$(MULTISIM_BINARY): $(BIN_DIR)
	@echo "Building multisim..."
	@$(GOBUILD) -o $(MULTISIM_BINARY) ./cmd/multisim

$(SIM_BINARY): $(BIN_DIR)
	@echo "Building sim..."
	@$(GOBUILD) -o $(SIM_BINARY) ./cmd/sim

$(LATENCY_ARB_BINARY): $(BIN_DIR)
	@echo "Building latency_arb..."
	@$(GOBUILD) -o $(LATENCY_ARB_BINARY) ./cmd/latency_arb

## rebuild: Clean and rebuild all binaries
rebuild: clean build

##@ Testing

## test: Run all tests
test:
	@echo "Running tests..."
	@$(GOTEST) -v ./... | grep -E "^(PASS|FAIL|ok|---|\s+[a-z_]+\.go)"

## test-short: Run tests without long-running tests
test-short:
	@echo "Running short tests..."
	@$(GOTEST) -short ./...

## test-verbose: Run all tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	@$(GOTEST) -v ./...

## test-race: Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	@$(GOTEST) -race ./...

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	@$(GOTEST) -bench=. -benchmem ./...

##@ Coverage

## coverage: Generate test coverage report
coverage:
	@echo "Generating coverage report..."
	@$(GOTEST) -coverprofile=$(COVERAGE_FILE) ./...
	@$(GOCMD) tool cover -func=$(COVERAGE_FILE)
	@echo ""
	@echo "Coverage report saved to $(COVERAGE_FILE)"
	@echo "Run 'make coverage-html' to view in browser"

## coverage-html: Generate and open HTML coverage report
coverage-html: coverage
	@echo "Generating HTML coverage report..."
	@$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Opening coverage report in browser..."
	@if command -v xdg-open > /dev/null; then \
		xdg-open $(COVERAGE_HTML); \
	elif command -v open > /dev/null; then \
		open $(COVERAGE_HTML); \
	else \
		echo "Please open $(COVERAGE_HTML) in your browser"; \
	fi

##@ Maintenance

## clean: Remove built binaries and coverage files
clean:
	@echo "Cleaning..."
	@$(GOCLEAN)
	@rm -rf $(BIN_DIR)
	@rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@echo "✓ Clean complete"

## tidy: Tidy go modules
tidy:
	@echo "Tidying go modules..."
	@$(GOMOD) tidy
	@echo "✓ Modules tidied"

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	@$(GOCMD) fmt ./...
	@echo "✓ Code formatted"

## vet: Run go vet
vet:
	@echo "Running go vet..."
	@$(GOCMD) vet ./...
	@echo "✓ Vet complete"

## lint: Run golangci-lint (requires installation)
lint:
	@if command -v golangci-lint > /dev/null; then \
		echo "Running golangci-lint..."; \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run 'make install-tools' to install."; \
		exit 1; \
	fi

##@ Development

## install-tools: Install development tools
install-tools:
	@echo "Installing development tools..."
	@command -v golangci-lint > /dev/null || \
		(echo "Installing golangci-lint..." && \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin)
	@echo "✓ Tools installed"

## run-multisim: Build and run multisim
run-multisim: $(MULTISIM_BINARY)
	@echo "Running multisim..."
	@./$(MULTISIM_BINARY)

## run-sim: Build and run sim
run-sim: $(SIM_BINARY)
	@echo "Running sim..."
	@./$(SIM_BINARY)

## all: Run fmt, vet, test, and build
all: fmt vet test build
	@echo "✓ All checks passed and binaries built"

## ci: Continuous integration - format check, vet, test, build
ci: fmt vet test-race build
	@echo "✓ CI pipeline complete"
