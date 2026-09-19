# V2 research progress report — last 24 hours

Status: **PAUSED BY OWNER after this report**  
Scientific branch: `autoresearch/ffa-ecology-gen0`  
Scientific code baseline: `8de607ddfb2f48bf13ff008e58892b18906d1eae`  
Scientific code tree: `c86f26d564cf798322de3f776dbc0d91dcc78957`

No further research, development, experiments, reviews, builds, or holdout
work should begin until a new owner prompt gives instructions.

## What the goal was

The active goal was to establish a scientifically defensible V2 integrated
artificial-market ecology candidate, not to make every realism metric green.
The required result is a reproducible account of which effects are endogenous,
inherited, inactive, falsified, or unsupported, with strict evidence,
independent adversarial review, and a development/holdout boundary.

The immediate scientific line was the R2 derivative-calendar amendment and its
successor liquidity question:

- replace stalled rolling tenor cursors with a deterministic expiry calendar;
- deduplicate schedule-family collisions by economic expiry;
- provide overlapping futures and option maturities over the compressed 24-hour
  horizon; and
- test whether a finite, delayed-information, inventory/risk-bearing CDF/USD
  supplier could keep the ecology valuatable without becoming a scripted
  survival device.

The long-term goal is not complete yet. No registered 24-hour development cell,
freeze authorization, or untouched holdout has been consumed in this current
line.

## What changed in the last 24 hours

The work moved from a repeatedly invalid activation packet to a mechanically
reconstructable, scientifically bounded negative result.

### 1. Evidence and activation-contract hardening

The succession of focused commits corrected real reachable contract defects:

- `88ea00d` deferred transient scheduled-risk telemetry gaps without relaxing
  stale-mark or terminal-valuation rules;
- `47bd21a`, `85fc5a4`, `b33f7b9`, `19c3727`, `bf91ef0`, and `2192451` aligned
  arm, manifest, cell, config, and identity schemas;
- `3d1135f` admitted only the explicit empty sequence-zero bootstrap frame and
  excluded it from actor-observable snapshots;
- `8103bcd` accepted the valid zero-based first trade identity;
- `4f14dfb` made supplier decisions use the actual participant-local callback
  execution time, preserving delayed-information causality;
- `7f28bfa`, `a113a7f`, and `4d7f4b7` closed terminal-tail, live-order, and
  same-timestamp rejection-order evidence gaps;
- `4908766` reconstructed post-fill local quote remainders instead of
  comparing repricing against stale self-authored quantities; and
- `8de607d` reconciled a legitimate queued supplier fill after exchange-side
  cancellation while retaining fail-closed rejection for unanchored, late,
  mismatched, or overrun fills.

These changes did not retune the market, add capital, change the calendar,
change seeds or horizons, or alter historical trajectories. They made the
measurement contract capable of distinguishing invalid evidence from a real
economic non-activation.

### 2. R2 calendar semantics were preserved

The selected compressed calendar is:

| family | listing cadence | time to expiry |
|---|---:|---:|
| short | 1 simulated hour | 2 simulated hours |
| medium | 3 simulated hours | 6 simulated hours |
| long | 6 simulated hours | 12 simulated hours |

The identity is `underlying + contract type + expiry`. Overlapping requests are
one economic instrument; every schedule advances. Futures and option chains use
the same expiry set. The registered audit expects 28 realized expiries and 23
completed expiry cycles in the 24-hour compressed horizon, with simultaneous
near/far maturities and deliberate schedule-family overlaps.

This was treated as a scientific successor amendment, not a mechanical fix.
Historical rolling-ladder results remain attached to their original
populations and were not rescored.

### 3. The finite CDF successor was adjudicated honestly

The retained seed-659 treatment evidence originally failed strict extraction.
The queued-fill defect was independently reproduced and classified as an
analyzer defect: exchange state is ordered by global frames, while a supplier
can process an exchange response from its delayed local queue after a
cancellation frame. The minimal correction was implemented and reviewed.

The corrected retained-evidence treatment rescore now reports:

- evidence valid;
- zero strict evidence checks;
- anti-cheating predicates satisfied;
- 1,791 supplier decisions;
- 539 accepted orders;
- 523 supplier fills;
- 525 withdrawals;
- supplier volume share `0.11359795214583537`;
- zero one-sided-book restorations; and
- `activation_satisfied=false`.

The correct status is `TREATMENT_NOT_ACTIVATED`. This is not an invalid-evidence
failure and not a positive or negative 24-hour market-survival claim. No formal
tri-arm promotion score was substituted for the retained control contract, and
the old invalid score and raw artifacts remain untouched.

