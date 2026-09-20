# V2 R2/SV1D development-line closeout — negative development boundary

Date: 2026-09-20
Scientific branch: `autoresearch/ffa-ecology-gen0`
Closeout scope: publish the current R2/SV1D development result and stop this
line. This is not a frozen V2 autopsy and does not authorize a successor run.

## Final status

The current R2/SV1D line is **CLOSED AT A VALID NON-ACTIVATION BOUNDARY**.

The R2 predecessor remains **`NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`**.
The corrected retained seed-659 treatment evidence is reconstructable and
evidence-valid, but the preregistered SV1D activation predicate is not
satisfied. The treatment is therefore `TREATMENT_NOT_ACTIVATED`. The
long-term V2 market-ecology project is **NOT COMPLETE**.

This closes a development line; it does not establish that finite liquidity
suppliers cannot restore liquidity, and it is not a completed 24-hour
successor campaign, freeze, holdout validation, or universal ecological
negative.

## 1. Question and registered activation requirement

The SV1D question was whether a finite, inventory-sensitive CDF/USD supplier
using only delayed local information could sometimes restore a qualifying
missing side of a one-sided public book, while bearing ordinary inventory,
balance, PnL, and withdrawal risk. The intended mechanism was not a survival
script, external price anchor, guaranteed two-sided quote, or unlimited
recapitalization.

The preregistered treatment predicate required all twelve supplier/venue
instances to have eligible delayed observations, an accepted passive order, a
fill that changed inventory, an exchange-reconciled balance/PnL/risk
transition, and a later inventory-responsive decision. It also required at
least one one-sided-mode decision and at least one qualifying restoration,
plus a local cancellation/reprice/withdrawal response, strict terminal
valuation, complete evidence, and satisfied anti-cheating gates.

A qualifying restoration was narrower than merely trading: an accepted live
supplier order had to meet the registered quantity and one-to-twenty-tick
distance bounds on the observed touch, followed by a later public snapshot
showing the missing side at the registered threshold.

The development probe was the registered five-minute seed-659 treatment,
mode-off, and no-roster design. It was not a 24-hour R2 cell.

## 2. Keep the R2 predecessor separate

The predecessor result remains the historical R2 result:
**`NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`**. It is retained as a negative
control and was not rescored under the calendar amendment or the CDF roster.

The selected R2 calendar contract is nevertheless preserved and mechanically
tested:

| family | listing cadence | time to expiry |
|---|---:|---:|
| short | 1 simulated hour | 2 simulated hours |
| medium | 3 simulated hours | 6 simulated hours |
| long | 6 simulated hours | 12 simulated hours |

The economic identity is `underlying + contract type + expiry timestamp`.
Schedule-family collisions are deduplicated, every schedule advances, and
futures and option chains share the expiry set. The registered calendar audit
expects 28 realized expiries and 23 completed expiry cycles over the
compressed 24-hour horizon. Those are contract expectations, not observations
from a completed current-line 24-hour campaign. No historical rolling-ladder
result was relabeled as a calendar result; see
`research/v2-r5-r2-calendar-amendment-2026-08-30.md`.

## 3. Exact identities and review scope

These identities are deliberately separate:

| object | identity |
|---|---|
| retained simulator trajectory | source revision `971267d6bfac030e9fb1acb3468b369f863d39a2`, source tree `1e3a2183eaf9bac8566d68f45fa607f1d3af85a0`, simulator SHA-256 `295b241cf9ef6e98b6699815f09135bf6b4e824143684884188a04f330227168` |
| retained treatment configuration | SHA-256 `bfb63ebfb7a1705819aa8a6153aea9a300920fa482d3c8f6de6d4bc564952f1f`; seed `659`; horizon `5m`; `evstream_v3`; full log |
| retained raw treatment artifacts | run root `/home/vlad/external-scratch/sv1d-activation-971267d-20260919-cgroup8g/`; `events.evs` SHA-256 `60bc339615102e0cddef21848945243dd2ce70de124f7f063dca7d2bc3db5fc5` |
| corrected analysis candidate | commit `8de607ddfb2f48bf13ff008e58892b18906d1eae`; tree `c86f26d564cf798322de3f776dbc0d91dcc78957` |
| corrected analyzer | Go 1.27.0 pinned bundle; `mvanalyze` SHA-256 `8e2bed2163647e5908815bee2efb008df6bf165e00ac32da68ce81cffa2717e5`; renderer SHA-256 `4439dcf313c065f2104090d5b48222af435a2c7e3bbed3522090c61f5f0d2bf0` |
| corrected rescore | `/home/vlad/external-scratch/sv1d-activation-rescore-8de607d-20260919/`; treatment audit SHA-256 `dac6d620fb4797d649e57c8b31d881432648f835c79ddddd773053bab7f7eae7` |
| corrected rendered treatment stream | digest `fdf35d653dbdc8f88f0757e9692011a84fb79209e41e6c28e2aa8c1b53be4d14` |
| review package | `/home/vlad/external-scratch/sv1d-rescore-bundle-8de607d/`; manifest SHA-256 `0976cfaee51f7589da904f655b19db6f81ebd0ccc7e18f728b4bed51576845b71` |
| accepted review | fresh Luna xhigh, `COMPLETED / ACCEPT`, report SHA-256 `9126e7ad1f6dad9e33d3264895a947074af70311b0922ebfd644b9d205d24bc0` |

