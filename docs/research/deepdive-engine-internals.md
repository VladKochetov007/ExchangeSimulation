# Deep Dive: Real Matching Engine Internals vs. This Repo

Research date: 2026-07-28. Every external claim carries its source URL. Repo claims carry `file:line`.

Scope: how production exchanges build the matching hot path (architecture, data structures, wire
protocol, concurrency model), and what that implies for `exchange_sim`. Code inspected:
`exchange/exchange.go`, `exchange/order_handling.go`, `exchange/settlement.go`, `exchange/gateway.go`,
`book/book.go`, `book/orderbook.go`, `matching/`, `marketdata/marketdata.go`.

---

## 1. Matching engine architecture

### 1.1 The single-writer / sequencer consensus

Every high-performance venue converges on the same shape: **one thread owns the book, fed by one
ordered input queue, with all I/O pushed to the edges.**

- LMAX's Business Logic Processor runs "all trades, from all customers, in all markets — on a single
  thread" at **6M orders/sec** on a 2011-era dual-socket Nehalem box.
  <https://www.martinfowler.com/articles/lmax.html>
- The BLP is **entirely in-memory, no database, no locks** in business logic. Current state "is
  entirely derivable by processing the input events". Snapshot nightly, replay the day's journal to
  restart in under a minute. <https://www.martinfowler.com/articles/lmax.html>
- **External calls are forbidden inside business logic** — they would stall the whole processor. Any
  external interaction is split into "emit request event" / "consume response event".
  <https://www.martinfowler.com/articles/lmax.html>
- Island ECN (1996, Josh Levine) introduced the **sequencer**: a single-threaded UDP multicast bus
  that "multicasts all messages into a sequence of numbers". NASDAQ INET is a direct descendant and
  still uses it for billions of messages/day.
  <https://electronictradinghub.com/an-introduction-to-the-sequencer-world/>
- The sequencer's payoff is stated plainly: "fully deterministic, meaning that every node sees all
  messages in the same order and all events can be replayed on a single stream producing the exact
  same state." That is what makes hot/warm failover and state replication tractable — the backup is
  an exact mirror, not an approximation.
  <https://electronictradinghub.com/an-introduction-to-the-sequencer-world/>
- Reported sequencer-architecture performance: sub-10µs engine latency at >2M transactions/sec.
  <https://electronictradinghub.com/an-introduction-to-the-sequencer-world/>

### 1.2 Why locks are avoided in the hot path — the actual numbers

The Disruptor paper measured incrementing a 64-bit counter 500M times on a 2.4GHz Westmere:

| Method                     | Time (ms) |
| -------------------------- | --------- |
| Single thread              | 300       |
| Single thread **with lock** | 10,000    |
| **Two threads** with lock  | 224,000   |
| Single thread with CAS     | 5,700     |
| Two threads with CAS       | 30,000    |
| Single thread volatile write | 4,700   |

Source: <https://lmax-exchange.github.io/disruptor/disruptor.html>

Uncontended locking is ~33x slower than plain single-threaded work; **contended locking is ~750x
slower**. The conclusion the industry drew is not "use better locks" — it is "arrange for there to be
exactly one writer".

Queue-based handoff is rejected for a specific reason: "Queues typically have write contention on the
head, tail, and size variables", and those variables "generally occupy the same cache-line", so even
separate concurrent objects suffer false sharing (64-byte cache lines).
<https://lmax-exchange.github.io/disruptor/disruptor.html>

Latency comparison, 3-stage pipeline, ArrayBlockingQueue vs Disruptor:

| Metric | ABQ (ns)  | Disruptor (ns) |
| ------ | --------- | -------------- |
| Mean   | 32,757    | 52             |
| 99th   | 2,097,152 | 128            |

Source: <https://lmax-exchange.github.io/disruptor/disruptor.html>

The shape matters more than the magnitude: the queue exhibits the classic latency J-curve under load,
the ring buffer stays near-flat. A mutex-guarded engine inherits the J-curve.

### 1.3 Reference open-source implementation: exchange-core

`exchange-core` (Java, LMAX Disruptor + Agrona + Adaptive Radix Trees) is the closest public analogue
to what this repo is: <https://github.com/exchange-core/exchange-core>

- Pipelined multi-core processing where "each CPU core is responsible for certain processing stage,
  user accounts shard, or symbol order books shard" — i.e. **shard by symbol and by account**, do not
  share one lock.
- "Matching engine and risk control operations are atomic and deterministic", with **no floating-point
  arithmetic** (this repo already gets that right — integer `int64` throughout).
