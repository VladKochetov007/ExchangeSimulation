# Causal ablations

Each arm states its prediction and kill criterion before the run, and reports
what happened including where the prediction was wrong.

## V-002 — what makes the second spot book run away

**Question.** CDF/USD rises by a factor of forty-five to forty-nine over
twenty-four hours, driven by its own market maker becoming the dominant net
taker buyer. Is the cause the absence of a participant who cares about the
book's level, or the maker's own inventory-skew rule?

**Arms.** 24 simulated hours, seeds 101 and 103, against the frozen baseline.

| arm | change |
|---|---|
| control | frozen baseline |
| A | price-elastic participants split between ABC/USD and CDF/USD |
| B | maker inventory skew switched off for every Stoikov quoter |

**Prediction, stated before running.** If the missing level-caring participant
is the cause, arm A stays inside 2× on every seed and arm B still runs away on
the seeds where the control does. If the quoting rule is the cause, the
reverse.

**Result.** CDF/USD terminal price divided by opening price:

| arm | seed 101 | seed 103 |
|---|---|---|
| control | **49.41×** | **45.00×** |
| A — elastic supplier on CDF/USD | 2.07× | 1.05× |
| B — inventory skew off | **1.00×** | **1.00×** |

**The prediction was wrong, and in an informative way.** It was framed as an
either/or and both arms suppress the runaway. The two are not competing
explanations; they are the two halves of one feedback loop. The inventory skew
is the amplifier — as the maker accumulates inventory its reservation price
shifts, and when the shift exceeds the half-spread its own requote crosses the
standing book, printing a new midpoint that moves the next reservation price
further in the same direction. The price-elastic participant is the damper: it
sells into a rise and buys into a fall, so the loop cannot run away while it is
present. Remove either the amplifier or the damper and the instability stops.

ABC/USD is unaffected in every arm (0.96× to 1.00×), which is what a book that
already has a damper should look like.

**Second effect, not predicted.** The cross book's collapse is also driven by
the skew. ABC/CDF ends at 0.15–0.17× in the control, 0.15–0.25× with the
elastic supplier added to CDF/USD, and 0.80× with the skew off. So V-001's
compounding triangular dislocation is downstream of the same amplifier rather
than of the cross book's own lack of an anchor.

**Confounds and limits.**

- Arm B switches the skew off at every venue and on every book, so it is not a
  clean intervention on CDF/USD alone. It is the closest lever the frozen
  configuration exposes without adding one, and it was labelled as suggestive
  before the run.
- Two seeds per arm. The effect sizes are large enough — 45× against 1.00× —
  that the direction is not in doubt, but the dispersion is not characterised.
- Removing the skew removes a real economic behaviour: a maker that does not
  skew against inventory carries unbounded risk. The arm shows what causes the
  instability, not what the fix should be.

**What this establishes.** The runaway is causally identified: it requires the
inventory-skew rule and the absence of level-caring demand together. It is a
property of the population's composition, not an accounting or matching defect.
