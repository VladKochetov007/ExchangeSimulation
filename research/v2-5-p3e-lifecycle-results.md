# V2-5 P3e — passive-exit lifecycle causal result

Status: **SUPPORTED (screening), narrowly for the finite-term execution
contract.** Both preregistered seed pairs exercised the sub-minimum aggressive
exit condition. A closed no eligible terms; B admitted an ordinary passive
child for every eligible term and independently proved every term closed by
the registered deadline. This is not a funding-anchor, basis, profitability,
stability, liquidity-realism, or broader-market result.

The immutable protocol is
[`v2-5-p3e-lifecycle-preregistration.md`](v2-5-p3e-lifecycle-preregistration.md).
The compact machine record is
[`lifecycle-verdict.json`](artifacts/v2-5-p3e/lifecycle-verdict.json). Raw logs,
sidecars, and all ten extracted metrics remain retained and uncommitted under
`research/artifacts/v2-5-p3e/lifecycle-{A,B}-{107,109}/`.

## Provenance and evidence gate

All four cells used 98 simulated hours, full evidence, `GOMAXPROCS=4`, and
simulator source `4ef83610266e68981f5ea0e4df334e714c2e62f7`. The simulator,
final analyzer, and prune-gate SHA-256 values are respectively
`70303065…9805`, `a7cdaa86…2470c`, and `3ca77f0f…ac13`. The final analysis
revision is `9b21903d4aa3902477943059d489269a073984ea`.

Each run manifest records `modified=true` solely because the shared worktree
contained the four preserved user-owned
`research/artifacts/scoreboard/f2_baseline_101/{derivatives,exposure,reaction,streamhash}.json`
edits. There was no tracked simulator, analyzer, experiment-config, or script
change relative to the recorded simulation revision at launch.

The structural config guard passes: within each seed, removing only B's
declared `term_carry_allocator.passive_exit` leaves the complete A/B economic
configuration identical; experiment descriptions and IDs are provenance
metadata. The registered slice is 100,000 and the common B deadline/A cutoff
is `1736038805000000000`.

| cell | execution observations / ordered hash | exact evidence records / artifact digest |
| --- | --- | --- |
| A/107 | 50,614,472 / `82df4a9e…f85c` | 51,143,681 / `e1525e3a…4867` |
| B/107 | 50,636,477 / `cfe9bc22…b680` | 51,165,698 / `041483b1…6e24` |
| A/109 | 50,971,660 / `f86f4086…e181` | 51,500,869 / `62244ad7…2654` |
| B/109 | 51,011,869 / `bb6df851…88d3` | 51,541,090 / `da7560eb…b059` |

Every final `greeks.json` and `latency.json` is nonempty. Each extractor exited
zero and wrote the lifecycle, receipt, term-carry, order, position, derivative,
conservation, generic lifecycle, stream-hash, and exact-artifact metrics plus
analysis metadata. Runtime and offline exact-artifact event counts and digests
match in every cell. There are zero receipt-frontier, canonical-chain,
conservation, terminal-position, post-close-funding, or lifecycle integrity
failures.

The historical term-carry audit intentionally calls a legacy A settlement
after `term_end` “outside term.” The lifecycle gate accepts neither that label
nor a numeric exception by itself: for each A cell, the sole historical check
is exactly one such settlement and the independent lifecycle replay separately
reconstructs exactly one nonzero south residual-before-deadline settlement.
Any count mismatch, other historical failure, unknown arm, or B outside-term
settlement remains fail-closed.

## Registered paired score

| seed | arm | eligible | proven closed | closure fraction | all closed by deadline | passive endpoint / filled quantity | terminal residual magnitude | residual funding settlements / quote delta |
| ---: | --- | ---: | ---: | ---: | --- | --- | ---: | ---: |
| 107 | A | 2 | 0 | 0.0 | no | `not_applicable` | 40,000,000 | 1 / +150,014 |
| 107 | B | 2 | 2 | 1.0 | yes | observed / 200,000 | 0 | 1 / +49,495 |
| 109 | A | 2 | 0 | 0.0 | no | `not_applicable` | 40,000,000 | 1 / +150,014 |
| 109 | B | 2 | 2 | 1.0 | yes | observed / 200,000 | 0 | 1 / +150,014 |

Thus each seed has B−A closure fraction `+1.0`, proven-closed terms `+2`,
and terminal residual magnitude `−40,000,000`. Eligible-term count is unchanged.
Residual funding count changes by zero: the south funding instant occurs before
the eventual close in both arms. Its quote-delta effect is −100,519 in seed 107
and zero in seed 109. A has no observed first reduction or flatness by terminal
censoring, so a numeric time subtraction is not defined; B's finite times are
reported directly rather than treating A absence as zero.

## Per-term treatment endpoints

Every B term submitted and canonically admitted one 100,000-unit ordinary
post-only child. Each child had observed partial-filled quantity zero, then one
observed full 100,000 fill; no cancellation was applicable because all filled.

| seed / venue | canonical resting time | time from term end to first reduction | time from term end to flat | later close transitions | deadline / terminal residual |
| --- | ---: | ---: | ---: | ---: | ---: |
| 107 / central | 19 s | 21 s | 28 s | exactly 1 | 0 / 0 |
| 107 / south | 1 s | 3 s | 10 s | exactly 1 | 0 / 0 |
| 109 / central | 23 s | 25 s | 32 s | exactly 1 | 0 / 0 |
| 109 / south | 19 s | 21 s | 28 s | exactly 1 | 0 / 0 |

For every B term, independently reconstructed spot and perpetual positions
became zero before the one later named `TERM_CLOSED` transition, both events
preceded the deadline, no later event mutated the term, and no funding was
attributed after close. Each A term retained spot `+10,000,000` and perpetual
`−10,000,000` at the cutoff and terminal observation, with zero close
transitions.

## Analyzer repair audit trail

The first complete extraction exposed two offline analysis defects; neither
was a simulator, economic-policy, or persisted-evidence defect. The simulations
were not rerun.

- `05a1ffa` replaced timestamp-only decision position comparison with an
  explicit venue/client/order/trade identity link to the actor fill receipt,
  using ordinal only between decision and receipt in their same persisted log.
  Cross-file traversal order is never causal evidence.
- `9b4b99e` added the same-timestamp regression with canonical fills in a
  separately scanned file, ordered before the decision/outcome log.
- `9b21903` added the arm-specific cross-analyzer residual reconciliation and
  adversarial mismatch/unrelated-failure fixtures without changing the
  historical audit's P3/P3c meaning.

All four immutable raw cells were fully re-extracted after the final repair.

## Verdict and continuation boundary

The preregistered rule yields **SUPPORTED (screening)**: both exercised seed
pairs have a positive B−A closure effect, and B proves actual closure. P3e is
therefore promoted only as the development finite-term execution contract.
The result does not validate the 12-interval funding-persistence belief,
positive net carry after all costs, a funding-induced inventory change, basis
dynamics, price stability, liquidity realism, dated convergence, or robustness
beyond these fixed development seeds.

The only permitted continuation is a separately preregistered funding/carry
causal screen whose observable chain is delivered premium/funding, independent
expected-funding arithmetic, exact costed net carry, changed target inventory,
actual submitted and filled spot/perpetual orders, then basis dynamics. Any
missing link is `NOT IDENTIFIED`; funding must never write a target market
price. Dated carry remains a separate time-to-expiry/settlement protocol.
