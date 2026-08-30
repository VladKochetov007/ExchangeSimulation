# Preview-versus-committed matching: semantic audit

Branch `perf/auditB`, worktree `/home/vlad/development/exsim-auditB`, Go 1.26.7.

`exchange/order_handling.go:previewMatchExcluding` answers "what would happen if
this order matched?" for FOK preflight, spot admission, and the resting-level
aggregate check. It clones both book sides into detached copies
(`cloneBookForPreviewExcluding`), runs the configured matcher against the
clones, and discards them. The codebase relies on the preview's `MatchResult`
being what committed matching would actually produce. That claim had no direct
test. This audit tests it adversarially, specifies what a clone-free preview
would have to reproduce, and measures how much book state a preview really
touches.

Verdict: **no divergence in executable behaviour**. Three latent asymmetries
were found, all currently unreachable through the production callers, all
recorded as pinned tests. None is scientific-blocking.

## 1. Differential fuzz

`exchange/preview_differential_test.go`,
`TestPreviewMatchesCommittedMatching`. For each random scenario:

1. build a book, then warm it up with real matcher passes so partial fills,
   exhausted-and-refreshed iceberg tranches and re-queued time priority are
   *reachable* states rather than hand-forged ones;
2. run `previewMatchExcluding`;
3. assert the live book is byte-identical afterwards (full structural
   fingerprint: level order, aggregates, per-level queue, per-order live state,
   ID index);
4. remove the excluded makers — which is exactly what the caller does before
   committing — and run the configured matcher against the live book;
5. compare every field of every execution, in order, plus `FullyFilled`.

Both matchers run under the same fixed clock, so execution timestamps are
compared too. 40,000 scenarios per matcher.

Result: **0 divergences, 0 preview refusals, 80,000 scenarios**
(`PriceTimeMatcher` and `ProRataMatcher`).

The generator tallies which of the deliberately targeted conditions it actually
reached, so this is a claim about tested states rather than about the
generator's intentions. Reached at least this many times per matcher:

| condition | count (pricetime / prorata) |
|---|---|
| level queue depth > 1 / > 3 | 49,115 / 20,377 · 49,868 / 21,460 |
| single-order levels | 38,878 · 38,176 |
| multiple price levels | 22,564 · 22,580 |
| multiple *reachable* levels | 14,487 · 14,499 |
| levels the order cannot reach | 5,540 · 5,545 |
| empty opposite side | 4,399 · 4,382 |
| both sides empty | 507 · 501 |
| exclusion set non-empty | 23,644 · 23,787 |
| exclusion emptied a whole level | 7,426 · 7,347 |
| exclusion removed the touch order | 5,863 · 6,020 |
| resting iceberg | 79,455 · 80,095 |
| resting iceberg already refreshed mid-life | 10,133 · 14,569 |
| resting hidden order | 75,548 · 79,090 |
| resting partially filled | 22,597 · 47,319 |
| incoming client owns a crossable resting order | 25,547 · 25,977 |
| crossable level is *entirely* the incoming client's own orders | 6,972 · 6,878 |
| market / limit incoming | 8,024 / 31,976 |
| IOC / GTC / FOK | 10,012 / 19,925 / 10,063 |
| post-only incoming | 4,955 |
| iceberg / hidden incoming | 4,924 / 5,007 |
| outcome: no fill / partial / full | 16,110 / 5,086 / 18,804 |
| executions spanning several levels | 5,696 · 5,796 |
| same maker filled twice (iceberg refresh mid-match) | 5,260 · 5,459 |

Two book invariants the short-circuit depends on were asserted on every
scenario and never broke: `Best == ActiveHead` on both sides, and no empty
`Limit` left linked in the active list.

Boundary quantities the random corpus cannot draw are covered separately in
`TestPreviewBoundaryQuantities`: zero-quantity incoming (limit and market), unit
quantity, `math.MaxInt64` resting, `math.MaxInt64` incoming, and both at
`MaxInt64`. All agree.

