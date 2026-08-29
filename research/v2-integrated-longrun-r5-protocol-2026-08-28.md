# V2 integrated long-run r5 protocol amendment

Date: 2026-08-28
Status: implementation and review gate; no freeze or holdout authorization

## Reason for the successor namespace

The clean r4 development cell 607 completed its simulator run and all
mechanism checks except the r4 position and settlement predicates. The raw
evidence and r4 derived artifacts remain immutable under
`/home/vlad/v2-integrated-longrun-candidate-20260828-v4`.

The r4 failure was localized, not repaired by tolerance: its public-entry
auditor reported an open-value gap of 908 fixed cash units and 33 settlement
mismatches. An analysis-only exact replay of the same raw stream reconstructed
1,171,190 trade transitions with zero replay failures, zero settlement-event
mismatches, and zero settlement residual. The representative independent
falsifier was north/client 10: public-entry arithmetic gave 12,699,264,512,
while the exact logged settlement was 12,699,264,523. The +11 difference is
the aggregate-basis rounding effect exposed by the r4 auditor.

Newton and Maxwell independently classified r4 as a progression block caused
by the auditor representation, not evidence of a simulator mechanics
regression. Neither review authorized reusing r4 as a qualified gate.

## r5 changes

The successor keeps the same registered economic configurations and simulator
parameters. It changes only evidence observability and the fail-closed audit:

* every linear position update records `position_side` and `base_precision`;
* every expiry settlement and expiry balance change records `position_side`;
* instrument lifecycle announcements record `quote_asset` and
  `base_precision`;
* the full integrated runner records explicit `instrument_listed` descriptors
  for its static spot/perpetual books as well as dynamically listed contracts;
* position and settlement analyzers replay the exact aggregate basis and
  carry lattice independently, keyed by venue/client/symbol/side;
* malformed links, missing exact fields, nonpositive precision, arithmetic
  overflow, wrong wallet/asset, and ambiguous records fail the metric;
* strict positions reconcile every nonzero linear terminal report row to one
  successful replay state and require a mark at or before the terminal account
  timestamp;
* strict settlements bind the immutable listing descriptor to the terminal
  descriptor, require the settlement action at expiry, require a same-file
  settlement-before-payment order, and reconstruct delivery fees through the
  registered `zero` policy (or an injected resolver);
* the expiry boundary is strict: records at `timestamp >= expiry` are late;
* the public rounded-entry formula remains a diagnostic (`display_formula_gap`)
  and is not used as the exact mechanics result.
* the runner records a manifest over every fixed sidecar and every retained raw
  venue JSONL file; an external sibling attestation cross-binds that manifest
  to completion status and binary/config provenance, and the scorer
  re-extracts every derived artifact from raw evidence before evaluating it.
  These are cross-bound integrity records, not cryptographic signatures or an
  external immutable ledger;
* settlement rejects nonzero net dated-future supply, and lifecycle audits
  reject position, mark, fill, snapshot, and settlement use before listing.
  Within one file, persisted sequence order is required in addition to
  simulated time; same-timestamp records in separate files are ambiguous and
  fail strict causality;
* the registered 24-hour start/end timestamps are required in the report,
  checkpoint stream, and completion status; the exact replay also binds each
  nonzero transition to the independent `realized_pnl` event stream, while
  carry residues remain covered by the position-rounding audit;
* option terminal announcements and option expiry payouts are required at the
  declared option expiry, with timing violations emitted as an explicit
  derivative predicate failure rather than absorbed by an amount-only payoff
  check.
* strict derivative analysis requires funding-rate identity, a positive
  interval, a future `next_funding`, and a rate record physically before the
  funding ledger boundary. Each funding settlement must match that exact
  announced instant. Shifted, backdated, malformed, overflowed, or silently
  undecodable derivative records are counted as failures rather than discarded.

The exact replay is not a realism score. The accepted claim, if the r5 gate
passes, is limited to cross-bound retained-evidence integrity, exact
position/basis and settlement mechanics, lifecycle behavior, and registered
activation predicates. It does not license claims about market realism, price
discovery, funding anchoring, endogenous option shape, liquidation
reachability, or inactive P3/P4/P5 recorders.

## Registered r5 execution order

1. Commit the complete amendment after `make test` plus the shell contract
   tests pass.
2. Obtain an independent Sol-xhigh review of that committed amendment.
3. Build simulator, analyzer, and prune-gate binaries from one clean,
   provenance-pinned Go 1.27 worktree with `-trimpath`, `CGO_ENABLED=0`, and
   `vcs.modified=false`.
4. Run only `dev-607` in the new v5 root, extract it, and independently review
   the exact output.
5. After that review accepts the dev-607 gate, run only registered `dev-613`
   and `dev-617`; also run the registered 607 G4/G8/no-log controls and parity
   check. No holdout directory is read or launched.
6. A separate freeze authorization artifact must precede any use of holdouts
   619, 631, or 641.

The r5 namespace is `/home/vlad/v2-integrated-longrun-candidate-20260828-v5`.
The updated scripts carry runner v5, candidate v5, parity v3, and scorer v4
identities. The old r4 scripts remain recoverable at commit `16a5e91`.

The v4 raw tree was archived before cleanup at
`/home/vlad/v2-integrated-longrun-candidate-20260828-v4-preserved.tar.zst`.
Its SHA-256 is
`cd33387f1c21aed61e91c8bc86befc65521ccc22e82f137ea177b173334c0793`;
the archive was tested before the live duplicate was removed. No holdout was
read or launched during this amendment.
