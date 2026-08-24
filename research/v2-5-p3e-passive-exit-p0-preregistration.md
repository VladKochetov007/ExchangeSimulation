# V2-5 P3e P0 — passive exit integrity and activation preregistration

Status: **preregistered before any P3e configuration run or P3e raw-evidence
inspection.** P3e follows P3c's valid lifecycle falsification: an owned
finite term reached its declared end but repeated locally observed contra
touches were below the venue's legal minimum. P3d's lower-floor premise was
invalid. P0 tests only whether the new, bounded passive policy can be
constructed and admitted through ordinary venue paths; it is not a funding,
basis, profitability, stability, or realism experiment.

## Fixed cell

| config | SHA-256 | seed | horizon | output root |
| --- | --- | ---: | --- | --- |
| `configs/v2-5-p3e/p0-B-107.json` | `206fc24ee0bc7f16aacabcbebc794b1fedd611922ef58e4578d5df142b323835` | 107 | 98 simulated hours | `artifacts/v2-5-p3e/p0-B-107/` |

The cell copies the P3c population, seed, financial terms, local-feed
contract, 96-hour term horizon, treasury mandate, and 98-hour run horizon.
Its only economic-policy delta is:

```text
term_carry_allocator.passive_exit = {
  slice_qty: 100000,
  deadline_at_nano: 1736042405000000000
}
```

The slice is exactly the declared venue minimum. The deadline is five seconds
after the P0 horizon, so P0 cannot be interpreted as a deadline-cancellation
or eventual-close screen. It gives an actual passive child nearly two hours of
ordinary counterparty exposure after the P3c term end, without revealing the
simulation stop to the actor.

The historical P3c seed-107 control is retained as a diagnostic reference for
the registered sub-minimum aggressive-touch condition. It is not a same-build
paired market-outcome control, and P0 will make **no B-minus-A market-level
claim**. A fresh A/B lifecycle comparison is allowed only after P0 validates
the B evidence contract and a separate lifecycle protocol is registered.

## Cheap preflight

Run one five-minute full-evidence preflight from the committed source and
exact P0 config before the 98-hour cell. It may establish only config parsing,
completion sentinels, manifest provenance, P4 decision serialization, and
required analyzer outputs. It cannot activate the 96-hour exit policy and is
not P0 evidence.

Build each executable from source before the preflight and full cell:

```text
go build -o bin/multivenue ./cmd/multivenue
go build -o bin/mvanalyze ./cmd/mvanalyze
go build -o bin/prunegate ./cmd/prunegate
```

Use `GOMAXPROCS=4`, full logs, and require nonempty final `greeks.json` and
`latency.json` as the only completion sentinels. Preserve raw JSONL,
manifest/checkpoints, market-data sidecars, hashes, and all listed analyzer
outputs. Never prune before the measurement manifest and `prunegate` pass.

## Primary activation contract

B is activated only if all of the following are true in independently replayed
persisted evidence:

1. at least one P4 term reaches `TERM_ACTIVE` with canonical entry/fill and
   local receipt/frontier evidence;
2. after its declared `term_end`, the corresponding ordinary IOC unwind is
   ineligible specifically because the locally delivered opposing displayed
   quantity cannot form a legal child at the effective minimum;
3. the actor emits `SUBMIT_UNWIND_{PERP|SPOT}_POST_ONLY` with the same-side
   public touch, a quantity of exactly 100,000, `LIMIT`, `GTC`, and explicit
   `post_only=true`;
4. the compact gateway decision and canonical venue accepted/rejected evidence
   agree exactly on symbol, side, price, quantity, order type, time-in-force,
   and post-only bit;
5. the information replay, term-carry replay, generic order lifecycle,
   position/fill-position, balance/conservation, derivative funding,
   lifecycle, latency, stream hash, and evidence-artifact digest all parse and
   satisfy their registered mechanical counters.

No condition in P0 requires a passive fill. A zero-fill or partial-fill child
remains a valid liquidity observation if the policy and evidence gates pass;
it must not be called a term closure.

## Falsifiers, invalidation, and interpretation

The P3e policy is **FALSIFIED AT ACTIVATION** if the registered P3c-like
sub-minimum condition occurs but B cannot issue a legal local post-only child,
or if its own evidence shows an ordinary IOC was legal. It is **NOT
EXERCISED** if no finite matched term or no registered depth condition occurs.

The cell is **INVALID** if a P4 source/frontier, gateway, canonical-order,
post-only, actor-outcome, position, accounting, determinism, completion, or
artifact-hash contract fails. A post-only rejection because the order became
marketable after ordinary request latency is an observed venue outcome, not a
synthetic fill; it does not rescue activation unless all contractual evidence
still establishes a valid child attempt.

P0 cannot support a claim that passive liquidity fixes P3c, that the term
closes, that funding anchors basis, or that the broader ecology is realistic.
Those require separately registered lifecycle and paired market screens.
