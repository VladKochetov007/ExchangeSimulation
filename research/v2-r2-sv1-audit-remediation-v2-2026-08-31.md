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

## Binary evidence neutrality checkpoint — 1db7067

The fresh probe after `1db7067` found and corrected an evidence-contract defect
that was not visible in the earlier candidate review. `LogEvidenceOnly` records
share a per-venue route sequence with execution records so the renderer can
merge them exactly. Those records are persistence metadata, not economic
execution. Before this checkpoint, enabling the optional sidecars changed the
route sequence embedded in later binary frames and therefore changed the
binary execution hash, even when the economic trajectory and checkpoints were
unchanged.

The correction adds an explicit `evstream.FrameHasher` projection and declares
the production binary contract as `route_sequence_neutral_v1`. Production
binary frames retain the route sequence for deterministic reconstruction, while
the execution digest hashes that field as zero. The default evstream contract
remains raw-frame hashing, so historical and manually constructed raw streams
are backward-compatible. The renderer selects the hash projection from the
attestation before it verifies the completion trailer and rejects unknown
contracts.

The exact scientific tree tested here is `1db7067ee470d4ee587fff047ede6d5524d14a1c`.
The asynchronous performance branch was fetched at this checkpoint; it had no
commit newer than reviewed `c4434ad`. No performance-branch implementation was
merged.

Mechanical gates on the pushed tree:

* clean `GOMAXPROCS=4 make test` passed all Go packages, integrated long-run
  contracts, R2 contracts, and both archive suites;
* `GOMAXPROCS=4 go vet ./...` passed;
* focused `evstream`, `types`, `exchange`, and `simulations/multivenue` tests
  passed;
* targeted race tests passed for `analysis`, `exchange`, `evstream`, and the
  binary/CDF/render multivenue tests;
* the full `simulations/multivenue` package passed in 191.854 seconds after the
  correction.

The clean pinned binaries were built with Go 1.27.0, `CGO_ENABLED=0`,
`-trimpath`, `-buildvcs=true`, and `vcs.modified=false` at the exact tree above.

| binary | SHA-256 |
| --- | --- |
| `multivenue` | `6b5da13729b6246ab6300a276c26486b5b046336857350db007b6a75b62628d4` |
| `cdf-liquidity-audit` | `28b144c7b5e3a88e055b04ec9e8a67a6097abe46c8cd64c1730100262522f069` |
| `evsrender` | `1dc12a7606f9f4cc2fd231ad598d7e28c54cadd58ec8cc24cef9340f1f2202ab` |

### Fresh development-only evidence probe

The probe used seed 607 and a five-minute simulated horizon. It ran two
identical treatment processes, one evidence-off process with all optional
receipt/decision streams disabled, and the paired no-CDF control. All completed
successfully. The raw evidence and rendered outputs remain in external scratch
storage and are not substituted for a registered development campaign.

| artifact | on-a | on-b | evidence-off | no-CDF control |
| --- | ---: | ---: | ---: | ---: |
| binary event frames | 150,771 | 150,771 | 150,771 | 148,701 |
| binary stream frames | 150,851 | 150,851 | 150,851 | 148,780 |
| normalized execution hash | `254d730774c3dfebb4845d3bf6ace6d4a4f96fd078b7140d9676ede64b6504e0` | same | same | `c56390b966fb954acd6a49d0c7260093b1779c613472f6191cd0f530335e2d58` |
| checkpoint file hash | `a9bcb53a2ac0febf249966fc962555cbc1b4ecca9fda3ff1ff14102643ee3fc5` | same | same | `fd51f0b2b1c6082c1f90af4b9866ab63f74482c071d0e96ddcb5c16fb9e90a46` |

The two treatment raw streams were byte-identical. The evidence-off raw stream
was different, as expected from its different sidecar-derived route sequence,
but its attestation and normalized execution identity were identical to the
treatment. Rendering passed for treatment A, treatment B, and evidence-off;
the three compact render reports were byte-identical, and the treatment A/B
route JSONL files were byte-identical. This establishes fresh-process
determinism and evidence-neutral execution identity without pretending that
the stored persistence metadata is identical.

