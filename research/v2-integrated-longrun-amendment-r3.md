# Integrated V2 long-run candidate: post-r2 amendment

Date: 2026-08-28  
Amendment: `v2-integrated-longrun-candidate-v3`  
Parent contract: `v2-integrated-longrun-candidate-v2`

## Status and boundary

This is a versioned amendment to the pre-run candidate protocol. It is not a
rewrite of the parent contract and it does not change the development/holdout
partition, seeds, horizon, economic configuration, or conservation tolerance.
The parent r2 evidence remains immutable failed evidence. It is never
retroactively promoted to PASS and is not included in a v3 score.

The r2 run at
`/home/vlad/v2-integrated-longrun-candidate-20260827-r2/dev-607` remains
preserved because its fail-closed extraction reported failed conservation,
maker-refresh, and hedging predicates. Its raw logs, derived artifacts, and
failed-attempt records remain historical evidence; no v3 analyzer result may
overwrite them.

Holdout identities 619, 631, and 641 remain reserved and unread. This
amendment authorizes no holdout activity. Development remains limited to full
cells 607, 613, and 617 plus the registered seed-607 full/no-log/G8 parity
controls.

## Evidence-schema correction

The venue now emits a canonical `OrderCancelled` record whenever exchange
mechanics remove a resting order without an actor cancel request. The payload
contains `order_id`, `remaining_qty`, and one exact reason from this closed
allowlist:

* `EXCHANGE_FORCED_FEE_RESERVATION`
* `EXCHANGE_FORCED_LIFECYCLE`
* `EXCHANGE_FORCED_SELF_TRADE_PREVENTION`
* `EXCHANGE_FORCED_BOOK_ADMISSION`

These records intentionally omit `request_id`; an exchange action is not
relabelled as an actor request. The scorer accepts a forced cancellation only
when the reason is exactly allowlisted, `request_id` is absent, the quantity
matches the tracked order remainder, and its physical record ordinal precedes
the replacement acceptance/rejection record in the same canonical book file.
Unknown `EXCHANGE_FORCED_*` reasons, allowlisted reasons carrying
`request_id`, malformed quantities, and post-replacement cancellation order
are integrity failures. Ordinary actor cancellations retain request-based
ordering checks. The correction covers fee-reservation failure, fee-remainder
failure, lifecycle/expiry/liquidation, cancel-all, self-trade prevention, and
book-admission removal paths.

## Hedging-symbol correction

The hedging analyzer uses an explicit `-hedge-symbol` flag, defaulting to
`ABC/USD`, the underlying symbol written by the V2 event stream. The existing
`-base` flag remains the default `ABC-USD` triangle-book selector. For legacy
hedging invocations that explicitly supplied `-base` but not `-hedge-symbol`,
the explicit base is accepted as a compatibility alias. When both are
explicit, `-hedge-symbol` wins. Tests cover the default, new flag, legacy
alias, and precedence.

## Provenance and rerun authorization

The runner, extractor, and development scorer use the v3 contract identifiers
(`v2-integrated-longrun-runner-v3`, `v2-integrated-longrun-candidate-v3`, and
`v2-integrated-longrun-scorer-v2`). A fresh
clean simulator and analyzer build is required after this amendment, with
`trimpath`, `CGO_ENABLED=0`, clean embedded VCS metadata, and recorded SHA-256
identities. These settings and the Go toolchain version are recorded in the
immutable run and analysis metadata and carried into the write-once score. The
simulator source identity and analyzer source identity are recorded
separately; no analyzer correction is applied retroactively to r2.

The parity attestation independently requires schema 3, runner-v3, clean
VCS/build identity, `trimpath=true`, and `CGO_ENABLED=0` on the full-G4,
no-log-G4, and full-G8 controls; a legacy runner-v2 control cannot satisfy a
v3 parity claim. The scorer additionally verifies that each development
`integrity.json` and `activation.json` carries the candidate-v3 contract
inside the artifact itself, rather than trusting analysis metadata alone.

Before any v3 development cell is launched, an independent Sol-xhigh review
must ACCEPT the combined source correction, exact forced-cancellation schema,
fail-closed scorer, hedging compatibility rule, v3 extractor, and v2 scorer.
The clean-build manifest and review decision are part of the gate record.

The parent conservation rule remains `abs(residual) <= 1000` fixed-point report
units independently for global and per-venue identities. This amendment does
not relax that bound after seeing r2. The preserved r2 residual is a reason to
measure the corrected clean rerun, not permission to tune the threshold.

## Required post-amendment checks

1. Run the full mandatory test suite and shell contract tests from a clean
   worktree.
2. Rebuild the simulator and analyzer in a separate clean worktree and verify
   hashes, toolchain, revision, and `vcs.modified=false`.
3. Run only registered development cells 607, 613, and 617 and the seed-607
   full/no-log/G8 controls. Do not access holdouts.
4. Extract all registered metrics sequentially under the bounded resource
   policy. Preserve raw and derived evidence until every contract passes.
5. Require independent review at the development result gate before the
   write-once v2 scorer is authorized.

The v3 scorer still licenses only an immutable freeze bundle. It is not a
market-realism claim, and it cannot authorize holdout validation by itself.
