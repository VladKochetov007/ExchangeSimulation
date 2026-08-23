# V2-3 P0 evidence-contract correction

Status: **resolved before P0 scoring; configurations and simulator execution
unchanged.**

The registered P0 configuration and causal comparisons remain exactly A/B/C.
This document records a post-run discovery in the evidence selector, so the
original preregistration and run plan remain historical records rather than
being silently rewritten.

## Finding

The first extraction attempted to select CDF/USD passive-order evidence with
the role set:

```text
spot_maker,fixed_distance_maker,imbalance_maker
```

Two separate naming/encoding facts made that wrong for the CDF Stoikov
population:

1. CDF Stoikov clients are recorded as `cdf_spot_maker_N`, whose analysis
   group is `cdf_spot_maker`; `spot_maker_N` is the independent ABC/USD
   Stoikov population.
2. Spot-book JSON envelopes intentionally omit `data.symbol`; their symbol is
   the immutable `venues/<venue>/spot/CDF-USD.jsonl` file identity. The
   post-only analyzer had treated the blank envelope field as the filter
   value, producing a false zero-event result for every symbol-selected spot
   query.

The raw CDF logs contain the relevant orders. For example, the first accepted
CDF/USD order in A/101 is in the CDF spot log at simulation time
`1735689603000000000`; the initial failure therefore did **not** show an
inactive maker or missing evidence.

## Correction and scope

`MeasurePostOnlyActivity` now recovers a symbol only from the explicit
single-book spot file path when its envelope field is empty. It does not infer
a symbol from any multi-instrument log. A regression fixture reproduces the
real logger shape.

P0 passive activation now selects exactly:

```text
cdf_spot_maker,fixed_distance_maker,imbalance_maker
```

The V2-0 receipt audit continues to use its declared `spot_maker` receipt role
because that independent telemetry contract groups the Stoikov family under
that feed role. This correction concerns only the raw-order activity metric.

No simulator code, policy field, seed, configuration, clock, population,
price rule, or raw evidence was modified. Re-extraction reads the original
six retained worlds. The correction is an analyzer/evidence defect fix, not
a post-hoc economic adjustment.
