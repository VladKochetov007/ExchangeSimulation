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

The subsequent independent review of the exact `b33f7b9` tree found one more
runner/analyzer boundary defect before any rerun: the shell runner wrote
`cell` only in `run-status.json`, not in each arm's `run-metadata.json`, while
the strict completion validator compared those identities. The same review
also required the CDF identity loader to bind `metadata.experiment_id`
directly to the copied registered config rather than relying only on the outer
SV1D validator. The successor correction therefore adds `cell: $arm` to the
runner metadata, makes `cell` a strict metadata field, and rejects empty or
mismatched cell/arm and experiment/config identities. Runner-shaped metadata
and completion regressions cover missing/mismatched identity, ineligible
status, complete-horizon acceptance, and status/hash mutation fail-closed
behavior.

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

The fresh Luna review of `19c3727` then found a second, shared-provenance
schema omission: the strict SV1D arm decoder used by the actual probe path
still lacked `cell` and rejects unknown fields, even though the CDF decoder and
runner now agreed. No world or capacity measurement was run under that
candidate. The successor below adds `cell` to the shared arm metadata decoder,
requires it, binds it to the registered arm name, and adds a runner-shaped
decoder regression covering presence and omission. This is another
analyzer/provenance contract correction only; it does not alter the participant,
market, evidence, or development boundary.

## Preserved gates

* `85c927d` review, build, capacity artifacts, and activation artifacts remain
  immutable historical records.
* The failed no-cgroup capacity attempt remains retained separately and is not
  interpreted as a measurement.
* Holdouts `619`, `631`, and `641` remain untouched.
* No old economic result is rescored, and no R2 predecessor claim is reopened.

## Post-review activation finding — explicit false manifest field — 2026-09-19

The reviewed `2192451` candidate passed its fresh Go 1.27 rebuild, seed-977
finite-cgroup capacity preflight, and independent capacity verification. The
authorized development-only seed-659 tri-arm then completed simulation and
rendering for treatment, mode-off, and no-roster. The strict audit accepted
treatment and no-roster, but mode-off failed closed before scoring with:

    cdf activation: decode config: decode elastic supplier 0: missing quote_on_one_sided_local_book

The checked-in mode-off configuration did contain the explicit registered
`false` value, and its copied `run-config.json` hash matched that source. The
production simulator instead serialized `manifest.config` from the typed Go
configuration. `ElasticLiquiditySupplierSpec.QuoteOnOneSidedLocalBook` used
`omitempty`, so the false value disappeared from the manifest. The strict
decoder correctly requires the key because omission and explicit false are
different contract states. This is an evidence/provenance serialization bug,
not a CDF economic result.

The activation namespace and its incomplete typed mode-off result are retained
outside the repository and classified as `INVALID_EVIDENCE`; no arm score,
development-cell result, freeze, or holdout claim is derived from them. The
minimal successor correction removes `omitempty` from this one field and adds
a production `NewSim` regression that loads both registered treatment and
mode-off configurations, reads the emitted manifest, and requires the field
with its exact true/false value.

Because the defect was reachable in the authorized probe, the old bundle,
capacity attestation, binaries, and activation namespace do not authorize a
rerun. The corrected tree requires focused/full tests, fresh independent Luna
review, a clean pinned rebuild and bundle, a new capacity namespace, and only
then a fresh seed-659 activation. No historical R2 result was affected; no
registered 24-hour development cell or holdout was consumed.

## Independent-review follow-up — explicit mode-off triad presence — 2026-09-19

The fresh Luna xhigh review of exact `11d2497` / tree
`72931a4fa05890e00f3a33939619b172da947013` returned **ACCEPT WITH
CONDITIONS**. It confirmed that the `omitempty` correction is minimal, that
the regression exercises the production `NewSim` manifest path, and that no
market or participant behavior changes. It identified one adjacent
fail-closed contract inconsistency: `validateSV1DSupplierRoster` rejected a
present non-false mode-off value but accepted an omitted key, while the strict
production CDF decoder requires explicit presence for every supplier. An
omitted mode-off key therefore could pass the triad precheck and fail only
later during arm audit.

The follow-up correction requires `quote_on_one_sided_local_book` to be
present and explicitly false in mode-off, and adds a triad regression that
removes the field from a registered mode-off roster and requires rejection.
No-roster omission remains intentional because it has no supplier entries.
This is evidence/provenance-only, preserves fail-closed behavior, and does not
alter economics or any retained trajectory. The Luna conditions require a
fresh exact-tree review followed by a new pinned bundle, capacity preflight,
and development-only seed-659 probe; the previous `INVALID_EVIDENCE`
activation remains immutable and is not rescored.
