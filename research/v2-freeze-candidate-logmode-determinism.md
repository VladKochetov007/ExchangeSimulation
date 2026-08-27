# Candidate log-mode determinism diagnostic

## Purpose

This is a short, non-promotional diagnostic for the V2 freeze candidate.  It
checks the required instrumentation invariant that changing scientific log
verbosity does not change the ordered execution stream.

## Source and build

- simulator source: `548f92c0f026bc92d43b28a7e471179ec50c107f`
- simulator change under test: canonical ordering of `e.Books` in
  `buildAccountMarginProfile`; this removes process-map-order dependence from
  the ordered execution observation payload
- binary: `/tmp/v2-freeze-bins3.ni8bPl/multivenue`
- binary SHA-256:
  `048204c05d0b53cffe09b37980e938601935bcdd65f95d7e8ee11c88a523e28a`
- config: the candidate `FROZEN-2` configuration, with only
  `checkpoint_interval_seconds=60` and `log_mode` varied
- seed: `101`
- horizon: `6h2m`
- `GOMAXPROCS`: `10`
- modes: `full` and `none`

## Result

Both runs completed normally.  Each produced 363 checkpoints and 25,097,365
execution observations.  The final ordered execution hash was identical:

`49e028a3e91dfc53d7d1373c7d7741f1261e072058dd812188bc7e6217adcaaf`

The complete checkpoint sequences (event counts, timestamps, and hashes) were
byte/object-equivalent, including the previously divergent checkpoint at
`sim_time=1735711260000000000` and `event_count=25015712`, whose hash was
`858776e9d67b49411562b2e3f8013fcb1e69d75970e1c499795be707dcce9d0c` in both
modes.  `greeks.json` and `latency.json` were also byte-identical.

The earlier candidate at source `29d2883` failed this comparison only after
late option-margin activity.  The first differing payload was a
`price_unavailable` observation whose missing dated-future symbol depended on
the Go map walk order.  That run remains historical diagnostic evidence and is
not promoted as freeze evidence.

## Interpretation

This diagnostic supports the narrower statement that the `548f92c` ordering
fix removes the observed full-vs-none execution-hash divergence for this seed,
horizon, and path.  It is not a substitute for fresh-process determinism at
the final candidate horizon or for the full mechanical/evidence gate.
