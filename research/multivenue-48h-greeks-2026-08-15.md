# Three-Venue 48-Hour Greek Experiment - 2026-08-15

## Scope

This experiment is a deterministic, direct-connect baseline. It creates three
separately prefunded venues (`north`, `central`, `south`) on one simulated
clock. Each venue has `ABC/USD` spot with local auto-borrow spot margin,
`ABC-PERP`, rolling dated futures, European calls/puts, two A-S linear makers,
an option dealer with finite-band delta hedging, and seeded noise/option flow.

It is **not** a cross-venue arbitrage result: no router, transfer, common
collateral, delayed gateway, or atomic multi-leg execution exists in this arm.
The option dealer is the existing linear-inventory-skew baseline, not an A-S
options dealer. The new A-S control is deliberately confined to the linear
spot/perpetual makers.

## Reproduction and integrity

Configuration: [multivenue-expiry-48h.json](multivenue-expiry-48h.json).

```bash
GOMAXPROCS=14 go run ./cmd/multivenue \
  -config=research/multivenue-expiry-48h.json -duration=48h \
  -logdir=/tmp/multivenue-expiry-48h-exact-tenor
go run ./cmd/greekreport \
  -input=/tmp/multivenue-expiry-48h-exact-tenor/greeks.json \
  -output=/tmp/multivenue-expiry-48h-exact-tenor/greek-tenors.json
```

The run took 1m35s wall-clock and produced 836 MB of venue-scoped logs. A
second full run with `GOMAXPROCS=1` took 1m45s. Both emitted the identical
`greeks.json` SHA-256:

```text
b1b3a54ac181c88feaa4e0bdee785eb28409e65bd5c5ec9c54b4fdb7a9d742db
```

The sorted digest of all nine venue JSONL files was identical in both runs:

```text
229bb28e51da9440a60c4fc8a4dcefa94eb3987d1f79607e45a850dc0c04ff2f
```

Each venue recorded 87 listings and 75 option/future settlement lifecycle
events. The report carries raw position rows with venue, listing timestamp,
expiry, strike, call/put, signed position, spot-mid forward proxy, flat IV,
and signed Black-76 delta/gamma/vega.

## Results

Risk is grouped by **remaining** maturity, not the original listing label:
short is `0 < TTE <= 6h`; long is `TTE >= 24h`. This matters because a
48-hour option becomes short-dated as it ages, and a same-expiry option is one
economic contract rather than two independent "short" and "long" books.

| Venue | Peak abs gamma, short | Peak abs gamma, long | Peak abs vega, short | Peak abs vega, long |
| --- | ---: | ---: | ---: | ---: |
| central | 5.902e-08 | 3.537e-09 | 1.065e+08 | 1.940e+08 |
| north | 9.192e-08 | 3.764e-09 | 1.035e+08 | 2.087e+08 |
| south | 8.757e-08 | 2.993e-09 | 1.004e+08 | 1.642e+08 |

Thus the live short bucket had roughly 17x, 24x, and 29x the long bucket's
peak gamma across the three venues. The live long bucket had 1.6x to 2.0x the
short bucket's peak vega. This is the expected direction for the fixed
Black-76/flat-IV model and supports H-013 **only as a model-conditioned
sensitivity result**.

Aggregate dealer delta was controlled but not identically zero because the
report is taken after quoting and before same-phase hedge fills. Its maximum
absolute net delta was `0.232`, `0.243`, and `0.159` contracts for central,
north, and south; mean absolute net delta was approximately `0.0065`. Final
net delta was `+0.0163`, `-0.00411`, and `+0.000624` respectively. Final
aggregate gamma remained negative on each venue, consistent with the option
buy-flow arm leaving the dealer short convexity.

The final periodic observation occurs 121 seconds before the last 48-hour
contract expiry. This is an actor-owned pre-settlement observation, not an
exchange-owned terminal risk row. The report is sufficient to show the trend
and preserve every observed exposure, but not to claim the exact risk at the
settlement instant.

## Defects found before acceptance

The first long-run artifacts were rejected rather than interpreted:

1. Listing code rounded `now + tenor` down to a Unix-epoch grid. A configured
   48-hour option could therefore be issued with about 24 hours remaining.
2. Replacing that with upward grid rounding avoided shortened contracts but
   produced 12-hour, 72-hour, and 96-hour maturities for nominal 6/48-hour
   settings.

`DatedFuturesLister` and `OptionChainLister` now retain a per-tenor rolling
expiry and create each new generation at exactly `listing_time + tenor`.
Regression tests cover exact futures/options tenor, successor generation, and
timestamp overflow. The accepted run above was performed only after that fix.

## What this does not establish

- Vega is local sensitivity only. IV is static, so realised vega PnL is zero
  by construction.
- Black-76 receives the venue spot midpoint as a zero-carry forward proxy.
  A maturity-matched forward curve and dynamic surface are not yet modeled.
- The retained hedge is the dealer's hedge-trade inventory, not a complete
  account-level delta allocation by expiry.
- One seed replicated across runtime parallelism tests causality/reproducibility,
  not statistical robustness. The next acceptance gate is at least three
  common-random-number seeds per arm.
- No latency or cross-venue execution outcome is represented in this baseline.

## Next discriminants

1. Add deterministic exchange-owned `post_mark`, `pre_expiry`, and
   `post_settlement` risk rows so the final short-dated gamma state survives
   delisting.
2. Add a dynamic IV/surface process and a forward provider keyed by expiry;
   then decompose realised PnL into delta/gamma/vega/theta residuals.
3. Compare common-random-number linear-skew and A-S linear makers on inventory
   tails, fill distance, quote survival, markouts, and marked PnL.
4. Add a venue-qualified cross-venue router with per-leg fills and prefunded
   accounts before testing fragmentation or arbitrage claims.
