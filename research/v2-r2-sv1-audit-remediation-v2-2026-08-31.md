# V2-R2-SV1 audit remediation v2

Date: 2026-08-31  
Candidate: `V2-R2-SV1-CDF-LIQUIDITY`  
Scientific branch: `feature/r2-cdf-survival-successor`  
Predecessor: R2, retained unchanged as **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**  

This is an append-only successor record. It does not rewrite the predecessor
result or reinterpret retained historical JSON experiments.

## Boundary and inputs

The exact scientific HEAD remediated here was:

```
ad51e7ae138c8c25195f11d581890c71088383ae
```

The preceding independent Sol-xhigh review was Carver's review of `f5fc52a`.
It rejected promotion to the 24-hour campaign while allowing only narrow
mechanism activation. Its four blockers were:

1. the configured 1000-CDF inventory limit was only a signed displacement
   limit, allowing roughly 2000 CDF gross holdings;
2. aggregate balance-snapshot fields were reported as zero and aggregate
   historical-count semantics were ambiguous;
3. account-equity PnL hid endowment revaluation and fixed-point trading
   rounding;
4. the extractor did not verify the receipt-prefix digest or compare a local
   supplier frontier with the gateway's order-decision frontier.

The performance feed was fetched at this checkpoint. There were no commits
newer than the last reviewed `c4434ad` on
`origin/autoresearch/v2-performance-research`; no performance code was merged.
No holdout artifact or holdout seed was consumed.

## Remediation contract

`MaxPosition` remains a signed position-displacement limit. The successor now
also registers `MaxInventory` as an absolute gross base holding limit. The
development roster deliberately uses:

* initial CDF balance: 500 CDF;
* signed position limit: +/-500 CDF;
* absolute gross CDF limit: 1000 CDF;
* initial USD balance and all other economic parameters unchanged from the
  successor activation registration.

The actor reports initial base balance, signed position, gross inventory, and
gross inventory limit on every decision. Its target clamp respects both the
position-displacement and absolute-holding bounds. The extractor checks the
reported identity, the exact `initial base + position` relation, and every
balance snapshot's gross holding against the registered cap.

The extractor now reports aggregate balance-snapshot counts and residuals by
exact accumulation of the per-supplier values. `ExpectedHistoricalCount` is
the aggregate count across observed venues; each venue retains its own
per-venue expected count.

PnL is reported in three auditable pieces:

* `EndowmentRevaluationPnL`: terminal marks applied to the initial balances
  minus initial marks applied to those same balances;
* `TradingPnL`: terminal marked wallet value minus the terminal-marked initial
  endowment;
* `TradingPnLReconciliationResidual`: trading PnL minus realized plus
  unrealized PnL.

The existing account-equity versus marked-wallet residual remains separate.
Trading decomposition residuals are accepted only within an explicit
fixed-point allowance of the supplier fill count plus two quote atoms; larger
residuals invalidate the audit. This exposes, rather than silently erases,
integer execution-rounding effects.

The CDF decision JSON now carries the local receipt-prefix digest. The
extractor reconstructs each per-link receipt chain from the canonical receipt
records and checks that digest. It also decodes the gateway's decision-record
frontier and requires link, ordinal, delivery time, and prefix digest equality
with the local supplier decision. Missing or malformed digests fail closed.

## Tests added or strengthened

The focused CDF tests now cover:

* aggregate balance snapshot counts and zero residuals;
* receipt-prefix digest mutation;
* gross-inventory-limit mutation;
* positive quote fee conservation and decomposition;
* existing fingerprint, ordinal, malformed-balance, missing-snapshot, and
  terminal-censoring mutations.

The actor-side tests and roster fixture use the explicit successor inventory
contract. `git diff --check` passed. The focused race suite passed:

```
GOMAXPROCS=2 go test -race ./analysis ./exchange ./evstream ./simulations/multivenue \
  -run '^Test(MeasureCDFLiquidity|CompareCDFLiquidity|ElasticLiquiditySupplier|BalanceSnapshot)' -count=1
```

`go vet ./...` passed. A preceding dirty-tree `make test` reached all Go
packages but intentionally failed its provenance-sensitive parity/archive
checks because the working tree was modified. After committing the exact
remediation, clean `GOMAXPROCS=4 make test` passed all Go packages, integrated
long-run contracts, R2 contracts, and both archive suites.

## Fresh corrected activation probe

The clean binaries were built from `ad51e7a` with `/usr/local/go/bin/go`
1.27.0, `CGO_ENABLED=0`, `-trimpath`, and `-buildvcs=true`. All three reported
`vcs.modified=false` and the exact source revision.