- Event sourcing: "disk journaling and journal replay support, state snapshots (serialization) and
  restore operations, LZ4 compression."
- Two order book implementations shipped side by side: "Naive" (simple) and "Direct" (performance) —
  a useful precedent for keeping a readable reference implementation next to a fast one.
- Latency at 1M ops/sec: p50 0.5µs, p99 4µs, p99.99 31µs, worst 45µs. Throughput 5M ops/sec single
  symbol.

---

## 2. Order book data structures in production

The canonical design (WK Selph, "How to Build a Fast Limit Order Book") is a **three-structure
composite**, and it is essentially what this repo already implements:

- Price levels held in a sorted structure (binary tree in the original write-up), each level being a
  **doubly linked list of orders** in arrival (FIFO) order.
- A **hash table keyed by order ID** for O(1) cancel: look up the order, unlink from the level in O(1)
  via its `prev`/`next` pointers, using a `parentLimit` back-pointer.
- Target: add / cancel / execute all O(1) amortized.
  <https://gist.github.com/halfelf/db1ae032dc34278968f8bf31ee999a25>

The design choice that varies between venues is **how price levels are indexed**, and it is chosen by
book sparsity — "depending on the expected sparsity of the book (the average distance in cents between
limits that have volume), there are different implementations used."
<https://gist.github.com/halfelf/db1ae032dc34278968f8bf31ee999a25>

Two production families:

- **Dense / flat array indexed by tick.** "the limit order book is represented using a flat linear
  array (pricePoints), indexed by the numeric price value", each entry a FIFO queue. O(1) level
  lookup, no allocation, cache-friendly; costs memory proportional to price range and needs a
  best-bid/best-ask cursor that walks on level exhaustion. Used where tick range is bounded.
- **Sparse / tree.** Balanced BST, skiplist, or Adaptive Radix Tree (exchange-core's choice) for
  O(log n) level insert. Used for wide/unbounded price ranges such as crypto.

Hardware order-book patents note the same trade space explicitly: implementations use "simple arrays
or more complex data structures (such as heaps, trees, etc.) to avoid shifting too many data."
<https://image-ppubs.uspto.gov/dirsearch-public/print/downloadPdf/10846795>

Common optimizations reported across implementations: slab/pool allocation for orders and levels,
side-specialized matching, and **cache-hot best-level pointers** so the touch is never a tree
traversal. <https://github.com/mansoor-mamnoon/limit-order-book>

**Where this repo sits:** `book/book.go` uses exactly the Selph composite — `Limits map[int64]*Limit`
for level lookup, `Orders map[uint64]*Order` for O(1) cancel (`book.go:158`), intrusive doubly-linked
orders with `Parent` back-pointer (`book.go:39-74`), `sync.Pool` for `Limit` reuse (`book.go:10-26`),
and a cached `Best` pointer (`book.go:244`). The one divergence from production practice is the level
ordering structure — see §5, E5.

---

## 3. Protocol level: what real feeds actually publish

### 3.1 ITCH — market data, order-by-order

NASDAQ TotalView-ITCH 5.0 "is composed of a series of messages that describe orders added to, removed
from, and executed on Nasdaq".
<https://www.nasdaqtrader.com/content/technicalsupport/specifications/dataproducts/NQTVITCHSpecification_5.0.pdf>

Message set (the full lifecycle, not a snapshot stream):

- **System Event** — market open/close/halt state transitions.
- **Add Order** (with and without MPID attribution) — introduces an order reference number.
- **Order Executed** — "sent whenever an order on the book is executed in whole or in part. It is
  possible to receive several Order Executed Messages for the same order reference number if that
  order is executed in several parts."
- **Order Executed With Price** — execution at a price other than the display price.
- **Order Cancel** — *partial* quantity reduction, not removal.
- **Order Delete** — full removal.
- **Order Replace** — cancel + add with a new reference number, atomically.
- **Trade (non-cross)** — the mechanism for reporting executions against **non-displayed** liquidity,
  which by definition never had an Add Order message.
- **Cross Trade**, **Broken Trade**, **Stock Trading Action**.

Two structural points worth internalizing:

1. The feed is a **delta log keyed by order reference number**, not a level-quantity feed. A
   subscriber reconstructs the book by applying add/execute/cancel/delete in order. Level aggregation
   is the *client's* job.
2. **Hidden liquidity is representable without leaking it**: it produces a `Trade` message at
   execution time and never an `Add Order`. This repo's dark-order handling (`order_handling.go:96-98`,
   `restOrReleaseOrder` suppressing deltas for `Hidden`) matches this convention.

