# SV1D activation contract repair — 2026-09-19

## Boundary

The scheduled-risk successor at `85c927d9a087be9108992c56df4994a291d9cd56`
(tree `e93c1a866ba67c63c0e328aceb3230e01dbeb799`) passed its clean mechanical
tests, independent source review, Go 1.27 rebuild, and outcome-ineligible
seed-977 capacity preflight. The capacity result is retained under:

    /home/vlad/external-scratch/sv1d-capacity-85c927d-20260919-cgroup8g.attestation.json

The authorized development-only seed-659 tri-arm was then run with the exact
pinned binaries, registered configurations, and finite 8 GiB cgroup envelope.
All three simulators reached the five-minute endpoint and all renderers
completed. The strict activation audit failed closed for every arm, so no
activation score or scientific directional result was issued.

The raw activation namespace is retained at:

    /home/vlad/external-scratch/sv1d-activation-85c927d-20260919

This is an analyzer/configuration-contract failure, not evidence that CDF
liquidity activated or failed economically. C5 and the `85c927d` activation
attempt remain historical incomplete arms and are not rescored or rewritten.

## Observed failures

The retained strict-audit stderr was:

* treatment: `cdf activation: decode run status: json: unknown field "cell"`;
* no-roster: `cdf activation: decode run status: json: unknown field "cell"`;
* mode-off: `cdf activation: decode config: decode elastic supplier 0: missing quote_on_one_sided_local_book`.

The activation runner emits `v2-r2-sv1d-arm-status-v2` with the arm cell,
experiment/config identities, hypothesis, and `scientific_result_eligible`
fields. The strict CDF status decoder omitted those producer fields even though
the capacity status decoder already modeled them. This is a reachable analyzer
schema defect exposed only after a complete binary-evidence run.

The mode-off registered roster intentionally disables one-sided quoting. Its
runtime default was false, but the strict CDF configuration contract requires
the economic option to be explicit for every supplier. The checked-in mode-off
JSON omitted the field, making the registered config unrepresentable to the
strict analyzer. Adding explicit `false` values does not change simulator
behavior; it closes the config/provenance contract and changes the successor
config identity as required.

## Minimal successor correction

The successor correction makes two bounded changes:

1. `analysis/cdf_activation.go` accepts and requires the complete arm-status
   identity fields emitted by the activation runner, including `cell`, and
   rejects a status that claims scientific eligibility. A regression fixture
   covers the exact runner schema.
2. `activation-659-mode-off.json` explicitly records
   `quote_on_one_sided_local_book: false` for all four finite supplier rows.
   The registered config checker now requires that explicit field, while the
   existing tri-arm identity filter continues to ignore only this registered
   one-sided option when comparing treatment and mode-off.

No matching, risk, lifecycle, participant economics, seed, horizon, schedule,
event order, historical supplier, capacity formula, or holdout boundary is
changed by this correction. The config hash, probe plan, binary bundle, review
package, and all downstream evidence identities must nevertheless be rebuilt
because a registered configuration and analyzer source changed.

## Scientific classification and next gate

Classification: **ANALYZER/REGISTERED-CONFIG CONTRACT BUG, REACHABLE IN THE
AUTHORIZED DEVELOPMENT PROBE, NO ECONOMIC OUTCOME AVAILABLE**.

The failed arm evidence cannot be repaired offline: the original trajectories
remain retained, but they are not valid strict activation inputs under the
candidate's own fail-closed contract. No historical experiment is affected;
this exact strict activation path did not exist in an accepted scientific
result, and no holdout or development cell was consumed.

The successor must pass focused tests, full tests, vet, race and fresh-process
evidence checks, then receive a fresh independent review of the exact final
tree. A new Go 1.27 pinned bundle and capacity-bound review package are
required before rerunning seed 659. The next seed-659 result remains
development-only and cannot authorize dev cells, freeze, or holdouts by itself.

## Preserved gates

* `85c927d` review, build, capacity artifacts, and activation artifacts remain
  immutable historical records.
* The failed no-cgroup capacity attempt remains retained separately and is not
  interpreted as a measurement.
* Holdouts `619`, `631`, and `641` remain untouched.
* No old economic result is rescored, and no R2 predecessor claim is reopened.