End-to-end, `TestFOKAdmissionMatchesRealExecution` drives the whole `PlaceOrder`
path rather than the matcher boundary, 3,000 orders per matcher. An accepted FOK
never left a residual resting on the book; a killed FOK never mutated the book.
All 2,421 (pricetime) / 2,419 (prorata) kills were `FOK_NOT_FILLED`.

### Finding 1 — the preview silently requires an unfilled incoming order

**Classification: AMBIGUOUS SEMANTICS (latent; unreachable today; safe direction).**

`previewMatchExcluding` validates its own result with

```go
if result.FullyFilled != (filled == order.Qty) { return nil, false }
```

where `filled` is the sum of *this preview's* execution quantities. That identity
holds only when the incoming order arrives with `FilledQty == 0`. Minimal case:
one resting ask of 10 at 100; incoming buy, `Qty: 10, FilledQty: 7`, price 100.
Committed matching fills the 3-lot residual and reports `FullyFilled`. The
preview produces the same single 3-lot execution, then compares 3 against 10,
disagrees with itself and returns `(nil, false)`.

Every production caller (`order_handling.go:116`, `:658`,
`spot_execution_plan.go:108`, `:161`) builds a fresh order, so this cannot fire
today. The direction is safe: `(nil, false)` becomes an order rejection at every
call site, never a phantom fill. It is a trap for a future caller — an amend
path, or a re-preview after a partial IOC — which would see silent
`FOK_NOT_FILLED` / `INSUFFICIENT_BALANCE` rejections. `previewRemainingQty`
carries the same unstated assumption.

Pinned by `TestPreviewRequiresAnUnfilledIncomingOrder`. Fix, if wanted, is to
compare against the residual: `order.FilledQty + filled == order.Qty`.

### Finding 2 — the clone repairs an exhausted iceberg tranche

**Classification: AMBIGUOUS SEMANTICS (latent; unreachable today; masks a
corruption signal).**

`cloneBookForPreviewExcluding` routes every order through `Book.AddOrder`, and
`LinkOrder` grants a fresh display tranche to any iceberg handed to it with
`DisplayRemaining == 0`. A live resting iceberg therefore cannot be copied in an
exhausted state — the clone silently normalizes it.

On a well-formed book this is invisible: `DisplayRemaining > 0` is an invariant
of every resting iceberg, established by `LinkOrder` on insertion and restored by
`refreshIcebergTranche` the moment a tranche is consumed. The asymmetry matters
because it points the wrong way. On a book that has broken the invariant, the
preview reports a clean full fill while committed matching, on the identical
state, hits `panic("matching engine bug: attempted zero-quantity execution")` in
`matching/default.go:execute`. The clone hides a corruption signal instead of
propagating it.

Pinned by `TestPreviewCloneNormalisesAnExhaustedIcebergTranche`.

### Finding 3 — `marketDepthSaneExcluding` is unreachable on a well-formed book

**Classification: PERFORMANCE ONLY.**