The retained trajectory is not assigned to `8de607d`: its manifest identifies
the simulator as `971267d`. The `8de607d` review covered the queued-fill
analyzer correction and retained-evidence rescore only. It did not authorize
CDF activation, a 24-hour campaign, freeze, or holdouts.

The documentation checkpoint before this package was commit `7cebda1`; its
tree was `9cc78e7a13915e0bab9ae6d37cf84ccc34a18adb`. The final documentation
commit containing this closeout is separate from the scientific code identity.

## 4. Corrected treatment result

The exact machine fields from the corrected treatment audit and its rescore
manifest are:

| field | value | interpretation |
|---|---:|---|
| `evidence_valid` | `true` | retained evidence reconstructs under the corrected analyzer |
| `checks_count` | `0` | the rescore manifest reports zero strict evidence checks; this is not a guessed count |
| `anti_cheating_satisfied` | `true` | the registered anti-cheating checks pass at the bounded rescore scope |
| `supplier_count` | `12` | all registered supplier/venue instances are represented |
| `decision_count` | `1791` | supplier decisions were recorded |
| `accepted_order_count` | `539` | accepted passive orders were recorded |
| `fill_count` | `523` | supplier fills were recorded |
| `withdrawal_count` | `525` | cancellations/withdrawals were recorded |
| `supplier_volume_share` | `0.11359795214583537` | aggregate supplier quantity share |
| `one_sided_decision_count` | `4` | four recorded one-sided-mode decisions/episodes in the audit contract |
| `one_sided_restoration_count` | `0` | no qualifying restoration was reconstructed |
| `activation_satisfied` | `false` | the preregistered activation conjunction is unmet |
| overall `result.valid` | `false` | overall audit validity is false because activation is false; it does not override `evidence_valid=true` |

The last distinction is defined by the analyzer: overall validity requires
evidence validity, activation satisfaction, and anti-cheating satisfaction.
Thus this result is evidence-valid but non-activating. The supplier was
operationally active: it submitted orders, filled, changed inventory, bore
bounded PnL, repriced/withdrew, and remained below the registered
concentration limits. It is incorrect to describe it as “never traded” or as
invalid evidence.

The controls remain historical control artifacts. Direct treatment-contract
`cdfactivation` diagnostics on controls were not substituted for the formal
tri-arm score, and no formal tri-arm promotion score is claimed here.

## 5. One bounded retained-evidence diagnostic

One read-only `jq` pass was used on the corrected treatment audit. It confirms
that a relevant opportunity existed at the public-book level but cannot
reconstruct a complete opportunity-to-restoration denominator from the
retained aggregate fields:

| diagnostic | retained value |
|---|---:|
| central venue one-sided duration | 2 seconds, ask-only |
| north/south one-sided duration | 0 seconds each; each had a 2-second empty-book interval |
| maximum uninterrupted non-two-sided duration | 2 seconds at each venue |
| recorded one-sided-mode decisions | 4 |
| minimum eligible observations per supplier | 148 |
| accepted orders / fills | 539 / 523 |
| post-fill responsive decisions | 308 in aggregate |
| maximum censored order count / quote lifetime | 0 / 0 |
| qualifying restorations | 0 |
| terminal book mode | two-sided at all three venues |

Therefore:

- a public one-sided episode was present, and one-sided-mode decisions were
  recorded;
- the aggregate evidence does not expose whether each one-sided decision was
  joined to that episode, whether a missing-side order met the quantity and
  tick bounds, or whether a later snapshot met the restoration threshold;
- no horizon-censoring flag explains the result in the retained supplier
  summaries, but the exact episode-level denominator is **NOT DETERMINABLE
  FROM RETAINED EVIDENCE**; and
- the zero restoration count is not evidence that the supplier ignored a
  known qualifying opportunity. It is simply the registered outcome of the
  corrected reconstruction.

This decomposition is diagnostic only. It does not alter thresholds,
eligibility, activation status, or the preregistration.

## 6. Why the analyzer correction matters

The correction separated two orderings that the earlier strict analyzer had
conflated:

1. public and exchange state is reconstructed in canonical global frame order;
2. an actor receives exchange responses through a delayed local queue.

The retained central example has `Trade` global sequence `4764`, exchange
`OrderFill` `4766`, exchange cancellation `5346`, and supplier-fill evidence
`5359`; the supplier payload retains the earlier exchange execution time. A
queued, already-anchored fill processed after the exchange cancellation is not
automatically a new execution after cancellation.

