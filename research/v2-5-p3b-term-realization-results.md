# V2-5 P3b — first realized funding-term result

Status: **SUPPORTED (development screening), narrowly.** The installed P3 v2
term-carry allocator formed two independently reconstructed matched
spot/perpetual terms from delayed local public observations, remained active
through one ordinary funding instant, and received its persisted funding ledger
transfers. This single, nine-hour development cell does **not** establish that
funding anchors price, that the 12-interval persistence belief is realistic,
that net carry remains positive through exit, that basis converges, or that
the result is robust.

The immutable preregistration is
[`v2-5-p3b-term-realization-preregistration.md`](v2-5-p3b-term-realization-preregistration.md).
The machine-readable verdict is
[`term-realization-107-verdict.json`](artifacts/v2-5-p3b/term-realization-107-verdict.json).
Raw evidence and every extracted artifact remain retained under
`research/artifacts/v2-5-p3b/term-realization-107/`; this cell is not safe to
prune.

## Provenance

| item | value |
| --- | --- |
| config / SHA-256 | `configs/v2-5-p3b/term-realization-107.json` / `b58a21a1f9c2e77f2516a5e45736f3990790cf10ff0377cde45ac3166203cab3` |
| seed / horizon / process setting | 107 / 9 simulated hours / `GOMAXPROCS=4` |
| simulator revision recorded by manifest | `9bdb0b5132a14e1610ec72ab3f38d98098071f41` (manifest records a modified worktree; the P3 v2 implementation is committed at this revision) |
| simulator / analyzer / prune-gate SHA-256 | `648d2c977a6ae1360485b793f911a866c2e99ce9f1681e4298113e1451b0f881` / `e161bef701ef52f1e62da33b3b0ca302340070ea26ae2e545891868da496cfeb` / `fd6bd26afc6df91cc3ecae01c4ec04106714315e45a37c240a39785de5d5d9cc` |
| final execution observations / ordered hash | 4,925,694 / `da7e8b13bdcad482e22260252c420bd9187df80676f2a23b632ee8f65cf553a6` |
| persisted evidence records / multiset digest | 4,974,303 / `f84ab492b4522a9bbc0b80637b41506af4cb32b32ddf5f00b5e1a3bbdc6af218` |
| exact persisted JSON-record artifact digest | 4,974,303 / `d30a416469e52a02a2c5492dca238a18087c28e43402ebc98e82a5d4a84dc0f4` |

Both final-only completion sentinels (`greeks.json`, `latency.json`) are
nonempty. The exact config, manifest, checkpoints, full venue evidence, V2
receipt and decision sidecars, and every preregistered extracted metric are
retained.

## Registered evidence and activation gates

| check | result |
| --- | --- |
| P3 v2 term-carry replay | valid; 48,600 decisions, 4 submitted/accepted requests, 5 fills |
| realized first exposure | 0 mismatches between decision evidence and independent canonical-fill replay |
| ownership state at terminal | 2 active/open terms, 0 closed; terminal censorship is expected for a 96-hour commitment in a 9-hour world |
| active-term funding attribution | 2 settlements; 0 inactive or overlapping-term attributions |
| V2 receipt/frontier replay | valid; 291,582 schedules, 291,573 deliveries, 4 audited decisions; nine terminal undelivered schedules are due after the horizon |
| delivered latency | each allocator feed: 40 ms market data; entry request/response: 20 ms |
| balance delta replay | 306,081 rows checked; 0 delta mismatches, 0 broken chains, 0 decode failures |
| position replay | 0 disagreements, 0 non-zero net contracts, 0 unrepresentable open values |
| generic order lifecycle | 330,896 accepted, 180,774 fills, 239,868 cancellations; all registered error counters 0 |
| generic funding direction/duplication | 3 venue funding records, 0 broken/sign-wrong/misdirected/undirected/duplicate payments |

The two active P3 terms are at `central` and `south`; `north` did not form a
term. Both active terms were reconstructed as equal-and-opposite nonzero
spot/perpetual inventory before the funding instant. Their allocator account
(`client_id=8`) has independently preserved `funding_settlement` balance
changes at `1735718401000000000`:

| venue | symbol | P3 USD ledger delta | funding rate | contract-wide direction audit |
| --- | --- | ---: | ---: | --- |
| central | `ABC-PERP` | +150,014 | 3 raw rate units | long pays / short receives; 3 directed changes; residual -1 raw USD |
| south | `ABC-PERP` | +150,014 | 3 raw rate units | long pays / short receives; 3 directed changes; residual -1 raw USD |

The allocator was short perpetual in each matched cash-and-carry term, so
both positive ledger deltas have the independently reconstructed direction.
The one-unit venue residuals are the registered integer truncation residual;
the central and south funding records have no duplicate, misdirected, or
sign-wrong payment. `north` settled independently but has no active P3 term
and is not attributed to the allocator.

The full-world accounting identities retain an exact ABC residual and a USD
residual of -23 raw units (`2.27e-14` relative) with the recorded open linear
value of -300,000 USD units. This is bounded integer truncation in the
existing conservation contract, not an unaccounted P3 payment.

## Interpretation and next gate

This passes the P3b activation chain only:

```text
delayed local public observations
  -> named finite-term policy
  -> ordinary non-atomic fills
  -> first canonical exposure
  -> matched active inventory
  -> ordinary directed funding ledger transfer
```

It is a single enabled cell, not an A/B market comparison and not a
replication. There is no claim about a marginal effect of funding on basis,
price discovery, return, spreads, or realized net carry. The two terms remain
open by design, so neither close correctness nor exit-cost-adjusted economics
has been measured.

P3c is the next required lifecycle gate: preregister one full-evidence horizon
long enough to cross the declared 96-hour commitment and allow deterministic
ordinary exit retries. It must establish exactly one unwind, terminal flat
spot/perpetual ownership, and conservation before any paired market-level
funding/carry experiment is admissible.