### 3.2 Transport and gap recovery — the part sims usually skip

- **MoldUDP64** is the sequenced-multicast layer under ITCH: "a lightweight protocol layer built on
  top of UDP that provides a mechanism for listeners to detect and re-request missed packets."
  Each packet carries a sequence number; "a receiver may need to send a request when it detects a
  sequence number gap", answered by a unicast retransmission.
  <https://www.nasdaqtrader.com/content/technicalsupport/specifications/dataproducts/moldudp64.pdf>
- **GLIMPSE** provides point-in-time book snapshots so a late joiner or a hopelessly-gapped client can
  re-seed rather than replay from open. <https://www.onixs.biz/insights/itch-protocol-usage>
- **SoupBinTCP** is the guaranteed-delivery point-to-point sibling used for order entry (OUCH) —
  "guaranteed delivery of sequenced messages from a server to a client in real-time".
  <https://www.nasdaqtrader.com/content/technicalsupport/specifications/dataproducts/moldudp64.pdf>

CME MDP 3.0 makes the two-level sequencing explicit, and this is the design detail most relevant to
this repo:

- **Packet-level sequence number** — "incremental; therefore, if a gap is detected between packets,
  this indicates a packet has been missed. In such a case, it should be assumed that all books
  maintained in the client system may no longer have the correct, latest state."
- **Per-instrument sequence number** — tag `83-RptSeq` "represents the sequence number per
  instrument". Clients track RptSeq *per instrument* and detect per-book gaps independently.
- **Recovery** is a separate snapshot loop; clients "compare the Market Recovery Snapshot message tag
  369-LastMsgSeqNumProcessed to the Incremental feed ... packet sequence number. Drop all cached
  Incremental feed updates with a packet sequence number < 369-LastMsgSeqNumProcessed."

Sources:
<https://www.cmegroup.com/confluence/display/EPICSANDBOX/MDP+3.0+-+Market+Data+Incremental+Refresh>,
<https://cmegroupclientsite.atlassian.net/wiki/spaces/EPICSANDBOX/pages/457325847/MDP+3.0+-+Recovery+Services>

The invariant: **a gap must be detectable by the receiver, and there must be a defined path back to a
known-good state.** Silent loss is never acceptable, because a book built from a lossy delta stream is
wrong forever, not just briefly.

---

## 4. Thread-safety model: real engines vs. a mutex-based Go sim

| Dimension        | Sequencer / LMAX engine                          | This repo                                                       |
| ---------------- | ------------------------------------------------ | --------------------------------------------------------------- |
| Writers to state | Exactly one thread                               | N client goroutines serialized by `e.mu` (`exchange.go:81`)       |
| Ordering source  | Sequencer assigns global seq before processing    | Go scheduler + mutex acquisition order                            |
| Replay           | Journal input events, replay to identical state   | None                                                              |
| I/O in hot path  | Forbidden; pushed to edge disruptors              | Channel sends (some blocking) inside the lock                     |
| Failure model    | Replicas process the same stream, µs failover     | Single process                                                    |
| MD sequencing    | Per-channel packet seq + per-instrument seq       | One global counter (`marketdata.go:95`)                           |

Go-specific behavior that matters for the "serialize with a mutex" approach:

- `sync.Mutex` has two modes. In **normal mode** "waiters are queued in FIFO order, but a woken up
  waiter does not own the mutex and competes with new arriving goroutines over the ownership. New
  arriving goroutines have an advantage — they are already running on CPU and there can be lots of
  them, so a woken up waiter has good chances of losing." That is barging, and it is a fairness
  hazard: a goroutine can be repeatedly overtaken.
- **Starvation mode** engages when a waiter has waited **>1ms**, after which ownership is handed off
  directly FIFO and new arrivals do not compete.

Source: <https://victoriametrics.com/blog/go-sync-mutex/>, and the upstream rationale
<https://groups.google.com/g/golang-codereviews/c/72wkxOKtil0>

The consequence for a simulator: **the order in which two concurrently-submitted orders reach the
book is decided by the Go scheduler and mutex mode, not by anything in the model.** Two runs of the
same scenario can produce different trade sequences. That is fine for a load test and fatal for a
reproducible research harness. Real engines get determinism structurally (one input stream, one
writer) rather than by hoping the scheduler behaves.

---

## 5. Elephants in the room