The accepted exception requires exact trade/fill identity, quantity and
economics, producer ordering, execution-time ordering, and a preserved local
quote remainder. Unanchored, late, mismatched, duplicate, and overrun fills
still fail closed. The scientific gain is the distinction between invalid
evidence and valid evidence that fails the economic activation gate.

## 7. What is established and what is not

Established at this bounded scope:

- the R2 predecessor remains non-viable at its registered 24-hour survival
  gate;
- the calendar amendment is explicit, deterministic, deduplicating, and
  mechanically tested, but was not validated by a completed 24-hour ecology
  run in this line;
- the corrected retained SV1D treatment evidence is reproducible and valid as
  evidence;
- the finite supplier was active but did not satisfy the preregistered
  restoration predicate; and
- the relevant analyzer defect did not affect historical R2 trajectories,
  because the finite CDF roster was absent from those populations.

Not established:

- that finite suppliers cannot restore liquidity in general;
- that the supplier failed a completed 24-hour survival test;
- that the CDF supplier saved or destabilized the market;
- a formal tri-arm causal promotion score;
- emergent price discovery, option-surface emergence, funding-driven basis,
  or broad ecology survival; or
- completion, validation, or falsification of the entire V2 market-ecology
  project.

The historical-impact statement is scoped to this identified queued-fill
defect and the absent R2 CDF roster. It is not a claim that every later code
change is irrelevant to every historical experiment.

## 8. Stages explicitly not run

The following were not authorized or consumed from this boundary:

- no registered 24-hour successor campaign;
- no `dev-607`, `dev-613`, or `dev-617` cell;
- no `dev-607` parity or no-log controls;
- no new capacity preflight, pinned campaign build, or economic rerun;
- no freeze authorization or final V2 autopsy; and
- no holdout inspection or consumption of `619`, `631`, or `641`.

The retained five-minute seed-659 probe and its corrected offline rescore are
not equivalent to any of those stages.

## 9. Reproduction instructions

The authoritative reproduction package is the retained raw namespace plus the
corrected Go 1.27 analyzer/renderer bundle. No simulator rerun is needed.

```bash
bundle=/home/vlad/external-scratch/sv1d-rescore-bundle-8de607d
raw=/home/vlad/external-scratch/sv1d-activation-971267d-20260919-cgroup8g
rescore=/home/vlad/external-scratch/sv1d-activation-rescore-8de607d-20260919

sha256sum -c "$bundle/review-package/retained-artifact-hashes.txt"
sha256sum -c "$bundle/review-package/rescore-artifact-hashes.txt"

rendered=$(mktemp -d)
"$bundle/rebuilt-binaries/evsrender" \
  -dir "$raw/arms/treatment" -out "$rendered"
"$bundle/rebuilt-binaries/mvanalyze" -metric cdfactivation -json \
  -cdf-rendered-evidence-dir "$rendered" \
  -cdf-config-sha256 bfb63ebfb7a1705819aa8a6153aea9a300920fa482d3c8f6de6d4bc564952f1f \
  -cdf-source-revision 971267d6bfac030e9fb1acb3468b369f863d39a2 \
  -cdf-binary-sha256 295b241cf9ef6e98b6699815f09135bf6b4e824143684884188a04f330227168 \
  "$raw/arms/treatment" > /tmp/sv1d-treatment-rescore.json

jq -e '.result.evidence_valid == true and
        .result.activation_satisfied == false and
        .result.one_sided_restoration_count == 0' \
  /tmp/sv1d-treatment-rescore.json
rm -rf "$rendered"
```

The expected rendered digest is
`fdf35d653dbdc8f88f0757e9692011a84fb79209e41e6c28e2aa8c1b53be4d14` and the
expected treatment audit hash is recorded above and in the immutable rescore
manifest. The original invalid score is not overwritten by this reproduction.

## 10. Owner decision, limitations, and next boundary

The owner selected publication of this negative development boundary as the
stopping point. The raw evidence, original invalid score, and historical
verdicts remain immutable. No protected evidence was pruned, moved,
recompressed, or deleted.

The next choice is documented, but not authorized, in
`research/market-ecology-next-study-brief.md`. It is a bounded study of how
counterparty composition and capital allocation affect one existing strategy's
capacity, profitability, risk, and impact. It is not an SV1D rescue and must
not be started from this closeout.

Final status:

```text
CURRENT R2/SV1D LINE: CLOSED AT A VALID NON-ACTIVATION BOUNDARY
CURRENT CLOSEOUT TASK: COMPLETE
LONG-TERM V2 MARKET-ECOLOGY GOAL: NOT COMPLETE
FULL SUCCESSOR CAMPAIGN / FREEZE / HOLDOUT: NOT AUTHORIZED OR RUN
NEXT STUDY: PROPOSED, AWAITING A SEPARATE OWNER DECISION
```