| binary | SHA-256 |
| --- | --- |
| `multivenue` | `028c25bfce496f191be07cc289ec250950c8674845d0334d867b0c0ee49157ce` |
| `cdf-liquidity-audit` | `b69fbe90e72c98deac98ee809dbb886a8bc1184f9b03fb8381ef0224baf1e307` |
| `evsrender` | `ff7e836397afcd61b74c443529e14c0b2c3839b84d944f8cb66ff79d51eb4c37` |

The development-only paired run used seed 607, a five-minute horizon,
`GOMAXPROCS=4`, full `evstream_v3` evidence, and the registered treatment and
no-CDF control configurations. It ran in an external scratch directory that
is intentionally not part of the repository.

```
external-scratch/v2-r2-sv1-activation-ad51e7a
```

The raw evidence remains retained there. It has not been substituted for an
archived scientific campaign artifact.

| diagnostic | treatment |
| --- | ---: |
| audit valid | true |
| CDF suppliers | 12 |
| decisions | 1,800 |
| supplier fills | 111 |
| supplier filled quantity | 2,400,000,000 raw CDF units |
| supplier CDF volume share | 0.08874577955823758 |
| balance snapshots | 60 |
| balance residual | 0 |
| account-equity/marked-wallet residual | 0 |
| endowment revaluation PnL | 100,000,000 raw USD units |
| trading PnL | 3,599,986 raw USD units |
| realized PnL | 0 |
| unrealized PnL | 3,600,000 |
| trading decomposition residual | -14 raw USD units |

The largest observed supplier gross base holding was 502 CDF (raw value
`50,200,000,000`), below the registered 1000-CDF cap (`100,000,000,000`).
All 12 suppliers traded and changed PnL. The control had zero CDF suppliers
and was independently valid. The treatment and control side-absence fractions
remained equal at `0.006666666666666667` on each side; the probe therefore
does not establish a survival improvement.

Fresh evidence hashes:

| artifact | SHA-256 |
| --- | --- |
| treatment `events.evs` | `a2ea8500c9d314c72e0acb323b1394054caaf76b1c653238c02bc9eb44d1745f` |
| control `events.evs` | `0e7f50dbc8981d788f7ee80ac9a90dc9ffc20c8da1b6906dec6d2b87ef26e82b` |
| treatment `greeks.json` | `70dbf4f9691b9571e86044b74c974b28a95aa4be11677d3729a53853a3aaabfe` |
| control `greeks.json` | `b4f2a1f45d2eadec6e1a444ecf0dd3b1eea0c0238cb10603195c9dbcb7818404` |
| `cdf-liquidity-audit.json` | `6eb653dbe67b1806698b37e0fa40812f3e35d324f9b805620fa85c065680791a` |

## Scientific status

The remediation closes the four concrete review blockers for a fresh
candidate-specific review at the narrow activation scope. It does not grant
24-hour freeze authorization. The next gate is one fresh independent
Sol-xhigh review of the exact post-remediation tree and probe. If that review
accepts the candidate, the next development work must still exercise stale or
one-sided withdrawal, terminal censoring, concentration, determinism, and
evidence-neutrality before any 24-hour campaign consideration. Holdouts remain
untouched.

## Post-v2 hardening checkpoint — c6bfcf5

The exact successor implementation checkpoint is now `c6bfcf5`:
`fix: harden CDF activation evidence boundaries`. It is pushed on the
successor branch. The performance feed was fetched again at this checkpoint;
there are still no commits newer than reviewed revision `c4434ad`, and no
performance-branch code was merged.

The CDF fee schedule is now real configuration rather than an audit-only
fixture. Each registered successor supplier is wired to its configured maker
fee, and the extractor recomputes the expected quote-asset fee from price,
quantity, precision, and registered basis points. Positive fees therefore
must be visible in fills and in the supplier's balance/PnL reconciliation.

The participant information boundary now includes a separate fixed-width
gateway-action sidecar. It records subscribe, place, cancel, and unsubscribe
requests with request identity, order identity where applicable, decision
time, and the exact delayed-feed frontier. Its own sequence is contiguous and
validated. CDF submit and cancellation decisions must match the corresponding
gateway request; missing, reordered, stale-frontier, digest-mutated, or
identity-mutated action evidence fails closed. The legacy schedule/receipt /
place-decision contract remains unchanged for existing consumers.

The extractor now separates live accepted quotes, pending submissions, and
accepted quotes whose cancellation is still pending. It reports per-supplier
volume share and time-weighted resting-depth share, including zero-depth
intervals. The actor's quote quantity is bounded by admission-time gross
inventory headroom on both buy and sell sides. Focused tests cover gross-cap
boundaries, one-sided withdrawal, stale withdrawal after the configured age,
delayed cancellation without replacement, action sequence integrity, and a
semantic cancellation-identity mutation.

The focused `analysis`, `simulation`, and `simulations/multivenue` suites are
green at this checkpoint. A clean full `make test` is the next mechanical
gate; no development or holdout cell has been started.