The rebuilt treatment audit was valid. It observed 12 configured CDF suppliers,
1,800 decisions, 111 supplier fills, 2,400,000,000 raw CDF units of supplier
volume, and 8.874577955823758% supplier volume share. All 12 suppliers traded
and changed PnL. The configured 5-bps maker fee was present in actual supplier
fill records as positive quote-asset fees, and the audit's balance and account
equity residuals were both zero. The largest supplier resting-depth share was
22.661106211326726%; individual supplier shares were approximately 2.56% to
3.10%, and no supplier exceeded the 75% concentration threshold. This is the
largest venue-aggregate supplier-depth share, not an individual supplier
share.
The treatment had six bid-absent and six ask-absent snapshots out of 900, or
`0.006666666666666667` on each side, matching the control. Ordinary flow had no
withdrawals, so the stale/one-sided withdrawal activation criterion remains
unproven; the focused delayed-cancellation and stale-withdrawal tests remain
mechanism-level evidence only.

The fresh audit artifact hash is
`5a995a7ae7c7c1045122e85cde347aaab99d868377352d87847f9072e6804eb6`.

## Promotion status after neutrality correction

The evidence representation is now mechanically coherent and fresh-process
neutral, but this is not freeze authorization. The next gate is one fresh
independent Sol-xhigh review of the exact tree, the explicit hash contract, the
rendered evidence, and the activation audit. That review must decide whether
the ordinary-probe absence of withdrawals requires a dedicated development
stress probe before any larger campaign. No registered dev-607 run and no
holdout has been consumed.

## Independent promotion review — rejection at a60ce0b

Helmholtz performed a fresh independent Sol-xhigh review of the exact pushed
tree, the binary attestation contract, the rendered probe evidence, and the CDF
activation audit. The reviewer rejected the candidate for promotion and for a
24-hour development campaign. This is a rejection of the evidence and
activation gate, not a rejection of the finite CDF supplier hypothesis. No
holdout was authorized, inspected, or consumed.

The review identified the following concrete blockers:

* The analyzer rejects every decision whose observation age exceeds the
  configured maximum, including the actor's deliberately emitted stale-data
  withdrawal. Therefore the required end-to-end stale withdrawal criterion
  cannot currently pass; the existing stale test only exercises the actor and
  not the complete gateway, receipt, frontier, and final-audit path.
* The neutral execution digest alone does not attest the route sequence used
  by the renderer. A sequence swap can preserve the normalized digest and the
  sequence set while changing reconstructed order. The successor contract must
  attest both normalized economic identity and raw canonical reconstruction
  identity, and must include a sequence-swap mutation test.
* Supplier resting-depth shares use the current snapshot as the weight for the
  preceding interval and omit important empty/current intervals. The contract
  must specify left- or right-continuous depth semantics, include empty and
  terminal intervals, and have hand-checkable tests.
* Terminal receipt validation does not require every schedule due by
  `TerminalAt` to have a receipt, nor does it validate global event ordinal
  continuity across schedule, receipt, and decision records. The missing
  post-terminal schedules observed in the probe are legitimate, but the
  completeness rule is not enforced.
* The endpoint checkpoint is a prefix: it has fewer event frames than the
  completed binary attestation because equal-time final writes are suppressed.
  The evidence contract must either write an explicit final checkpoint equal to
  the completed attestation or classify the existing row as a prefix.
* Focused fixed-point tests are still required for partial fills, long/short
  closure, reversal, weighted-entry rounding, fees, and quote-capital
  admission. In addition, the reviewer noted that quote admission is not yet
  bounded by pre-trade quote headroom; generic exchange borrowing can occur and
  is rejected only after the fact.

The remediation decision is to reproduce each invariant on the scientific
tree, add fail-closed regressions, and make minimal attributable fixes before
another narrow development stress probe. The historical R2 predecessor remains
archived as a negative control and is not rewritten.
