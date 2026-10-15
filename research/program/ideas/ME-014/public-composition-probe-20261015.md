# ME-014 public-API composition probe — 2026-10-15

Status: **synthetic mechanical finding only; ME-014 remains PREPARE / NOT RUN-READY.**

## Question and pinned source

Can a consumer package outside the exchange and simulation packages use the
existing public APIs on `main` to preflight a coherent local mark set for every
exposed fixture account before any liquidation sweep, and stop the same-time
venue phase if the preflight fails?

The probe is based on source commit
`d654d323e777505c7d64cac065d29fa288cc926e`, tree
`3f70580785b1d5754dee509dc6d7bcccbbf6e9c8`. The test is an external consumer package at
[`composition_test.go`](../../../me014composition/composition_test.go), not a
change to library source.

## Result

The narrow composition claim survives: with exclusive ownership of a
deterministic callback and no interleaving state mutation, a consumer can:

1. read a positive two-sided local spot midpoint through `TwoSidedMidPrice`;
2. install it as both perp index and mark reference with
   `UpdateMarkReferences`;
3. commit each venue's mark set with `CommitMarkEpoch`;
4. preflight its fixture accounts with `MarkedAccount` and compare the returned
   position marks to the values selected by that callback;
5. only after every venue/account preflight succeeds, call `CheckLiquidations`
   using the matching trigger mark.

The fixture also checks that a mismatched trigger mark does not produce a
liquidation-check event, that an unpriceable second venue prevents all venue
sweeps, and that wiring the failing local-mid preflight into
`BeforeTimestampPhase` stops the runner before same-time venue phases. A
separate control verifies generic registered-stop finalization at the actual
clock.

This answers only “can these public methods be sequenced by an external
consumer?” It does not establish that this mark policy is economically correct,
that it matches the exchange's normal configured mark calculator, or that a
production E2 world has the required exclusive event-loop ownership.

## Consequence for the ME-014 gate

On this `main` baseline, risk preflight ordering is not shown to require a new
exchange method: a consumer-owned runner callback can sequence the public mark,
epoch, account-valuation and liquidation calls for a fixed roster. It remains
conditional on settling the mark-source contract and proving runner phase
ownership, including the intermediate-event drain requirement above.

The separate payment-commit gate remains open. Main provides
`CaptureFundingAccountSnapshot`, `CaptureOwnedFundingBatchInput`, the pure
`PreviewFundingBatch`, and one-time `EndowFundingRoundingReserve`; the preview's
own contract says a later adapter must compare the whole snapshot at commit and
record account and reserve movements atomically. `Client.MutatePerpBalance`
alone does not commit persistent fractional accrual state, finite reserve
movements, and canonical evidence as one transaction. This probe neither
implements nor validates such a committer. The `SettleFundingEpoch` work on the
separate E2 branch was not imported and is not covered by this finding.

The registry therefore remains unchanged: ME-014 is still preparation-only,
with no economic run authorization. This result narrows one design question; it
does not remove the registered payment, evidence, source-window, valuation, or
fixed-protocol gates.

## Independent critique

A fresh read-only Sol-6 medium critic reviewed the core fixture and relevant
source on the pinned tree. That review preceded the later runner-integration
test; it agreed that the **narrow API-composability claim survives**, while
identifying these limitations:

- The fixture chooses the underlying spot midpoint and assigns it as both the
  perp index and mark. Production `UpdatePerpPrices` may resolve a declared
  index and calculate a different mark. For example, a local spot midpoint of
  100 and a configured perp mark of 104 are not equivalent risk decisions.
- `UpdateMarkReferences` accepts the supplied values; `CommitMarkEpoch`
  snapshots available marks; and `MarkedAccount` checks those stored values.
  Their agreement proves plumbing consistency, not independent source or mark
  correctness.
- The positive-price guard is specific to the ABC/CDF-vs-USD fixture. It is not
  a general rule for every instrument price domain.
- Epoch commits are per venue, not a cross-venue atomic transaction. The test
  does not exercise a later venue commit failure or concurrent state mutation
  between public calls. A production composition therefore needs a serialized,
  exclusive phase owner and a prevalidated complete venue roster; otherwise
  the fixture's no-interleaving assumption is unsupported.
- `BeforeTimestampPhase` runs before venue jobs and ingress at the step
  boundary, but that hook alone does not establish the complete `t⁻` frontier:
  scheduler callbacks stamped at the boundary may already have queued work,
  and earlier off-grid deliveries may still be queued. The runner source
  requires `DrainIntermediateEvents` with its scheduled deterministic clock to
  drain those earlier events; concurrent clock advances or scheduler insertions
  must also be excluded. This fixture has no scheduler and tests none of those
  timing conditions.

The critique did not review funding economics or approve ME-014.

## Validation and limits

Observed checks on the pinned source:

- `GOMAXPROCS=2 go test ./research/me014composition -count=1` — passed.
- `GOMAXPROCS=2 go test -race ./research/me014composition ./exchange ./simulation -run 'TestConsumerCanCompose|TestUnavailableVenue|TestRunnerStopsBeforeVenue|TestRunnerStop|TestBeforeTimestampPhase' -count=1` — passed.
- `GOMAXPROCS=2 go vet ./research/me014composition` — passed.

The captured full `make test` attempt returned exit code 2. `go test ./...` and
the integrated-long-run contract, R2 contract, and non-R2 archive scripts
passed. The final R2 archive/parity script rejected the dirty worktree because
its parity gate requires a clean checkout; the new fixture and note were still
untracked at that point. This is a failed gate attempt, not a pass. The full
gate must be repeated from the committed clean candidate before promotion.

Not tested: production mark/index derivation; funding calculations or posting;
fractional remainder and finite reserve accounting; live settlement positions;
mixed margined instruments; atomicity of a multi-venue coordinator; concurrent
market mutations; production runner/evidence integration; terminal valuation;
or any economic outcome. No market world or holdout was run or read.

Therefore the test does not close any ME-014 economic gate. It supports a
design option for an external consumer-owned E2 coordinator on the current
public API, conditional on specifying and independently validating the mark
source and guaranteeing exclusive phase ownership. The E2 payment/evidence
integration and fixed protocol remain separate readiness work.
