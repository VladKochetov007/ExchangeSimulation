# Quick Start Guide

## Build & Run (First Time)

```bash
# Build everything
make build

# Run the simulation
./bin/multisim

# Check logs
ls logs/
```

## Essential Commands

```bash
make build          # Build all binaries
make test           # Run tests
make coverage-html  # View test coverage
make clean          # Clean build artifacts
make help           # Show all available commands
```

## Daily Development

```bash
# Before making changes
make test

# After making changes
make fmt            # Format code
make test           # Run tests
make vet            # Static analysis

# Or run everything at once
make all
```

## Viewing Results

### Test Coverage
```bash
make coverage-html
# Opens coverage report in your browser
```

### Simulation Logs
```bash
# Logs are in logs/[exchange]/[type]/[symbol].log
cat logs/binance/spot/BTCUSD.log | jq

# Python analysis (if .venv is set up)
python3 scripts/analyze_book_health.py logs/binance/spot/BTCUSD.log 15
```

## Troubleshooting

```bash
make clean          # Clean everything
make rebuild        # Clean and rebuild
make test-verbose   # Detailed test output
```

## Complete Documentation

- [BUILD.md](BUILD.md) - Comprehensive build and test documentation
- [CLAUDE.md](CLAUDE.md) - Development guidelines and project structure
- Run `make help` - See all available Makefile targets
