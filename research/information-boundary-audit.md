# Information boundaries and look-ahead

What each participant class can see, and whether any of it is information a
real counterpart could not have. Established by reading the construction and
subscription code rather than by running anything, so it is a statement about
what the code permits, not a measurement of what was used.

## Information sets

Every class receives market data through a gateway, and every gateway with a
configured latency profile delivers through the scheduler at or after the
publication instant. No class subscribes to a feed of future events, and the
scheduler cannot deliver a message before the timestamp it was scheduled for,
so there is no look-ahead in the delivery path.

| class | subscribes to | reads directly, outside the message path |
|---|---|---|
| Stoikov makers (spot, cross, perpetual, dated) | own book snapshots; reference-symbol trades; the venue index when anchoring is not `own_mid` | — |
| fixed-distance and imbalance makers | own book snapshots | — |
| option dealers | underlying book snapshots; the reference-data feed | — |
| option value takers | underlying book snapshots; each listed option's book; the reference-data feed | — |
| vanna-volga desks | underlying book snapshots; the reference-data feed | **the dealer's live option inventory** (see IB-1) |
| triangular desks | the three spot books' snapshots | — |
| carry, dated-carry and parity desks | the relevant books' snapshots and the reference-data feed | — |
| uninformed, option and future flow | book snapshots for the books they trade | — |
| elastic suppliers | own book snapshots | — |
| execution desks | own book snapshots | — |

## IB-1 — the vanna-volga desk reads the dealer's inventory directly

`simulations/multivenue/sim.go:1822` wires the desk's exposure source to
`dealer.Exposures`, a method on the dealer object. The desk therefore knows
every option position the dealer holds, at the instant it asks, with no
message, no subscription and no latency — including under configurations where
every other channel between those two participants is delayed.

Whether this is legitimate depends on what the desk is meant to be. As a desk
hired to lay off a specific dealer's book it is defensible: a real risk-transfer
desk is shown the inventory it is quoting against. As an independent
relative-value participant it is not, and any measurement of how quickly
second-order risk moves between the two participants inherits the shortcut.

It also makes the pre-registered `abl-vanna-volga-off` arm easier to
interpret than it first appears: whatever that arm changes cannot be attributed
to the desk *discovering* the dealer's risk, because it never had to.

**Not withdrawn, but qualified:** any claim that vanna-volga hedging
demonstrates risk transfer through the market. The risk is transferred through
a function call and only the resulting trades go through the market.

## IB-2 — the index is a zero-latency cross-venue aggregate

Recorded in full as V-004. One provider aggregates all three venues' midpoints
within the same automation tick and each venue publishes the result; every
Stoikov maker anchors thirty percent of its reservation price to it. This is
not look-ahead — the aggregate is of the same instant, not a later one — but it
is instantaneous cross-venue information that no real participant would have,
and it is what holds the venues' prices together in the absence of any
cross-venue arbitrageur.

## IB-3 — index ordering inside a tick is unspecified

The provider is updated by each venue's post-mark hook and read by each venue's
index publication in the same tick. Whether a given venue's published index
already contains another venue's midpoint from the same tick depends on the
order the hooks run in. The order is fixed by the venue list, so it is
deterministic, but it is not stated anywhere as a modelling decision, and it
means "the index" is not the same object at all three venues within a tick.

## What is not established here

- **No mutation test has been run.** The plan asks for future information to be
  injected into one actor deliberately, to prove the audit would catch it. That
  requires execution and has not been done, so this document says what the code
  permits and not what a detector would find.
- **No per-observation timestamp check.** The plan asks that every observation
  used in a decision satisfy `information_timestamp <= decision_timestamp`.
  The delivery path makes this structurally true for market data, but nothing
  instruments it, and IB-1 bypasses the path entirely.
- **Liquidation and reference pricing** were not audited for their information
  sources, because neither ever runs in the frozen baseline (V-005).
