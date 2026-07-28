# Realism Roadmap — synthesis of the July 2026 deep-research pass

Synthesis of three parallel research reports (full detail and citations in
each):

- [deepdive-engine-internals.md](deepdive-engine-internals.md) — matching
  engine architecture vs production (LMAX/sequencer model, ITCH/OUCH, order
  book structures, thread-safety)
- [deepdive-derivatives-mechanics.md](deepdive-derivatives-mechanics.md) —
  mark/index construction, funding, liquidation waterfalls, margin systems vs
  Binance/BitMEX/Deribit/OKX
- [deepdive-sim-realism.md](deepdive-sim-realism.md) — ABIDES,
  queue-reactive/Hawkes models, Cont 2001 + "Get Real" validation checklists,
  impact laws

Ranked by (correctness first, then realism gain per line of code). Every item
is shaped as an injectable interface per the library-first rule.

## Tier 0 — correctness holes (exchange logic, fix like bugs)

1. **Options are invisible to the risk engine.** `marginCore()` resolves nil
   for `*EuropeanOption`, so option maintenance margin is 0 and option
   mark-to-market is excluded from cross equity: a short-vol account is
   unliquidatable and premium selling is unlimited. Cheap fix first (wire
   `MMBps` + option MTM into `buildAccountMarginProfile`), portfolio margin
   later (Tier 2). *(derivs rec 5a)*
2. **Insurance fund is write-only-negative** (`exchange.go` sole write is
   `-= debt`): starts at 0, decreases forever, credits nothing. Minimum fix:
   credit liquidation surplus; full fix is the Tier-2 liquidation policy.
3. **Blocking gateway sends inside the exchange write lock** (`sendResponse`
   retry loop under `e.mu` in settlement/STP/cancel-all/liquidation paths):
   one slow consumer stalls every book and client; becomes a deadlock the
   moment any consumer calls back into the exchange. Move notification
   delivery outside the lock (drain queue). *(engine E1/R1)*
4. **Non-deterministic event stream from map iteration** in `publishLevels`,
   `cancelOwnCrossingQuotes`, `CancelAllClientOrders`: runs are not
   replayable, undermining the fuzz suites (a failure at seed N may not
   reproduce). Sort or use ordered containers at the publish boundary.
   *(engine E2/R2)*

## Tier 1 — highest realism gain per line

5. **Index-anchored mark price by default.** Today the default mark is the
   perp's own book mid, so liquidations trade into the book that sets the
   mark (self-feeding cascade), and `indexPrice` falls back to mark, making
   basis identically 0 in single-venue configs. Default to ClampedEMA when an
   index is configured; kill the mark-as-index fallback; add the Binance
   median(P1, P2, last) calculator. *(derivs rec 1)*
6. **Metaorder/order-splitting executor agent + impact measurement.** Parent
   size from a power law, child orders on a schedule; measure order-sign ACF,
   response function R(l), impact-vs-participation exponent in
   `cmd/loganalyzer`. Jointly unlocks long-memory sign ACF and the
   square-root impact law — the two facts the current ecology cannot produce.
   Measurement ships first or the rest is unfalsifiable. *(sim R1+R2)*
7. **Heterogeneous background agent population.** N ~ 50-500 takers with
   per-agent PRNGs, exponential/lognormal inter-arrivals (not fixed tickers),
   power-law sizes; plus background limit-order flow with power-law placement
   depth and power-law lifetimes (optionally queue-reactive intensities).
   Unlocks inter-arrival realism, cancel-to-trade ratio, emergent book shape.
   *(sim R3+R4)*
8. **Stochastic fundamental via `FundamentalProvider` interface** (OU/jump
   default, historical-replay option), noisy per-agent observation. Creates
   real adverse selection — without it MM spread levels and every fee/rebate
   experiment are arbitrary. *(sim R5)*

## Tier 2 — mechanism fidelity (derivatives)

9. **LiquidationPolicy interface**: bankruptcy-price takeover, clearance fee,
   insurance fund credited on surplus, partial liquidation to a target ratio
   (not 100% dump), ADLPolicy (BitMEX rank and the Campbell/Hey/Moallemi/Nutz
   water-filling variant — the latter gives an exact PnL conservation
   invariant to fuzz against). *(derivs recs 2+3)*
10. **MaintenanceMarginModel interface** with notional brackets (flat 500bps
    today). Changes cascade shape from few-huge to many-small. *(derivs 3)*
11. **Funding realism**: impact-depth premium sampler, time-weighted average
    over the interval, interest-clamp formula, BaseRate 10bp → 1bp. Current
    point-sample funding is manipulable by one print. *(derivs 4)*
12. **Options portfolio margin** (Deribit PME shape / OKX minimum charge) and
    a real IV surface — after Tier-0 item 1 closes the hole.

## Tier 3 — infrastructure that pays for itself

13. **Event journal + replay harness** (engine R3): turns a 40-minute fuzz
    failure into a replayable file. Highest debugging leverage per effort.
14. **Per-instrument MD sequencing + gap/resync protocol** (engine R4; matches
    the gen-3 deferred finding): per-symbol RptSeq, explicit gap signal,
    snapshot re-request path.
15. **Single-writer engine core** (engine R5) — structural; only after 13/14.
16. **Queue-position-aware heterogeneous MMs, tick-regime experiments,
    survivorship, LOB-Bench scoring, mild 24/7 seasonality** (sim R7-R11).

**Explicitly not recommended**: deep generative LOB models (TRADES/LOBGAN) —
non-interactive, need message-level real data; borrow their evaluation only.

## Validation gate (from the sim-realism checklist)

Realism claims require out-of-sample facts no parameter was tuned against:
impact exponent ~0.5, sign-ACF decay ~ tau^-0.6, |r| ACF decay exponent
0.2-0.4, order-size tail 1+mu ~ 2.3-2.7, cancel-to-trade ratio, lifetime
exponents. Fano/kurtosis alone are fitted, not evidence.
