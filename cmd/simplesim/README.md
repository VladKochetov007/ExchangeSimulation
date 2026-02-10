# Simple Exchange Simulation

A minimal exchange simulation demonstrating basic market microstructure with liquidity providers, market makers, and a randomized taker.

## Configuration

- **Symbol**: BTC/USD spot
- **Exchange**: Single exchange
- **Duration**: 60 seconds wallclock (~10 minutes simulated at 10x speedup)
- **Actors**:
  - 1x First Liquidity Provider (50 bps spread, bootstrap price $50,000)
  - 4x Pure Market Makers (5, 10, 20, 50 bps spreads, bootstrap price $50,000)
  - 1x Randomized Taker (alternating buy/sell every 500ms, 0.01-0.1 BTC per trade)

## Running

```bash
make run-simplesim
```

Or build and run manually:

```bash
make build
./bin/simplesim
```

## Analysis

After running the simulation, analyze the results with Python:

```bash
source .venv/bin/activate

# Simple analysis
python3 scripts/analyze_simple.py

# Detailed analysis with multiple plots
python3 scripts/analyze_detailed.py
```

This generates:
- `logs/trades_analysis.png` - Basic trade analysis
- `logs/detailed_analysis.png` - Comprehensive multi-panel analysis

## Results

Example from a 60-second run:

- **Simulated time**: 9.4 minutes
- **Total trades**: 203
- **Volume**: 9.38 BTC
- **Price range**: $49-$50 (stable around bootstrap)
- **Average spread**: $1.00
- **Actors**: All 6 actors active
  - LP: 6.14 BTC volume, 59 maker fills
  - MM 5bps: 2.10 BTC volume, 51 maker fills
  - MM 10bps: 1.00 BTC volume, 20 maker fills
  - MM 20bps: 1.92 BTC volume, 44 maker fills
  - MM 50bps: 1.24 BTC volume, 29 maker fills
  - Taker: 6.36 BTC volume, 0 maker fills (pure taker)

## Key Features Demonstrated

1. **Liquidity bootstrapping**: FirstLP provides initial liquidity at bootstrap price
2. **Multi-tier market making**: MMs with different spreads create depth
3. **Price discovery**: Spread competition between MMs at different levels
4. **Inventory management**: MMs and LP manage inventory risk
5. **Order flow**: Randomized taker creates realistic trade patterns
6. **Event logging**: Full JSONL logs for analysis

## Logs

All events are logged to `logs/simulation.log` in JSONL format with fields:
- `sim_time`: Simulated timestamp (nanoseconds)
- `server_time`: Wall clock timestamp
- `event`: Event type (Trade, OrderFill, OrderAccepted, etc.)
- `client_id`: Actor ID
- Additional event-specific fields

## Implementation Notes

- Uses simulated clock for deterministic replay
- 10x speedup (10ms sim time per 1ms real time)
- Integer arithmetic throughout
- No fees for simplicity (can be enabled with fee plans)
- Market makers use bootstrap price to quote on empty book
- Taker alternates between buy/sell to balance order flow
