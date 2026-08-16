# Basic-actor market, visualised

Source: `logs/demo_basic`, 24 simulated hours, three venues, three assets, seed 91.
Population is deliberately minimal: Stoikov spot makers, a perpetual maker, dated
futures makers, option dealers, noise takers, option flow, and the arbitrage
desks (spot-perp carry, dated carry, put-call parity, triangular).

Nothing in the runtime knows a price. Every series below is reconstructed from
trades the participants actually produced.

## 01 — price and venue dispersion

The ABC/USD path is an emergent random walk: no process generates it, it is the
residue of order flow against inventory-managing makers. The lower panel shows
the three venues agreeing to within a narrow band, which is what the cross-venue
and triangular desks enforce.

## 02 — spot against perpetual and dated futures

**This plot does not show what it was supposed to show, and that is the finding.**

The perpetual drifts to about -275 bps against spot over 24 hours rather than
oscillating near zero, and the dated futures diverge steadily to +4,000 bps
instead of converging into settlement. Two dated contracts show the same shape,
each starting at zero when listed and walking away monotonically.

Expected behaviour is the opposite: a dated future is cash-settled against spot,
so its basis must converge as expiry approaches, and the carry desks exist to
enforce that. They are not enforcing it. The futures makers are self-anchored,
meaning each dated book prices from its own trades, and nothing is pulling those
books back toward spot.

This is a live defect in the ecology, not a plotting artifact. It is recorded as
an open question rather than presented as a demonstration of arbitrage working.

## 03 — triangular loop residual

`ABC/CDF x CDF/USD` against `ABC/USD`, per venue. This is the loop the triangular
desks trade. Where it sits near zero the loop is arbitraged; excursions are the
opportunities the desks are taking.

## 04 — option premium against Black-Scholes

Traded premia against a textbook Black-Scholes value at the dealer's configured
volatility, zero rates. Points on the diagonal are contracts trading at model
value. Systematic departure from the diagonal is the dealer's skew and inventory
pressure, not mispricing against a truth: no participant here knows a true value,
and Black-Scholes is drawn only as a reference curve.

## Reproduce

    ./.cache/multivenue -config research/configs/ffa-2026-08-16/demo_basic.json \
        -duration 24h -logdir logs/demo_basic -seed 91
    python tools/visualize_market.py --run logs/demo_basic --out research/visual
