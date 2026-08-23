# ae13f9a clock-factorial follow-up

## Status and scope

This is a preregistered follow-up to the retained five-hour clock screen in
[`clock-artifacts-ae13f9a.md`](clock-artifacts-ae13f9a.md). It is an
intervention on timing semantics, **not** a claim about the frozen 1-second
ecology and not an economic redesign. Every arm uses the frozen participant
population, economics, seed, full log mode, and all non-timing fields without
change. Raw evidence will be retained regardless of outcome.

The screen showed that, relative to the 100ms-step control, the all-clock
destagger package compressed pooled perpetual basis from 201.01/188.78 bps to
2.80/2.37 bps in seeds 101/103. The package contained several timing changes,
so that result does not identify an individual clock or prove an LCM theory.

## Common design

- simulator freeze: `ae13f9aa6e5fd23539637a8c4a3d2d4f4c3ad107`
- duration: five simulated hours
- paired seeds: 101 and 103
- reference: retained `clock-step-100ms-5h-{101,103}.json` worlds
- runner step: 100 ms in **every** follow-up arm
- logging: full; completion only when final `greeks.json` and `latency.json`
  exist; before analysis, offline persisted-evidence digest must equal the
  runtime attestation

Primary observable: pooled absolute ABC-PERP basis in bps. Secondary,
descriptive observables: basis half-life, central triangular mean edge, 1s raw
return ACF(1), 1s absolute-return ACF(1), realized latency, and order lifecycle
contract. No threshold is introduced after outcomes are seen.

## Hypotheses and arms

| arm | changed fields relative to 100ms-step control | mechanism hypothesis | predicted discriminating result | falsifier |
|---|---|---|---|---|
| `publication` | `snapshot_interval: 1.0s -> 1.3s` | a common one-second public-book publication lattice synchronizes reactions and sustains basis | basis compresses directionally in both pairs toward the all-package result | neither pair compresses materially, or directions disagree |
| `maker-flow` | `quote_interval 1.0s -> 1.1s`; `noise_interval 2.0s -> 2.3s`; metaorder child/rest `1.0/20.0s -> 1.7/20.3s`; `future_flow_interval 5.0s -> 5.1s` | synchronization between supply refresh and directional/child flow creates recurrent dislocations | basis compresses directionally in both pairs | neither pair compresses materially, or directions disagree |
| `risk-options-carry` | `greek_interval 60.0s -> 60.1s`; `dated_carry_check_interval 0 -> 1.9s`; `maker_hedge_interval 60.0s -> 60.7s`; option-dealer hedge `60 -> 61s`; option-value `5.0s -> 5.3s`; VV `10.0s -> 10.7s` | periodic carry and option-risk flows phase-lock with the step and indirectly move perpetual basis | basis compresses directionally in both pairs | neither pair compresses materially, or directions disagree |

The three arms deliberately form non-overlapping partitions of the prior
package, except for the common 100ms scheduler step. `automation_interval`
stays at 1s in all arms because it was unchanged in the previous package; it
is not an identified factor here.

## Decision rule

This is screening-level causal localization only. For seed `s`, let `B_s` be
the retained 100ms-step basis and `D_s` the retained all-package basis. An arm
is provisionally **IMPLICATED** only if, in **both** seeds, its paired basis is
at or below `(B_s + D_s) / 2`: at least half of the already-observed package
compression. An arm with compression in only one seed or with discordant
directions is **UNRESOLVED**, not averaged. If no arm meets the primary rule,
the result is **INTERACTION UNRESOLVED**; a later factorial will be required
before ascribing it to a particular cadence. A large change in a secondary
metric is descriptive and cannot replace the primary-basis criterion.

## Evidence checks

For every arm/seed retain the resolved config and SHA-256, simulator and
analysis binary SHA-256, run horizon, runtime/offline evidence digest, latency
sidecar, `orderlifecycle`, and the metrics named above. Any absent final
sidecar, digest mismatch, malformed log, or lifecycle failure invalidates that
world rather than being treated as a result.

## Adaptive second-stage isolation

The first-stage outcomes are recorded separately below; they are not used to
rewrite its decision rule. All three families crossed the specified primary
threshold, so the next question is not whether the package matters but whether
the two narrowest, code-motivated cadences can each account for it. This is a
new discovery follow-up with its own declared tests.

| arm | changed field relative to the 100ms-step control | hypothesis | primary prediction | falsifier |
|---|---|---|---|---|
| `quote-only` | `quote_interval: 1.0s -> 1.1s` only | maker refresh phase relative to deliveries/flow is sufficient for the maker-flow family | paired basis is at or below `(B_s + D_s) / 2` in both seeds | either seed remains above its midpoint |
| `dated-carry-only` | `dated_carry_check_interval: default QuoteInterval (1.0s) -> 1.9s` only | the documented dated-carry/quote phase lock is sufficient for much of the risk/options/carry family | paired basis is at or below `(B_s + D_s) / 2` in both seeds | either seed remains above its midpoint |

Both arms retain the same five-hour horizon, full evidence, paired seeds, 100ms
step, and acceptance gates as stage one. The differing interval changes rate
as well as phase, so even a pass establishes *cadence sensitivity* rather than
an LCM-only mechanism. A pure initial-phase experiment is not exposed by the
frozen configuration; adding one would be a simulator-semantic feature and is
out of scope for this ae13f9a autopsy.
