# Liquidity Visualization Plots

Generated from the simple exchange simulation logs. All plots show dynamics of order book liquidity, best bid/offer, spread, and depth evolution over time.

## Available Plots

### 1. `logs/liquidity_analysis.png` - Comprehensive Liquidity Analysis
**6-panel detailed view:**
- **Panel 1**: Best Bid/Offer and trades over time with mid-price
- **Panel 2**: Bid-ask spread (USD and bps)
- **Panel 3**: Liquidity at BBO (quantity on best bid/ask)
- **Panel 4**: Order book snapshots at 6 different times
- **Panel 5**: Aggregated liquidity over time
- **Panel 6**: (see detailed_analysis.png for additional panels)

**Statistics:**
- Average spread: $1.00 (202 bps)
- Bid side average: 1.64 BTC at best
- Ask side average: 20.22 BTC at best
- Book update frequency: 0.46 updates/sec

**Generate:**
```bash
source .venv/bin/activate
python3 scripts/analyze_liquidity.py
```

---

### 2. `market_depth_timeline.png` - Market Depth Timeline
**Comprehensive L3 reconstruction showing:**
- Best bid/ask over time with spread shading
- Mid-price evolution
- Spread dynamics (absolute and bps)
- Liquidity at BBO over time
- Book depth levels (bid/ask count)
- Multiple depth snapshots across timeline

**Features:**
- Uses full L3 order book reconstruction
- Shows 20 evenly-spaced snapshots
- Tracks all order updates (accepted, filled, cancelled)

**Generate:**
```bash
source .venv/bin/activate
python3 scripts/plot_market_depth_timeline.py logs/simulation.log
```

---

### 3. `depth_plot.png` - Order Book Depth Chart
**Classic depth chart showing:**
- Cumulative volume at each price level
- Bid side (green) - cumulative from best bid downward
- Ask side (red) - cumulative from best ask upward
- Snapshot at a specific time (random or specified)

**Generate:**
```bash
source .venv/bin/activate
# Random time:
python3 scripts/plot_depth.py logs/simulation.log

# Specific time (nanoseconds):
python3 scripts/plot_depth.py logs/simulation.log 1770712500000000000
```

---

### 4. `bbo_plot.png` - Best Bid/Offer Over Time
**Simple BBO tracking:**
- Best bid price (green line)
- Best ask price (red line)
- Spread shading between bid and ask
- Clean, focused view of top-of-book

**Generate:**
```bash
source .venv/bin/activate
python3 scripts/plot_book.py logs/simulation.log
```

---

### 5. `logs/detailed_analysis.png` - Full Simulation Analysis
**8-panel comprehensive view:**
- Trade prices over time (color-coded by side)
- Cumulative volume
- Trade interval distribution
- Actor inventory tracking
- Volume by actor
- Best bid/ask with spread
- Maker vs taker fills by actor

**Generate:**
```bash
source .venv/bin/activate
python3 scripts/analyze_detailed.py
```

---

## Key Findings from Liquidity Analysis

### Spread Characteristics
- **Constant at $1.00** (202 bps at $49.50 mid)
- Reflects tight MM spreads (5, 10 bps) at $49 and $50
- No spread widening under flow (stable liquidity)

### Depth Asymmetry
- **Ask side**: 20.22 BTC average (LP selling inventory)
- **Bid side**: 1.64 BTC average (LP constrained by USD balance)
- Ratio: 12:1 ask-heavy
- Reflects LP's initial position (20 BTC, $1M USD)

### Update Frequency
- 0.46 book updates per second
- 261 total updates over 9.4 minutes
- Indicates moderate requoting activity
- MMs adjust quotes on fills and periodic checks

### Liquidity Provision
- **5 liquidity providers active:**
  - 1x FirstLP (50 bps spread)
  - 4x PureMarketMakers (5, 10, 20, 50 bps)
- **Multi-tier depth:**
  - Tightest at $49/$50 (5-10 bps MMs)
  - Wider levels from 20-50 bps MMs
  - LP provides backstop liquidity

### Market Quality
- ✅ Continuous two-sided quotes
- ✅ No empty book periods
- ✅ Stable spread (no widening)
- ✅ Deep liquidity on both sides
- ✅ Efficient price discovery

---

## Quick Reference

| Plot | Focus | Best For |
|------|-------|----------|
| `liquidity_analysis.png` | BBO dynamics, spread, liquidity | Understanding spread behavior and liquidity at top |
| `market_depth_timeline.png` | Full timeline, L3 reconstruction | Comprehensive depth evolution over time |
| `depth_plot.png` | Cumulative depth chart | Classic market depth visualization at moment |
| `bbo_plot.png` | Simple BBO tracking | Clean view of top-of-book movement |
| `detailed_analysis.png` | Full simulation stats | Overall trading activity and actor behavior |

---

## Generating All Plots

```bash
#!/bin/bash
source .venv/bin/activate

echo "Generating liquidity analysis..."
python3 scripts/analyze_liquidity.py

echo "Generating detailed analysis..."
python3 scripts/analyze_detailed.py

echo "Generating market depth timeline..."
python3 scripts/plot_market_depth_timeline.py logs/simulation.log

echo "Generating order book depth..."
python3 scripts/plot_depth.py logs/simulation.log

echo "Generating BBO plot..."
python3 scripts/plot_book.py logs/simulation.log

echo "All plots generated!"
ls -lh *.png logs/*.png
```