The gate runs on every preview, walking the entire opposite side. It can only
return false when `TrySub(Qty, FilledQty)` fails or yields a negative remaining
— an already-corrupt book — because its per-level sum is a *subset* of
`limit.TotalQty`, whose representability `LinkOrder` already enforced on
insertion (it skips excluded orders and the incoming client's own). The 5-minute
integrated run confirms this: 105,672 previews, **0 refusals**. The 80,000 fuzz
scenarios likewise produced 0 refusals.

It is not dead code — it is the only thing standing between a corrupt aggregate
and pro-rata's allocation denominator — but it is a whole-side walk on the
admission path that can only fire on a book that is already broken. Pinned by
`TestPreviewRefusesOnlyOnACorruptBook`, which also documents the safe direction:
every caller turns `(nil, false)` into a rejection.

### Noted, not a finding: post-only rejects a self-cross

`postOnlyWouldTake` (`order_handling.go:617`) compares against the raw
`Best.Price`. A post-only buy priced at an ask level occupied *only* by the same
client's orders is rejected `POST_ONLY_WOULD_TAKE`, while the identical order
without the flag rests without taking anything (the matcher skips own orders).

This is **INTENDED MARKET SEMANTICS**, not a divergence: the venue runs
cancel-maker STP (`cancelOwnCrossingQuotes`, `order_handling.go:1810`), so
accepting that order would force-cancel the client's own resting quote. Refusing
it is the conservative reading of the post-only contract. The rejection *reason*
is a slight misnomer — it would not take, it would cancel — but the outcome is
right. Recorded because "post-only" was in scope and the check does not go
through the preview at all.

## 2. Is the clone necessary?

### Mutations the matchers perform on the books they are given

Both `PriceTimeMatcher.Match` and `ProRataMatcher.Match`:

*On the resting maker*
1. `maker.FilledQty += execQty`
2. `maker.DisplayRemaining -= execQty` (iceberg, while positive)
3. `maker.Parent.TotalQty -= execQty`
4. `maker.Status = Filled | PartialFill`
5. on full fill, `ebook.UnlinkOrder(maker)` — splices it out of the level queue
   (mutating neighbours' `Prev`/`Next` and `limit.Head`/`Tail`), decrements
   `limit.OrderCnt`, subtracts the remaining (zero here) from `limit.TotalQty`,
   and nils `Prev`/`Next`/`Parent`. The `book.Orders` index entry is deliberately
   left in place for settlement.
6. on tranche exhaustion, `refreshIcebergTranche` — `UnlinkOrder` (this time with
   a non-zero remaining, so `TotalQty` and `OrderCnt` do move), a fresh
   `DisplayRemaining = min(IcebergQty, remaining)`, then `LinkOrder` re-appending
   at the level tail. Net effect on the aggregates is zero; net effect on the
   queue is a loss of time priority. If `LinkOrder` fails the order is left
   unlinked with `Status = Cancelled`.

*On the level list*
7. `if IsEmpty(limit) { book.RemoveLimit(limit) }` — unlinks the `Limit`,
   `delete(b.Limits, price)`, resets `b.Best = b.ActiveHead`, and **returns the
   `Limit` to the shared `limitPool`**.

*On the incoming order*
8. `taker.FilledQty += execQty`, `taker.Status`.

`ProRataMatcher` additionally reads `limit.Price` for the execution price where
`PriceTimeMatcher` reads `maker.Price`; for a linked order these are the same
value. Neither matcher ever reads `book.Orders`, `Order.Status`,
`Limit.TotalQty` (except inside `LinkOrder`'s overflow guard) or `Limit.OrderCnt`
(except inside `IsEmpty` and `LinkOrder`'s guard).

### Which mutations are observable in the MatchResult

Directly encoded per execution: `MakerFilledQty` (1), `TakerFilledQty` (8), and
`Qty` — which depends on `makerAvailable`, hence on the live `DisplayRemaining`
(2). `FullyFilled` derives from (8). `Price`, `MakerTotalQty`, and the ID / client
/ side fields are read from state the matcher never mutates.

Not observable: `Status` (4), `Parent.TotalQty` (3), `OrderCnt`, and level
removal (7) — none is ever read back by either matcher, and (7) cannot affect
the walk because `nextLimit := limit.Next` is captured before the inner loop.

Observable only through the *traversal*: the queue splices in (5) and (6).
`UnlinkOrder` on a full fill is harmless — both matchers capture the cursor
(`next := order.Next`, or the whole `candidates` slice) before filling. The
iceberg re-queue in (6) is genuinely load-bearing, and is the crux of this
question. See below.

### Specification: what a clone-free preview must reproduce

Local state, keyed by order ID, for touched orders only:

- `deltaFilled[id]` — quantity filled so far in this preview.
- `display[id]` — the live tranche, lazily initialised from
  `order.DisplayRemaining`, decremented locally, refreshed locally.
- a per-level local `TotalQty` (live value minus this preview's fills at that
  level) — needed only to reproduce `LinkOrder`'s overflow guard, below.
- a taker-side `filled` accumulator.

Redirected reads:

- `o.FilledQty < o.Qty` becomes `o.FilledQty + deltaFilled[o] < o.Qty`.
- `makerAvailable(o)` becomes
  `min(o.Qty - o.FilledQty - deltaFilled[o], display[o])` for icebergs, and the
  first term alone otherwise.
- `exec.MakerFilledQty` becomes `o.FilledQty + deltaFilled[o]`.

Queue order must be reproduced **without moving anything**:

- Each level's traversal sequence starts as the live queue
  `limit.Head → Next → …` and, on every local iceberg refresh, the refreshed
  order is removed from its position and appended to a virtual tail. The preview
  must walk that locally maintained sequence, not the live pointers.
- This is not merely a re-ordering across passes. `PriceTimeMatcher`'s cursor
  reaches the virtual tail **within the same pass**: with queue `A, B, C` and `A`
  an iceberg that exhausts, the queue becomes `B, C, A`, the cursor continues
  from the already-captured `next = B`, walks `B, C`, and then follows
  `C.Next → A`, filling `A` a second time on its fresh tranche — before the
  `refreshed` flag triggers the outer rescan. A clone-free implementation that
  models the refresh as "handle it on the next pass" produces a different
  execution sequence. (The fuzz reaches this case 5,260 / 5,459 times per
  matcher, so it is not hypothetical.)
- `ProRataMatcher` rebuilds `candidates` from `limit.Head` on each pass, so it
  needs the same local sequence; its leftover assignment loop
  (`for i := range candidates`) makes the order load-bearing for allocation, not
  just for ordering.
- The refresh must reproduce `LinkOrder`'s failure mode: when
  `TryAdd(limit.TotalQty, remaining)` would overflow, the real matcher sets
  `Status = Cancelled` and the order **drops out of the level**. A clone-free
  preview that consulted the *live* `TotalQty` instead of the local one would
  disagree with the commit about whether the refresh succeeded.

Everything else is a pure read-only predicate: `CanMatch` on `limit.Price`,
self-trade skipping on `ClientID`, and the exclusion-set lookup.

Level removal, `Status`, `Parent.TotalQty` and `OrderCnt` can all be skipped
entirely — none is observable in the `MatchResult`, and none feeds the walk.
The walk may start from `ActiveHead` rather than `Best`; those never disagreed
across the 80,000 fuzz books.

### What makes it impossible or subtle

1. **The decisive one: `MatchingEngine` is a public extension point.** The
   preview's whole purpose (`order_handling.go:917-922`, `:112-116`) is to run
   *the configured matcher*, so that a user-supplied allocation, iceberg or
   self-trade rule cannot disagree with the atomicity check. A clone-free
   preview cannot run an arbitrary injected matcher read-only; it would be a
   second implementation of a traversal the library does not own. It can only
   ever be an opt-in fast path for matchers that make an explicit promise —
   the pattern `PriceCrossingMatcher` already establishes — and the clone must
   remain the fallback. Replacing the clone outright would silently narrow the
   library's extension contract.
2. **Object pooling.** `RemoveLimit` calls `putLimit`, returning the `Limit` to a
   shared `sync.Pool`. Running the real matcher against live objects would hand
   live, still-referenced levels back to the pool. Any clone-free path must
   therefore be a *separate traversal*, never "run the matcher and undo it".
3. **The intra-pass iceberg re-queue** described above.
4. **`LinkOrder`'s overflow guard inside `refreshIcebergTranche`**, which needs a
   local level aggregate to stay faithful.
5. Execution pooling (`getExecution`) is unchanged either way: the release
   discipline in `releasePreviewExecutions` still applies.

No replacement was implemented, per the brief.

## 3. Touched-depth distribution

`research/configs/v2-integrated-longrun/dev-607.json`, seed 900101, 5 simulated
minutes, `-log-mode full`, `GOMAXPROCS=1`, temporary instrumentation in
`previewMatchExcluding` (removed before commit).

**Call shape: 105,672 previews — 62,246 short-circuited, 43,426 cloned, 0
refused.** Per crossing (cloned) preview:

| metric | mean | p50 | p90 | p95 | p99 | p99.9 | max |
|---|---|---|---|---|---|---|---|
| levels traversed | 1.00 | 1 | 1 | 1 | 3 | 3 | 5 |
| orders traversed | 2.00 | 1 | 4 | 6 | 6 | 6 | 6 |
| orders matched | 1.83 | 1 | 4 | 5 | 6 | 6 | 6 |
| executions | 1.83 | 1 | 4 | 5 | 6 | 6 | 6 |
| **exclusion-set size** | **0.00** | 0 | 0 | 0 | 0 | 0 | 0 |
| orders copied by the clone | 9.53 | 10 | 15 | 16 | 16 | 16 | 16 |
| levels copied by the clone | 5.17 | 5 | 9 | 11 | 14 | 16 | 16 |

Levels traversed: 88.4% traverse exactly one level, 3.4% two, 1.3% three, 0.1%
four or five. Orders traversed: 44.3% read one order, 21.5% two, 10.6% three,
9.5% four.

Across the run the clones copied 413,901 orders; a clone-free walk would have
read 87,067 — **21%**. Per preview the median is 1 order traversed against 10
copied.

Two facts worth separating out.

**The exclusion set is never exercised in this workload.** All 43,426 crossing
previews ran with `excluded == nil`. The set only becomes non-empty when a
resting spot maker cannot fund its side of a planned trade
(`prepareSpotExecutionPlan`'s re-plan loop), which never happens under dev-607.
The exclusion path is therefore covered *only* by the fuzz above (23,644 /
23,787 scenarios, including 7,426 that emptied an entire level and 5,863 that
removed the touch order), not by any integrated run. That is worth knowing
before trusting it in a campaign.

**2,925 clones are built for a market order against an empty opposite side.**
`previewCannotCross` returns false for `order.Type == Market` before it looks at
the book, so a market order whose opposite side holds no included liquidity
still builds two detached books and matches nothing. This exactly reconciles the
count in the existing code comment at `order_handling.go:993`: 62,246 caught by
the short-circuit + 2,925 missed = 65,171, the "cannot cross" figure that comment
quotes. Letting the short-circuit answer for market orders when the included set
is empty would remove them with no semantic change — the matcher returns nothing
for an empty book either way. Classification: **PERFORMANCE ONLY**.

No timing claims are made; this host's A/B noise floor is about ±1.2% and none
of the above was measured against it.

## What could not be established

- Whether the preview agrees with committed matching for a **user-supplied**
  `MatchingEngine`. The fuzz covers the two matchers in `matching/`. Any
  third-party matcher that mutates state the clone does not reproduce — or that
  reads `Book.Orders`, `Book.byClient` (nil on clones, by design), or `Limit`
  aggregates — is outside what was tested. The clone-free specification in
  section 2 is stated for these two matchers only.
- Whether the exclusion path behaves correctly under a *real* unfunded-maker
  sequence. It never occurred in dev-607, so the evidence is fuzz-only.
- Nothing was measured on the derivative / `Settleable` admission path, which
  takes `prepareFeeExecutionPlan` but not `prepareSpotExecutionPlan`. The
  preview call is the same; the surrounding accounting is not, and was not
  audited.

## Reproduction

```
go test ./exchange/ -run 'TestPreviewMatchesCommittedMatching' -v   # 80k differential scenarios
go test ./exchange/ -run 'TestFOKAdmissionMatchesRealExecution' -v  # 6k end-to-end FOK orders
go test ./exchange/ -run 'TestPreview' -v                           # incl. the three pinned findings
```
