# Build & Development Guide

This document describes how to build, test, and develop the exchange simulation project.

## Quick Start

```bash
# Build all binaries
make build

# Run tests
make test

# Generate coverage report and view in browser
make coverage-html

# Build and run the main simulation
make run-multisim
```

## Prerequisites

- **Go 1.21+** - The project uses modern Go features
- **Python 3.8+** - For visualization scripts (optional)
- **Make** - Build automation tool

## Building

### Build All Binaries

```bash
make build
```

This creates three binaries in `bin/`:
- `bin/multisim` - Multi-exchange simulation runner
- `bin/sim` - Basic simulation runner
- `bin/latency_arb` - Latency arbitrage simulation

### Build Specific Binary

```bash
go build -o bin/multisim ./cmd/multisim
go build -o bin/sim ./cmd/sim
go build -o bin/latency_arb ./cmd/latency_arb
```

### Rebuild Everything

```bash
make rebuild  # Clean and rebuild
```

## Testing

### Run All Tests

```bash
make test         # Concise output
make test-verbose # Full verbose output
```

### Run Specific Package Tests

```bash
go test ./exchange -v
go test ./actor -v
go test ./simulation -v
```

### Run With Race Detector

```bash
make test-race
```

This is important for catching concurrency bugs since the simulation is heavily concurrent.

### Run Short Tests Only

```bash
make test-short
```

Skips long-running integration tests.

## Code Coverage

### Generate Coverage Report

```bash
make coverage
```

This runs all tests with coverage tracking and displays a summary.

### View HTML Coverage Report

```bash
make coverage-html
```

This generates an HTML coverage report and automatically opens it in your default browser.

The coverage report shows:
- Which lines of code are covered by tests
- Coverage percentage by package and file
- Uncovered code blocks highlighted in red

### Manual Coverage Commands

```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View summary
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html
```

## Code Quality

### Format Code

```bash
make fmt
```

Runs `go fmt` on all packages to ensure consistent formatting.

### Run Static Analysis

```bash
make vet
```

Runs `go vet` to catch common mistakes.

### Run Linter (Optional)

```bash
make install-tools  # Install golangci-lint
make lint
```

Runs comprehensive linting checks for code quality.

## Running Simulations

### Multi-Exchange Simulation

```bash
make run-multisim
# Or directly:
./bin/multisim
```

This runs a 10-second simulation with multiple exchanges, symbols, and actors.

### Basic Simulation

```bash
make run-sim
./bin/sim
```

### Latency Arbitrage Simulation

```bash
./bin/latency_arb
```

## Development Workflow

### Complete Development Cycle

```bash
make all
```

This runs in sequence:
1. Format code
2. Run static analysis
3. Run tests
4. Build binaries

### Continuous Integration

```bash
make ci
```

CI-friendly target that includes race detection.

## Cleaning

### Remove Built Artifacts

```bash
make clean
```

This removes:
- `bin/` directory and all binaries
- Coverage files (`coverage.out`, `coverage.html`)
- Go build cache

### Tidy Go Modules

```bash
make tidy
```

Cleans up `go.mod` and `go.sum` files.

## Makefile Targets Reference

Run `make help` to see all available targets:

```
Building:
  build              Build all binaries
  rebuild            Clean and rebuild all binaries

Testing:
  test               Run all tests
  test-short         Run tests without long-running tests
  test-verbose       Run all tests with verbose output
  test-race          Run tests with race detector
  bench              Run benchmarks

Coverage:
  coverage           Generate test coverage report
  coverage-html      Generate and open HTML coverage report

Maintenance:
  clean              Remove built binaries and coverage files
  tidy               Tidy go modules
  fmt                Format Go code
  vet                Run go vet
  lint               Run golangci-lint

Development:
  install-tools      Install development tools
  run-multisim       Build and run multisim
  run-sim            Build and run sim
  all                Run fmt, vet, test, and build
  ci                 Continuous integration pipeline
```

## Python Visualization (Optional)

The project includes Python scripts for analyzing and visualizing simulation results:

### Setup Python Environment

```bash
python3 -m venv .venv
source .venv/bin/activate  # On Windows: .venv\Scripts\activate
pip install -r requirements.txt  # If exists
```

### Analyze Logs

```bash
# Analyze order book health
python3 scripts/analyze_book_health.py logs/binance/spot/BTCUSD.log 15

# Plot market depth timeline
python3 scripts/plot_market_depth_timeline.py logs/binance/spot/BTCUSD.log
```

## Troubleshooting

### Tests Failing

1. **Clear build cache**: `go clean -cache`
2. **Update dependencies**: `make tidy`
3. **Check for panics**: `make test-verbose`

### Build Failures

1. **Check Go version**: `go version` (need 1.21+)
2. **Clean and rebuild**: `make rebuild`
3. **Check module integrity**: `go mod verify`

### Race Conditions

If tests fail intermittently:

```bash
make test-race
```

This enables the race detector which will catch data races.

### Coverage Too Low

To identify untested code:

```bash
make coverage-html
```

Then browse the HTML report to see which functions lack test coverage.

## CI/CD Integration

### GitHub Actions Example

```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: make ci
```

### Local Pre-Commit Hook

Create `.git/hooks/pre-commit`:

```bash
#!/bin/bash
make fmt vet test-short
```

Then: `chmod +x .git/hooks/pre-commit`

## Performance

### Benchmarking

```bash
make bench
```

### Profiling

```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=. ./...
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof -bench=. ./...
go tool pprof mem.prof
```

## Best Practices

1. **Always run tests before committing**: `make test`
2. **Keep coverage above 70%**: Check with `make coverage`
3. **Format code**: `make fmt` before committing
4. **Run race detector on changes to concurrent code**: `make test-race`
5. **Update BUILD.md when adding new build targets**
