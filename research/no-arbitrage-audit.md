# No-arbitrage audit of the frozen baseline

Frozen commit `6bab42af38a7`, config `research/configs/frozen-baseline-2026-08-21.json`,
seed 101. The auditor is `mvanalyze -metric arbitrage`: an omniscient observer
that is not a participant, sees every venue's touch at once, pays the taker fee
on every leg and never queues. An edge it cannot find does not exist for
anybody; an edge it finds is a bug only if nothing ever closes it.

## Cycles searched

| cycle | result |
|---|---|
| cross-venue, same asset (ABC/USD across three venues) | no profitable instant in any pair; mean edge −4.2 to −4.8 bps, i.e. inside the two-leg fee |
| spot triangular (ABC/USD, CDF/USD, ABC/CDF at one venue) | **profitable in 86-89% of instants** at every venue, mean +284 bps over 30 minutes, positive without interruption for 1,484-1,582 of the 1,797 measured seconds |
| perpetual carry, calendar, put-call parity, settlement-boundary, tick-rounding | not yet searched |

## V-001 — the cross book is unanchored and its dislocation compounds

**What was measured.** The traded ABC/CDF price against the rate implied by
ABC/USD and CDF/USD at the same venue, bucketed and compared over a run.

The first version of this audit reported 99.4 percent of instants and +705 bps.
Those figures were wrong: the auditor evaluated cycles inside a concurrent
scan, so it priced some instants against quotes from later in the run, and
counted an instant once per publishing book. Rewritten to collect each book's
quote series and evaluate them in one time-ordered pass, it reports the numbers
above. The conclusion is unchanged and the numbers are not.

| elapsed | deviation |
|---|---|
| first minute | −1 bp |
| 5 minutes | −25 bps |
| 10 minutes | −111 bps |
| 20 minutes | −358 bps |
| 30 minutes | −695 bps |
| 24 hours | **+59,540 bps** (the cross ends 6.9× away from implied) |

The same figure appears at all three venues to within 30 bps of each other,
so it is a property of the population rather than of one book's luck.

**Mechanism.** Each spot book's maker is a Stoikov quoter whose reservation
price is its own book's midpoint shifted against its own inventory. For
ABC/USD and CDF/USD that midpoint is tethered by participants who care about
the level — elastic suppliers with a downward-sloping demand curve, carry desks
against the perpetual. Nothing plays that role on ABC/CDF. Its maker's
reservation price is therefore a random walk in the level, and the walk is
self-reinforcing: as the maker accumulates inventory its quotes shift, when the
shift exceeds the spread its own requote crosses the standing book, the print
moves its midpoint further in the same direction, and the next reservation
price starts from there. Measured over thirty minutes the cross maker was the
aggressor on 40 of the 42 ABC it took, all on one side.

**Why the arbitrageur does not stop it.** The triangular desk is present,
active, and correctly signed: it fired the profitable direction 2,584 times in
thirty minutes, buying the cheap cross and selling the base. It cannot restrain
the drift because its size is a constant — `lot_qty` of 0.05 ABC per firing,
independent of the edge. It traded 127 ABC into a book that moved 700 bps
against a 42 ABC one-sided maker flow. A desk whose size does not scale with
the opportunity is not an arbitrageur in the economic sense; it is a fixed
trickle.

**Classification.** Category 2 shading into category 3 of the plan's taxonomy:
the arbitrage is not guaranteed by an accounting inconsistency, and it is not
merely limited by capital either — the desk has capital and does not use more
of it. The dislocation is unbounded, which no capital-constrained equilibrium
produces.

**What this withdraws.**

- Any claim that the three spot books form an economically coherent triangle.
- Any claim that the triangular arbitrageur links them. It trades against the
  dislocation and loses the race by two orders of magnitude.
- The economic meaning of every ABC/CDF price in the frozen baseline, and of
  any statistic computed from that book: its returns are the output of an
  unanchored feedback loop, not of a market.
- The liveness result for ABC/CDF specifically. The book stays two-sided and
  traded, which is exactly why liveness alone is not evidence of a market.

**What it does not withdraw.** ABC/USD, CDF/USD, the perpetual and the
derivative books are not implicated by this measurement. Cross-venue pricing of
the same asset is arbitrage-free to within fees, which is a genuine pass.

**Follow-ups, none of them tuning the frozen baseline.**

1. Measure whether ABC/USD drifts the same way over a long run, with the
   elastic supplier ablated. That is the pre-registered test of the mechanism.
2. Measure the triangular desk's realised profit per firing against the quoted
   edge, to see whether it is even harvesting the edge it fires on.
3. Search the remaining cycles: perpetual carry, calendar, parity, and the
   settlement boundary.