## Findings and scientific value

The main scientific gain was preventing a false claim.

1. The predecessor R2 candidate remains **NON-VIABLE AT THE 24H
   MARKET-SURVIVAL GATE**. It is preserved as a negative control.
2. The calendar/lifecycle amendment is mechanically specified and tested, but
   its broad ecology-level consequences have not been validated on a completed
   24-hour campaign.
3. The finite CDF supplier hypothesis did not meet its preregistered short
   activation predicate in the retained probe. It therefore cannot authorize
   a full campaign or be described as a successful endogenous survival
   mechanism.
4. The analyzer defects found in this successor did not affect historical R2
   trajectories because the finite CDF successor roster was absent from those
   worlds.
5. Broad claims about ecology-wide survival, emergent price discovery,
   option-surface emergence, or funding-driven basis remain unsupported in
   this line. Earlier registered negative/inactive claims were preserved rather
   than “rescued.”

## Verification and resource state

The exact scientific code baseline passed the required mechanical checks before
this pause:

- full `make test` in a clean detached worktree: **PASS**;
- `go vet ./...`: **PASS**;
- targeted race suite: **PASS**;
- binary evidence contract tests: **PASS**;
- fresh-process determinism/evidence-neutrality tests: **PASS**;
- `git diff --check`: **PASS**;
- independent Luna review of exact `8de607d`: **ACCEPT** for the bounded
  analyzer-correction/rescore gate.

A first `make test` from the documentation-dirty main worktree correctly failed
only at the clean-worktree parity/archive guard. The same exact code revision
then passed all packages, integrated-long-run contracts, R2 contracts, archive
tests, and parity/archive checks in the clean detached worktree
`/home/vlad/external-scratch/v2-docs-clean-test-8de`.

At pause:

- disk: about 29 GiB free of 116 GiB;
- RAM: about 26 GiB available of 31 GiB;
- swap: none in use;
- no simulator, analyzer, renderer, probe, or test process remains active;
- holdouts `619`, `631`, and `641` remain untouched.

The performance branch was fetched at the natural checkpoint. No commit newer
than the last reviewed `b1847ac` was present, and no performance branch code
was merged into the scientific branch.

## Instructions followed

- Read the supplied persistent objective and the current `RESUME-HERE`,
  `V2-CURRENT-STATE-AUDIT`, and remote handoff context before acting.
- Treated current Git state, retained evidence, and exact source identities as
  authoritative; did not rewrite historical results.
- Kept the R2 calendar amendment separate from the SV1D economic successor.
- Required focused regressions, full tests, static checks, race checks,
  provenance checks, fail-closed extraction, and independent review at the
  meaningful gate.
- Used only development-only seed 659 for the successor probe and did not
  inspect or consume the reserved holdouts.
- Used the existing binary evidence contract but did not merge the independent
  performance branch wholesale or adopt unrelated optimizations.
- Used `rg` for repository search, Go for data-intensive processing, and tmux
  for the long-running full test gate. Python was not used for data processing.
- Kept CPU/RAM bounded during verification and monitored disk, RAM, and active
  processes.
- Did not use Astra. The independent review work used the permitted Luna/Terra
  paths; no reviewer was allowed to edit the candidate or run holdouts.
- Preserved raw evidence and did not delete logs or artifacts to satisfy a
  capacity floor.

## Exact state at pause

The code is at `8de607d`; the only current worktree changes are the requested
append-only documentation checkpoint:

- `research/RESUME-HERE.md` — current pointer updated;
- `research/V2-CURRENT-STATE-AUDIT.md` — current pointer updated;
- `research/v2-r2-sv1d-activation-closure-2026-09-19.md` — successor closure;
- this report.

Those documentation changes do not alter executable source, configs, evidence,
or scientific results. They are being committed as one documentation-only
pause checkpoint.

## What remains, when the owner resumes

The next decision is scientific, not a routine coding task:

- either explicitly publish this negative development boundary as the stopping
  point for the current R2/SV1D line; or
- register a materially different, economically motivated successor with new
  activation and kill criteria, then obtain fresh exact-tree review before any
  capacity or simulation work.

Do not resume by launching `dev-607`, `dev-613`, `dev-617`, parity cells, or
holdouts from the non-activating SV1D result. Do not repeat analyzer-only fixes
against the same mechanism, retune liquidity to force survival, reopen old
historical lines, or reinterpret retained trajectories offline.

