# V2-5 P0 replacement activation run plan

Status: **preregistered after evidence-schema repair, before replacement run.**
Attempt 0 remains historical only in
[`v2-5-funding-carry-p0-attempt0-invalidation.md`](v2-5-funding-carry-p0-attempt0-invalidation.md).

## Immutable replacement cell

| field | value |
| --- | --- |
| config | `configs/v2-5-p0/activation-r1-101.json` |
| output | `artifacts/v2-5-p0/activation-r1-101/` (must not pre-exist) |
| seed / horizon | 101 / 5 simulated minutes |
| economic delta from attempt 0 | none |
| evidence delta | v2 decision-frontier/source-prefix schema only |

Before execution, normalize both JSON files by deleting only
`experiment_id` and `description`; their remaining objects must byte-match.
The replacement must use binaries rebuilt from commit `d72189c` or a descendant
containing the same committed P0 evidence fixes. It must never write into the
attempt-0 artifact directory.

## Completion and acceptance

Completion is defined solely by final `greeks.json` and `latency.json`, not a
host process name. Retain full raw logs, `manifest.json`, checkpoints, the V2
receipt sidecars, and evidence artifact digest. Extract before any pruning:

1. `observationreceipts` / V2 receipt audit;
2. `fundingcarry` audit;
3. `evidenceartifacthash` digest;
4. terminal accounting and run provenance.

The cell passes P0 only if the receipt and funding-carry audits are valid, each
present cache input has exactly one source receipt in its declared decision
frontier prefix, and all submitted legs have gateway, venue, and actor-outcome
links. A nonzero submitted-leg count is diagnostic rather than a required
success condition. Any audit failure invalidates r1; it does not authorize a
policy/configuration change.
