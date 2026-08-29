# V2 integrated long-run r5 review record

Date: 2026-08-29 UTC
Status: hardening in progress; no r5 cell or holdout has run

## Independent gate result

The fresh Sol-xhigh adversarial review rejected the pre-hardening candidate.
The rejection was accepted as a hard scientific gate. It identified nine
fail-open paths: mutable derived evidence, an unenforced dated-future net
balance, unbound realized/carry cash, a declarative-only horizon, missing
listing-before-use order, post-observation analyzer changes, unverifiable
parity raw evidence, and holdout-directory/policy gaps.

## Implemented responses

The successor namespace remains `/home/vlad/v2-integrated-longrun-candidate-20260828-v5`.
The runner now records the exact registered 24-hour endpoints, verifies
terminal account timestamps and strictly increasing checkpoints, seals fixed
sidecars plus all raw venue JSONL files, and creates a one-time external
attestation binding the evidence manifest to completion status. The no-log
parity control has an explicit zero-raw-file manifest contract.

The extractor revalidates the raw manifest and attestation, recomputes the
derived evidence digest, requires one frozen source revision, and validates
the same horizon contract. The scorer invokes a fresh extractor in a temporary
directory and compares every derived JSON object canonically before reading
the stored score. It rejects any pre-existing holdout output directory.

The exact position replay now binds nonzero transitions to `realized_pnl`
records. Settlement rejects nonzero net dated-future supply. Position,
settlement, and expiry audits enforce listing-before-use, and the checkpoint
sink closes at the registered final simulation time. Option terminal and
payout timestamps are now bound to declared expiry as well. Adversarial unit
tests cover each newly executable mutation.

## Remaining gate sequence

Run focused tests, shell contract tests, and `make test`; commit this
hardening step; obtain a new independent Sol-xhigh review; and proceed only
on ACCEPT or ACCEPT WITH NARROWER CLAIM. Then build clean Go 1.27 binaries in
a separate worktree, run `dev-607`, review it independently, and only then
run `dev-613`, `dev-617`, and the registered 607 G4/G8/no-log controls. Holdout
619/631/641 remain unread and unauthorized until a separately committed freeze
authorization artifact exists.

Historical r4 evidence and all earlier blocked lines remain preserved and are
not reopened by this amendment.
