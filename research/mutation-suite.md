# Mutation suite

Section 15 asks for deliberately broken variants of the simulator, each paired
with the invariant that must fail. A mutation that passes means the audit is
too weak and has to be strengthened — that is the point of the exercise, and it
is why each row below names a specific detector rather than "the audit".

Two mutations have been run. The rest are specified here with their expected
detector so that executing them is mechanical.

## Method

Each mutation is a one-line edit to the engine, built to a scratch binary,
never committed:

    cp exchange/<file>.go /tmp/orig.go
    # apply the edit
    go build -o /tmp/mv_mutant ./cmd/multivenue
    cp /tmp/orig.go exchange/<file>.go     # revert before anything else
    GOMAXPROCS=1 /tmp/mv_mutant -config research/configs/frozen-baseline-2026-08-21.json \
      -seed 101 -duration 2h -logdir /tmp/mut_<name>
    ./bin/mvanalyze -metric <detector> /tmp/mut_<name>

Revert immediately after building. A mutant binary must never be produced from
a tree that stays mutated, and no mutation may be committed.

## Run

| mutation | detector | result |
|---|---|---|
| Credit 1000 extra units on ~0.1% of spot settlements | closed-system identity, per asset | **caught.** ABC and CDF residuals moved from exactly 0 to 41,726,000 and 24,702,000; USD from 4,248 to 31,624,727 |
| Move venue revenue without recording it (the pre-fix state of the engine) | venue take against its own movement stream | **caught after the fix.** Before it, the audit read the take from the report and could not see the 562,254 ABC and 17,232,038 USD that no event accounted for |

## Specified, not yet run

| mutation | invariant that must fail | detector |
|---|---|---|
| Reverse the funding sign | the side that pays must be the side the published rate says pays | `-metric derivatives`, funding direction consistency |
| Charge funding twice | each instant nets to zero within one unit per account | `-metric derivatives`, funding residual |
| Duplicate one fill | movements reconstruct the reported holdings; contract net size stays zero | `-metric conservation` chain check; `-metric positions` |
| Delete one fill | the same two | as above |
| Violate price-time priority | no accounting invariant catches this — **the audit is currently blind.** It needs a queue-order check over the book delta stream | none yet; this row is the strongest argument for building one |
| Omit one settlement | payout residual per contract; holders paid against holders present | `-metric settlements` |
| Settle an option at the wrong strike | payout equals intrinsic value at the published settlement price | `-metric derivatives`, exercise residual |
| Swap call and put payoffs | the same, and out-of-the-money contracts paying nothing | `-metric derivatives`, `worthless_paid` |
| Wrong sign on the Black-76 delta | dealer net delta grows without bound instead of being hedged back | `-metric hedging`, buy share and net delta drift |
| Ignore the option multiplier | payout equals intrinsic times position | `-metric derivatives`, exercise residual |
| Execute an order after expiry | no fill may be recorded after the expiry instant | `-metric settlements`, `TradesAfterExpiry` |
| Fail to cancel expired resting orders | the same, plus the book still quoting a delisted contract | `-metric settlements`; needs a delisting check that does not exist yet |
| Double-count a fee | venue take against the fee stream; closed-system identity | `-metric conservation` |
| Drop a cancellation | resting order count grows without bound; no current detector | none yet |
| Give one venue zero latency accidentally | cross-venue edge appears where none did | `-metric arbitrage`, cross-venue cycle |
| Use stale collateral for liquidation | cannot be tested: liquidation never fires (V-005) | blocked on the stress arm |
| Inject future information into one actor | no detector exists; the delivery path makes look-ahead structurally impossible but nothing instruments it (see `research/information-boundary-audit.md`) | none yet |

## What the table already says about the audit

Three classes of defect have **no detector at all**: matching-priority
violations, dropped cancellations, and injected look-ahead. Two more are
blocked behind mechanisms that never execute. So the audit as it stands covers
money and lifecycle thoroughly and covers matching, order handling and
information flow barely.

That is a statement about the audit rather than about the simulator, and it
belongs in the same document as the passes: an invariant suite is only as
strong as the mutations it can catch, and this one has not yet been shown to
catch the ones that matter most to a matching engine.