Ranked by how much damage they do to the simulator's credibility, not by how hard they are to fix.

### E1 — Blocking channel sends execute while holding the global exchange write lock

`sendResponse` is a **retry loop that blocks up to indefinitely**: on a full `ResponseCh` it loops
`for g.IsRunning()` with a `time.After(10 * time.Millisecond)` backoff, giving up only when the
gateway closes (`gateway.go:81-97`).

It is called from inside the `e.mu` write-locked region in at least four places:

- `settlement.go:305-329` — `sendFillNotification`, reached from `processExecutions` inside
  `PlaceOrder` (`order_handling.go:4-5, 39`).
- `order_handling.go:709` — STP forced-cancel notification in `cancelOwnCrossingQuotes`.
- `exchange.go:401` — `CancelAllClientOrders`, which holds `e.mu.Lock()` at `exchange.go:349`.
- `liquidation.go:73` — forced cancel during liquidation.

(`exchange.go:571` in `HandleClientRequests` is correctly *outside* the lock; `market.go:51` is a
different type and unaffected.)

This is precisely the thing LMAX forbids — "You can't make calls to external services within the
business logic" as they would stall the entire processor
(<https://www.martinfowler.com/articles/lmax.html>). Here the "external service" is a slow consumer
goroutine. One client that stops draining its `ResponseCh` (10,000 buffered, `gateway.go:9-12`)
freezes **every book, every client, and every position** in the exchange in 10ms increments until it
either drains or disconnects. The comment at `gateway.go:74-80` correctly argues for at-least-once
delivery; the flaw is *where* the waiting happens, not that it waits.

Deadlock note, stated precisely: a true cycle requires a `ResponseCh` consumer that synchronously
calls back into the exchange. Grepping `actor/`, `simulation/`, `simulations/` finds no such caller
today, so this is currently a **stall**, not a deadlock. It becomes a deadlock the moment any consumer
calls an `e.mu`-taking method (e.g. `GetBook`, `CancelAllClientOrders`) from its response-handling
path. Nothing in the API prevents that.

### E2 — Map iteration in the hot path makes the event stream non-deterministic

Go randomizes map iteration order by design. The engine iterates maps in places where the iteration
order is directly observable in output:

- `publishLevels` iterates `map[int64]Side` (`order_handling.go:606-644`) — **the order of published
  book deltas after a multi-level sweep varies run to run.**
- `cancelOwnCrossingQuotes` iterates `opposite.Orders` (`order_handling.go:682`) — the order in which
  self-crossing quotes are cancelled, and therefore the order of the resulting deltas and
  `ForcedCancelNotification`s, varies.
- `CancelAllClientOrders` iterates `e.Books` and each book's `Orders` (`exchange.go:~360-374`).
- `CancelOrder` scans `for _, b := range e.Books` to locate the order (`order_handling.go:58-64`).
- `hedgeReduceViolation` iterates `side.Orders` (`order_handling.go:319`) — order-independent since it
  only sums, but it is O(all resting orders on that side) per placement.

Against the sequencer standard — "all events can be replayed on a single stream producing the exact
same state" (<https://electronictradinghub.com/an-introduction-to-the-sequencer-world/>) — a run of
this engine is not reproducible even single-threaded with a `SimulatedClock`. This directly undercuts
the fuzz/invariant tests already in `tests/` (`invariant_fuzz_test.go`, `concurrent_fuzz_test.go`): a
failure found at seed N may not reproduce at seed N.

### E3 — One global market-data sequence number, and silent drops with no gap signal

`MDPublisher.seqNum` is a single counter incremented once per `Publish` across **all symbols and all
`MDType`s** (`marketdata.go:41, 95-96`). Every subscriber to every symbol draws from the same space.

Consequences, measured against CME MDP 3.0 and MoldUDP64:

- A subscriber to `BTC-USD` sees sequence numbers 3, 7, 12, 18 — gaps that are entirely normal because
  other symbols consumed the intervening numbers. **Per-instrument gap detection is structurally
  impossible.** Real feeds solve this with a per-instrument `RptSeq` alongside the packet sequence
  (<https://www.cmegroup.com/confluence/display/EPICSANDBOX/MDP+3.0+-+Market+Data+Incremental+Refresh>).
- When a subscriber's channel is full, the message is **silently dropped** (`marketdata.go:114-118`,
  `default:` with the comment "Gateway closed, silently drop"). The subscriber's book is now
  permanently wrong and it has no way to learn this. MoldUDP64's entire reason for existing is that
  receivers must "detect and re-request missed packets"
  (<https://www.nasdaqtrader.com/content/technicalsupport/specifications/dataproducts/moldudp64.pdf>).
- There is no snapshot-recovery path equivalent to GLIMPSE or the MDP snapshot loop. `PublishSnapshot`
  exists (`exchange.go:620-629`) and periodic snapshots can be enabled, but it is a broadcast on a
  timer, not a recovery service a gapped client can pull.

Note the asymmetry with E1: **responses block forever to avoid loss, market data drops silently.** Two
opposite backpressure policies in one engine, neither matching venue behavior (venues do the reverse
of both: order-entry is guaranteed-delivery over SoupBinTCP, market data is lossy-but-detectable over
MoldUDP64 with retransmit).

### E4 — No input journal, therefore no replay, therefore no bug reproduction

Grepping the whole tree for `Journal|Replay|EventLog` returns nothing. The `Logger` interface writes
human/analysis output, not a replayable input stream.

Both reference architectures treat this as foundational, not optional: LMAX's state "is entirely
derivable by processing the input events" with nightly snapshots and journal replay
(<https://www.martinfowler.com/articles/lmax.html>); exchange-core ships "disk journaling and journal
replay support, state snapshots (serialization) and restore operations"
(<https://github.com/exchange-core/exchange-core>).

For a *simulation library* this is arguably worth more than it is to a real venue: it turns "the
gen-8 fuzz run broke an invariant somewhere in 40 minutes of simulated time" into a file you can
replay in seconds. The bug-hunt history in `MEMORY.md` (14 fixes in July, gen-8 zombie-quote caveat)
is exactly the workload this would accelerate.

### E5 — Price-level insertion is O(number of levels), from the wrong end

`Book.insertLimit` walks the sorted level list from `ActiveHead` looking for the insertion point
(`book.go:184-224`). Inserting a level far from the touch costs a full traversal of every better
level, and it is a pointer-chase over pool-allocated `Limit` objects, so it is cache-hostile at depth.

This is the one place the repo diverges from the Selph composite it otherwise implements. Production
picks either a tick-indexed flat array (O(1), bounded price range) or a tree/ART (O(log n), sparse
books) precisely to avoid this walk
(<https://gist.github.com/halfelf/db1ae032dc34278968f8bf31ee999a25>,
<https://github.com/exchange-core/exchange-core>).

Impact is workload-dependent and currently probably modest — but it scales with book depth, and the
market-maker/random-walk simulations in `simulations/` are exactly the workload that builds deep books
and re-quotes constantly. `marketBuyCost` (`order_handling.go:395-415`) and `canFillFully`
(`order_handling.go:552-575`) additionally walk levels *and* orders on every market order and every
FOK, so a deep book multiplies through several paths.

Secondary: `Book.Best` duplicates information already in `ActiveHead` — the list is sorted, so the head
*is* the best. `updateBest` (`book.go:244-258`) and the `Best = ActiveHead` reset in `RemoveLimit`
(`book.go:238-240`) maintain a second copy of a derived value. Two fields that must agree are two
fields that can disagree.

### E6 — A single `RWMutex` for a workload that is almost entirely writes

`e.mu` (`exchange.go:81`) guards every book, every client, every position, and the instrument map.
`PlaceOrder` and `CancelOrder` take the full write lock; only queries and `GetBook` take `RLock`.

- Per-symbol books are logically independent. exchange-core shards "user accounts shard, or symbol
  order books shard" across cores (<https://github.com/exchange-core/exchange-core>); here `BTC-USD`
  order flow serializes against `ETH-USD` order flow for no modeled reason.
- `RWMutex` on a write-dominated path costs more than `Mutex` and buys nothing — the project's own
  `CLAUDE.md` flags exactly this pattern ("bad: RWMutex with no actual read paths").
- Contention cost is not linear. The Disruptor measurements show two threads contending a lock at
  224,000ms vs 10,000ms uncontended — **a 22x cliff, not a gentle degradation**
  (<https://lmax-exchange.github.io/disruptor/disruptor.html>). With one goroutine per client
  (`exchange.go:434`, `HandleClientRequests`), the contended-writer count equals the number of active
  clients.

### E7 — Market data payloads are aliased pointers shared across subscribers

`Publish` allocates a fresh `MarketDataMsg` per subscriber but assigns `Data: data` — **the same
underlying object pointer to every subscriber** (`marketdata.go:107-113`). It then escapes both the
publisher lock and `e.mu`.

`createTrade` is the sharpest case: it builds one `*Trade`, stores it as `book.LastTrade`, **and**
publishes that same pointer (`settlement.go:107-119`). The exchange and every subscriber now hold the
same object; `GetLastPrice` (`orderbook.go:15-20`) reads it under a different lock than the subscriber
that holds it.

Today `Trade` is not mutated after construction, so this is latent rather than live. What makes it
worth naming is the neighborhood: this code sits directly beside three `sync.Pool` recycling paths
(`putOrder`, `PutExecution` at `types.go:222`, `putLimit` at `book.go:23`). A future refactor that
pools `Trade` or `BookDelta` converts a latent alias into a use-after-recycle that will present as
impossible-looking corrupted market data. Real feeds are immune by construction because they serialize
to bytes on the wire.

### E8 — `PlaceOrder`'s return value

`Response{... Data: e.NextOrderID}` (`order_handling.go:44`) returns the field rather than the `order.ID`
captured at line 13-14. It is correct today only because `e.mu` is held for the whole function and
nothing else increments the counter in between. It is a data dependency on lock scope rather than on
the value the caller actually wants, and it breaks the instant order-ID allocation moves anywhere else
(pre-allocation, sharding, batch admission). Cheap to make explicit: `Data: order.ID`.

---

## 6. Concrete recommendations for this repo

Ranked by (value to a simulation library) ÷ (risk of the change). Each states the mechanism and the
expected benefit.

### R1 — Move all gateway notifications outside the exchange lock *(highest value, contained change)*

**Mechanism.** Give the write-locked paths a notification buffer instead of a direct send. Collect
`(gateway, Response)` pairs into a slice on the exchange or a per-call context during the locked
region; flush the slice after `e.mu` is released. In `PlaceOrder` that is a `defer` ordering change
plus threading a buffer through `processExecutions` → `notifyFill` → `sendFillNotification`. The four
call sites are `settlement.go:310`, `order_handling.go:709`, `exchange.go:401`, `liquidation.go:73`.
`sendResponse` itself does not change — its at-least-once retry is the correct policy, it just must
not run under the lock.

**Expected benefit.** Removes the only unbounded stall in the engine. A slow or wedged consumer can no
longer freeze unrelated symbols and clients. Eliminates the latent deadlock cycle described in E1
before anyone writes the consumer that triggers it. Also shortens the critical section on every fill,
which directly attacks E6's contention.

**Ordering caveat worth designing deliberately:** notifications currently interleave with book
mutation. Buffering changes observable ordering to "all state changes, then all notifications". That
is the same choice LMAX makes (output disruptor after the BLP) and is closer to venue behavior, but it
should be a stated decision, and the notification buffer must preserve *relative* order.

### R2 — Make the event stream deterministic

**Mechanism.** Eliminate observable map iteration in the hot path:

- `collectAffectedLevels` → return an ordered `[]struct{Price int64; Side Side}` deduped by insertion
  order (executions already arrive in matched order), so `publishLevels` emits deltas in causal
  order.
- `cancelOwnCrossingQuotes` → walk the level list (`opposite.ActiveHead` → `limit.Head`) rather than
  the `Orders` map, giving price-then-time order.
- `CancelAllClientOrders` → same, walk books in a deterministic order (sorted symbol, or a maintained
  symbol slice) and levels in book order.
- `CancelOrder` → index order ID → symbol once at placement instead of scanning `e.Books`; this is
  also an O(symbols) → O(1) win.

**Expected benefit.** Identical input sequence produces an identical output stream, which is the
property the whole `tests/*fuzz*` corpus silently assumes. Fuzz failures become reproducible from a
seed. This is the prerequisite for R3 — replay is meaningless if the engine is not deterministic.

### R3 — Event journal + replay harness

**Mechanism.** Define an append-only journal of accepted inputs: `(seq, simTimestamp, clientID,
Request)`, written by `HandleClientRequests` at the point of admission, plus the non-request inputs
that mutate state (funding ticks, index price updates, listing/expiry events) so the stream is closed
under state transitions. Add a `ReplayExchange(journal)` that constructs a fresh exchange with a
`SimulatedClock` driven from journal timestamps, feeds the stream single-threaded, and compares the
resulting state and output events against the recorded run. Keep the journal an injected interface
(`nil` = disabled) so it stays optional and library-clean.

**Expected benefit.** This is the highest-leverage feature for the project's actual workload. It turns
long-running fuzz and simulation failures into a seconds-long deterministic repro, gives every fixed
bug a permanent regression corpus entry, and provides a conservation-invariant checker that can run
offline over the stream. It is what LMAX and exchange-core both consider table stakes
(<https://www.martinfowler.com/articles/lmax.html>,
<https://github.com/exchange-core/exchange-core>). It also directly serves the `MEMORY.md` bug-hunt
workflow.

**Sequencing note:** R2 must land first, and the journal's sequence number should become *the* engine
sequence number — assigned once at admission and reused for market data (see R4), exactly as a real
sequencer does.

### R4 — Per-instrument market-data sequencing with explicit gap signalling

**Mechanism.** Three parts, each independently shippable:

1. Replace the single `seqNum` with a per-symbol counter (`map[string]uint64`, or a counter on
   `OrderBook` next to the existing `SeqNum` trade counter at `orderbook.go:12`), mirroring CME's
   per-instrument `RptSeq`. Keep a global packet-level counter alongside it if you want two-level
   detection.
2. Replace the silent `default:` drop (`marketdata.go:117`) with a recorded gap: increment a per-
   subscriber drop counter and set a `gapped` flag; on the next successful send, deliver an
   `MDGap{Symbol, FromSeq, ToSeq}` message first. The subscriber now *knows* its book is stale.
3. Add a pull-based recovery entry point — a `RequestSnapshot(clientID, symbol)` request type served
   from the current book, the GLIMPSE/MDP-snapshot-loop analogue. Clients that see `MDGap` re-seed
   instead of drifting.

**Expected benefit.** Subscribers can detect and recover from loss, which is the whole point of the
MoldUDP64/GLIMPSE and MDP incremental+snapshot designs
(<https://www.nasdaqtrader.com/content/technicalsupport/specifications/dataproducts/moldudp64.pdf>,
<https://cmegroupclientsite.atlassian.net/wiki/spaces/EPICSANDBOX/pages/457325847/MDP+3.0+-+Recovery+Services>).
Beyond correctness, it makes the sim *usable for research on feed-handler behavior* — you can now
model a slow consumer and study what a strategy does with a stale book, which is currently
unrepresentable. It also removes a whole class of confusing test failures where an actor's book
quietly diverged.

### R5 — Single-writer engine core (the structural fix; do after R1–R3)

**Mechanism.** Replace `e.mu` with the LMAX shape: all mutating requests land on one inbound queue
(buffered channel, or a ring buffer if allocation matters) consumed by exactly one engine goroutine
that owns all books, clients, and positions and takes no locks. Query paths get versioned snapshots or
become queued requests. Per-client goroutines become pure producers. Outputs go to an outbound queue
drained by a separate publisher goroutine — which is R1's notification buffer, promoted to
architecture.

**Expected benefit.** Determinism stops being something R2 maintains by discipline and becomes
structural. The contended-lock cliff (224,000ms vs 10,000ms two-thread lock cost,
<https://lmax-exchange.github.io/disruptor/disruptor.html>) disappears along with the lock. `-race`
noise on engine state goes away. And the engine's model matches the thing it is simulating, which for
a *teaching and research* library is worth as much as the performance.

**Intermediate option if R5 is too large a swing:** shard `e.mu` per symbol, with a separate lock (or
ordered acquisition) for cross-symbol operations like `CancelAllClientOrders` and cross-margin
settlement. Cheaper, gets most of the contention win, gets *none* of the determinism win — which is
why it ranks below R5 rather than above it.

### R6 — Price-level index appropriate to book density

**Mechanism.** Make the level-ordering structure an injected strategy behind the existing `Book` API
(consistent with the library's open-for-extension rule), with two implementations: the current sorted
list as the readable default, and a tick-indexed flat array (`[]*Limit` indexed by
`(price - basePrice) / tickSize`) for bounded-range instruments. exchange-core's "Naive" vs "Direct"
split is the precedent (<https://github.com/exchange-core/exchange-core>). Sparse/wide-range
instruments would take a skiplist or B-tree variant.

**Expected benefit.** `insertLimit` goes from O(levels) to O(1) for dense books, which compounds
through `marketBuyCost` and `canFillFully`. Benchmark first — build a depth-1000 book and measure —
because this is the one recommendation here whose payoff is genuinely workload-dependent, and YAGNI
applies until the profile says otherwise.

### R7 — Small, cheap correctness/clarity items

- `Data: e.NextOrderID` → `Data: order.ID` (`order_handling.go:44`). Removes a hidden dependency on
  lock scope (E8).
- Copy market-data payloads by value into each subscriber's message, or document them as
  immutable-after-publish and enforce it (E7). Cheapest version: make `Data` carry a value type for
  `BookDelta` and `Trade`.
- Drop `Book.Best` and use `ActiveHead` directly, or derive `Best` in an accessor (E5 secondary) —
  one source of truth for a derived value.
- Make `hedgeReduceViolation`'s resting-reduce sum incremental (a per-client-per-side counter
  maintained on add/cancel/fill) instead of an O(resting orders) scan per placement
  (`order_handling.go:314-323`).

---

## 7. What this repo already gets right

Worth recording, so a future pass does not "fix" these:

- Integer-only arithmetic throughout — matches exchange-core's "no floating-point arithmetic, no loss
  of significance is possible" guarantee (<https://github.com/exchange-core/exchange-core>).
- The Selph order-book composite is correctly implemented: intrusive doubly-linked orders per level,
  `Parent` back-pointer, order-ID hash index, O(1) cancel
  (<https://gist.github.com/halfelf/db1ae032dc34278968f8bf31ee999a25>).
- Object pooling for `Order`, `Limit`, `Execution`, `MarketDataMsg` — the same allocation-avoidance
  discipline the Disruptor paper argues for.
- Hidden and iceberg liquidity follow venue semantics: hidden orders emit no public deltas
  (`order_handling.go:94-98`), and iceberg tranche exhaustion re-queues at the level tail losing time
  priority (`matching/matching.go:55-74`) — the real venue refresh rule.
- Public vs. internal book views are separated (`GetPublicSnapshot` vs `GetSnapshot`, `book.go:260-293`),
  which is the correct structural defense against dark-liquidity leaks.
- Self-trade prevention with cancel-maker semantics is modeled explicitly
  (`order_handling.go:669-712`).
- Per-book trade sequence numbers already exist (`orderbook.go:12`) — R4's per-instrument MD sequence
  is a small extension of a pattern already present.

---

## Sources

- [The LMAX Architecture — Martin Fowler](https://www.martinfowler.com/articles/lmax.html)
- [LMAX Disruptor technical paper](https://lmax-exchange.github.io/disruptor/disruptor.html)
- [An Introduction to the Sequencer World — Electronic Trading Hub](https://electronictradinghub.com/an-introduction-to-the-sequencer-world/)
- [How the First Low-Latency Trading Market Was Designed: Island ECN](https://electronictradinghub.com/how-the-first-true-low-latency-market-was-designed-and-architected/)
- [exchange-core (GitHub)](https://github.com/exchange-core/exchange-core)
- [How to Build a Fast Limit Order Book — WK Selph](https://gist.github.com/halfelf/db1ae032dc34278968f8bf31ee999a25)
- [High-performance limit order book (C++ reference implementation)](https://github.com/mansoor-mamnoon/limit-order-book)
- [Order book management device in a hardware platform (USPTO 10846795)](https://image-ppubs.uspto.gov/dirsearch-public/print/downloadPdf/10846795)
- [NASDAQ TotalView-ITCH 5.0 specification](https://www.nasdaqtrader.com/content/technicalsupport/specifications/dataproducts/NQTVITCHSpecification_5.0.pdf)
- [MoldUDP64 Protocol Specification v1.00](https://www.nasdaqtrader.com/content/technicalsupport/specifications/dataproducts/moldudp64.pdf)
- [ITCH Protocol: Origins, Context, Usage — Onix Solutions](https://www.onixs.biz/insights/itch-protocol-usage)
- [CME MDP 3.0 — Market Data Incremental Refresh](https://www.cmegroup.com/confluence/display/EPICSANDBOX/MDP+3.0+-+Market+Data+Incremental+Refresh)
- [CME MDP 3.0 — Recovery Services](https://cmegroupclientsite.atlassian.net/wiki/spaces/EPICSANDBOX/pages/457325847/MDP+3.0+-+Recovery+Services)
- [CME MDP 3.0 — MBP and MBOFD Market Recovery](https://cmegroupclientsite.atlassian.net/wiki/spaces/EPICSANDBOX/pages/457672425/MDP+3.0+-+MBP+and+MBOFD+Market+Recovery)
- [Go sync.Mutex: Normal and Starvation Mode — VictoriaMetrics](https://victoriametrics.com/blog/go-sync-mutex/)
- [golang-codereviews: sync: make Mutex more fair](https://groups.google.com/g/golang-codereviews/c/72wkxOKtil0)
