# Arbitrage Actors

Both actors are `SubActor` implementations for use inside `CompositeActor`.

---

## InternalFundingArb

Cash-and-carry arb between a spot market and a perp. Holds a delta-neutral position
(long spot / short perp in contango; reverse in backwardation) and collects funding.

```go
arb := actors.NewInternalFundingArb(actors.InternalFundingArbConfig{
    ActorID:    id,
    SpotSymbol: "BCD/USD",
    PerpSymbol: "BCD-PERP",
    SpotInstrument: spotInst,
    PerpInstrument: perpInst,

    // Entry triggers (OR logic: either condition opens the position)
    BasisThresholdBps: 20,   // enter when (perpMid - spotMid)/spotMid > 20bps
    MinFundingRate:    15,   // enter when funding rate > 15bps

    // Exit triggers (OR logic: either condition closes the position)
    ExitBasisThresholdBps: 5,
    ExitFundingRate:       5,

    MaxPositionSize: 10 * BCD_PRECISION,
})
```

**Modes:**
- `ModeContango` (positive basis/funding): buy spot, sell perp
- `ModeBackwardation` (negative basis/funding): sell spot, buy perp

**Balance guards:**
- Contango: checks `CanReserveQuote(notional)` before buying spot
- Backwardation: checks `GetBaseBalance(base) ≥ MaxPositionSize` before selling spot

**Funding settlement:** On each `EventFundingUpdate`, if the funding timestamp advanced,
the actor settles funding P&L based on the perp position size and current funding rate.

**Exit condition is OR:** The position closes when *either* basis *or* funding rate
reverts past the exit threshold. Do not use AND — it traps positions indefinitely when
the signal that triggered entry normalizes while the other stays flat.

---

## TriangleArbitrage

Three-leg circular arb across a base pair, a cross pair, and a direct pair.

**Circuit (forward):** USD → ABC → BCD → USD
- Leg 1 (`DirectSymbol`): buy ABC/USD
- Leg 2 (`CrossSymbol`): buy BCD/ABC
- Leg 3 (`BaseSymbol`): sell BCD/USD

**Circuit (reverse):** USD → BCD → ABC → USD (opposite sides on all legs)

```go
arb := actors.NewTriangleArbitrage(actors.TriangleArbConfig{
    ActorID:          id,
    BaseSymbol:       "BCD/USD",
    CrossSymbol:      "BCD/ABC",
    DirectSymbol:     "ABC/USD",
    BaseInstrument:   bcdUsdInst,
    CrossInstrument:  bcdAbcInst,
    DirectInstrument: abcUsdInst,
    ThresholdBps:     5,  // minimum profit above fees before executing
    MaxTradeSize:     5 * BCD_PRECISION,
    TakerFeeBps:      3,  // total across all 3 legs
})
```

**Opportunity detection:** Checked on every book update when not already executing.
Uses float64 arithmetic to avoid int64 overflow at large price×quantity products.

**Execution:** All three market orders submitted simultaneously. `executing=true` blocks
re-entry until all three fills confirm. Any rejection resets state for retry.
