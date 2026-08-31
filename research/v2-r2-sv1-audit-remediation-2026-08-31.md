# V2-R2-SV1 audit remediation checkpoint

Date: 2026-08-31  
Scientific branch: `feature/r2-cdf-survival-successor`  
Exact source HEAD: `e4b04a4b5c6105561e03d234001fbc150fbf2a1f`  
Predecessor: the archived R2 candidate remains closed as
**NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**. Its configs, results, and
negative-control interpretation are unchanged.

This document records a successor-candidate remediation and a fresh
development-only activation probe. It does not authorize a 24-hour campaign or
consume any holdout.

## Why this checkpoint exists

The first exact-tree Sol-xhigh review of `a4237a2` rejected promotion. The
review was read-only and identified four blocking concerns:

1. the activation audit did not propagate per-supplier validity strongly enough
   to make an inactive treatment fail closed;
2. anti-concentration evidence was incomplete;
3. delayed local information was self-reported rather than reconciled to the
   receipt sidecars;
4. the supplier specification hard-coded the CDF/USD role and assets instead
   of keeping the participant contract configurable.

The rejected review is historical evidence, not an acceptance of this
successor. The current remediation is a new candidate revision.

## Remediation

The successor now:

- requires every configured supplier to be present in each venue's initial
  account roster and every supplier to have complete activation evidence;
- includes configured assets, symbol, finite initial balances, position limit,
  quote limit, and delayed-observation limit in the manifest-bound audit;
- binds decisions to venue-local market-data links, exact published timestamps,
  delivered frontiers, sequence numbers, message type, and fingerprints;
- permits only the initial no-observation `wait` decision, while submitted and
  resting quotes require a positive observed sequence;
- reconciles passive accepted orders, fills, cancellations, live-order count,
  displayed-depth share, per-venue volume share, aggregate volume share,
  balances, borrow events, and signed long/short PnL;
- treats negative inventory correctly in unrealized PnL reconstruction;
- retains the eight historical `elastic_supplier_N` suppliers unchanged;
- leaves the CDF roster separately configured and absent from historical R2
  configurations.

The public market-data receipt decoder also rejects schedule/receipt changes to
message type or fingerprint and scopes decision-request uniqueness by local
link. The audit is specific to the CDF successor evidence, while the actor
specification and simulator wiring accept a configured numbered liquidity role,
symbol, and asset pair.

## Validation at the remediation HEAD

All commands below ran against the exact source HEAD above:

- `GOMAXPROCS=6 go test ./... -count=1`: pass; multivenue package 192.162 s.
- clean `GOMAXPROCS=6 make test`: pass, including the integrated long-run
  contract, R2 contract, archive, and R2 archive tests.
- `GOMAXPROCS=6 go vet ./...`: pass.
- focused race suite over analysis, exchange, evstream, and multivenue:
  pass; no race report.
- focused binary-evidence contract suite: pass.
- `git diff --check`: pass.

The performance red-team branch was fetched at this checkpoint. There are no
commits after the last reviewed `c4434ad` revision. Its binary-evidence work is
not merged into this successor and no unrelated performance optimization was
imported.

## Clean binary provenance

The probe binary was built after the clean test gate with Go 1.27.0, CGO
disabled, `-trimpath`, and `-buildvcs=true`:

| artifact | SHA-256 |
|---|---|
| `multivenue` | `ae77c4042c8a2467ff95a492de13fbdad9bed0f5dc8e05a71691362334fecccd` |
| `cdf-liquidity-audit` | `24b3849835a1006790213d014f0df3ac33b668c374c0f3a9e491ffec0a55abb8` |

The build directory and probe output are outside the repository. The retained
layout is:

- treatment: `<activation-output-root>/treatment`
- control: `<activation-output-root>/control`
- audit: `<activation-output-root>/cdf-liquidity-audit.json`

The earlier failed remediation probe remains retained separately. Neither raw
probe was deleted or used as a 24-hour result.

## Fresh paired activation probe

Registered scope: `v2-r2-sv1-activation-607`, seed 607, five simulated minutes,
full logging, `evstream_v3`, treatment versus the registered no-CDF control.
The probe was run only after the remediation was built. Both processes exited
zero, and the audit rendered the binary evidence through the verified renderer.

| diagnostic | treatment | control |
|---|---:|---:|
| audit valid | true | true |
| CDF supplier count | 12 | 0 |
| supplier decisions | 1,800 | — |
| supplier fills | 111 | — |
| supplier volume | 2,400,000,000 | — |
| total CDF volume | 27,043,539,557 | 27,043,539,557 |
| supplier volume share | 8.8746% | — |
| trading suppliers | 12/12 | — |
| PnL-changing suppliers | 12/12 | — |
| realized PnL | 0 | — |
| aggregate unrealized PnL | 3,600,000 | — |
| maximum borrowed | 0 | — |
| aggregate depth over 75% | 0% | — |
| maximum supplier depth share | 22.6611% | — |
| accepted / completed / censored quotes | 416 / 409 / 7 | — |
| submit / rest / cancel / withdraw | 421 / 963 / 368 / 0 | — |
| bid absence fraction | 0.6667% | 0.6667% |
| ask absence fraction | 0.6667% | 0.6667% |

Per venue, historical supplier count was 8/8, supplier volume share was
8.8564–8.9042%, maximum supplier displayed-depth share was 19.9540–22.6611%,
and the over-75% depth fraction was zero. Every CDF supplier traded and
changed marked equity. No supplier borrow event occurred. Cancellations and
repricing evidence occurred; no forced two-sided replenishment or guaranteed
withdrawal was inferred.

The short control comparison does not establish market survival: treatment and
control have the same small initial one-sided absence fraction. The registered
24-hour survival gate remains untested.

## Promotion boundary

This checkpoint is **mechanically eligible for independent review**, not yet
scientifically promoted. The next required gate is one fresh independent
Sol-xhigh review of the exact tree at `e4b04a4`, supplied with this
preregistration, the code diff, the rejection/remediation history, and the
fresh paired-probe audit. A reviewer acceptance is required before running the
smallest full development cell. No holdout seed 619, 631, or 641 has been
run, inspected, or consumed.
