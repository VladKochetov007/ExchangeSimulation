# V2-5 P3d — invalid exit-liquidity attempt (seed 107)

Status: **INVALID ATTEMPT — NOT SCORED.** P3d does not supply a causal
finding about an exit-only materiality policy, funding, carry, basis, or
realism. Its raw evidence is retained and is not eligible for pruning.

## Registered question

P3d registered a paired 98-hour seed-107 screen:

| arm | `unwind_min_order_size` | registered interpretation |
| --- | ---: | --- |
| A | 100,000 | inherited entry/exit floor |
| B | 0 | no actor-specific exit floor above the alleged exchange positive-unit minimum |

The original design incorrectly asserted that the ABC spot/perpetual venue
admitted every positive quantity. In fact the V2 venue creates both instruments
with `MinOrderSize = mvBasePrecision / 1000 = 100,000` raw units
([`sim.go`](../simulations/multivenue/sim.go)). The P3 policy value happened to
match that venue rule; it was not an independent actor-only floor.

## Completion and immutable evidence

Both full worlds completed using the final sidecar sentinel (`greeks.json` and
`latency.json`); neither host-process state nor a partial JSONL file was used
as completion evidence. The full raw directories remain:

| arm | config SHA-256 | final execution observations / ordered hash | persisted records / unordered artifact digest |
| --- | --- | --- | --- |
| A | `cbb18d86752b59be3000998f8832851305f29f5cd2c440190d71039f9658ca76` | 50,614,472 / `a38aca12e63cf12f70be86cecc463d92a487ff25fbc8d143d5f70630c8236dbe` | 51,143,681 / `35fcd3cc5a391741c4cee1dfd22e169b1cf5ae359192d8b8d9b8f88faa331b83` |
| B | `3b5844597b5f89651888f4433183a118c4a842a6adf43dea903d37d86a97ed37` | 50,621,670 / `9e912f98fee7d41c290565c54b13bc8d937f8a41a4c0d536414e70a0a53e2821` | 51,158,077 / `8c69eccf93e406dba76f99152100a27cc490ed58e50834c24288af6a21da25db` |

The worlds were built from `a14cfe0b6bb0b903ce91dc44930be5d4ee4316b2`
with a modified working tree containing only the known user-owned frozen
scoreboard extracts. The simulator/analyzer/prune-gate binaries used for the
full worlds were respectively
`6e8877c1c8b16668d1297c43cdc8dd001740251dc088ce66fe0e3dde73c88053`,
`64ceeed2ba009287411329e12d5b472c9cc17fa8716534b13f57843cd602169b`,
and `3158d8de73111e61cd3d1ab3fdfb599662692d36318d5aae8023ab34e804636f`.

## Falsification of the experiment’s activation contract

Arm A reproduced the pre-existing blockage: two active terms, no close, 7,200
`EXECUTABLE_SIZE_UNAVAILABLE` decisions, and no submitted unwind child. Its
term replay is invalid only because an already-expired term remained exposed
through one later funding settlement; its source/frontier, gateway, venue,
actor, arithmetic, lifecycle, and position-continuity counters are zero.

Arm B formed the same two active terms but emitted 7,200
`SUBMIT_UNWIND_PERP_IOC` decisions. The first central and south children were
respectively 16,286 and 16,348 units at their locally delivered asks. The
exchange rejected them as `INVALID_QTY`, because both are below 100,000. Of
7,204 submitted decisions, 7,198 were rejected; the final two had no delivered
venue response before terminal censoring. Its first full replay (pre-hardening)
already failed; the replay rebuilt from `2cb51bf` independently reports all
7,200 unwind decisions as `submission_mismatch`, plus the same two missing
terminal outcomes. It never accepts a sub-minimum child as legal evidence.

Therefore B did not activate the registered intervention, and neither arm
passes the prerequisite lifecycle/evidence gate. The comparison is not
`SUPPORTED`, `MIXED`, or `FALSIFIED`: it is **INVALID / NOT SCORED** because
the registered treatment’s definition of exchange-admissible quantity was
wrong.

## Correction and regression protection

Two post-run V2 code corrections are deliberately separate from this historical
attempt:

1. `2cb51bf` makes an explicit v3 unwind floor an *additional* actor
   materiality floor. Its effective child minimum is never below the declared
   instrument minimum; the decision evidence retains the configured zero
   distinctly from numeric price availability.
2. `21d17ae` binds the P3 policy `min_order_size` to the current ABC venue
   instrument rule during configuration normalization. A divergent policy now
   fails before a simulated world is constructed.

Focused ordinary/race tests prove that explicit zero cannot generate a
sub-venue-minimum entry or unwind, that the independent replayer rejects a
forged sub-minimum v3 child, and that below- and above-venue configuration
values fail before simulation. These are V2 semantics corrections; they do not
alter any retained P3d trajectory.

## Consequence for V2-5

P3c and this invalid P3d attempt establish only that the current ecology has
insufficient displayed exit capacity for the registered 10m finite terms near
their end. They do not show that a hidden actor materiality rule caused the
failure. A future mechanism, if justified, must be designed separately around
entry-time exit-capacity/risk policy or an explicit participation-limited
execution contract. It must not bypass the venue minimum, invent a fill, or
reuse P3d's invalid zero-floor treatment.
