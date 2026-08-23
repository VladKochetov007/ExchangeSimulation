# V2-3 P0 attempt 0 — invalidated before scoring

Status: **diagnostic only; not a P0 result.**

The six worlds in `research/artifacts/v2-3-p0/{A,B,C}/seed-{101,103}` remain
retained with their manifests, raw logs, sidecars, extracted measurements, and
evidence artifact hashes. They must not be pruned or reported as the P0
screen.

## Defect

The treatment configuration is named `spot_passive_maker_post_only`, but its
implementation reached every `StoikovMMConfig` created by the multivenue
builder. This unintentionally included the `ABC-PERP` Stoikov maker. The
fixed-distance and imbalance populations can also be configured on
`ABC-PERP`, and received the same flag.

This was directly observed in B/101 persisted evidence: an accepted
`ABC-PERP` order from the perp maker has `post_only:true` at simulated time
`1735689603000000000`. Thus B minus A was not solely the declared *spot*
passive-admission intervention.

The same evidence also shows that post-only would-take rejections occurred on
the `ABC/CDF` spot book, so the attempt did exercise admission mechanics. That
does not repair the derivative scope error.

## Consequence

No market, viability, fill, or rejection statistic from attempt 0 is assigned
a causal P0 interpretation. It is retained as provenance for the implementation
defect and for the analyzer correction recorded in
`v2-3-p0-evidence-correction.md`.

The replacement attempt will use a new immutable artifact/configuration ID
after the implementation is tested. It will rerun **all** A/B/C paired cells,
including A, with one corrected binary revision. The A/B/C definitions,
seeds, horizon, no-tuning restriction, and B-A/C-B interpretation are
unchanged.
