# V2-5 P3 design — term carry allocator with realized lifecycle

Status: **design only; no P3 code or P3 market world exists.** This is the
next permissible V2-5 representation slice after P1a `NOT EXERCISED`, P2a's
narrow local-hedge activation pass, P2 public-signal readiness, and lifecycle
finding C-001. It does not alter an earlier V2 result.

## Observed failure and new representation

The current `FundingCarryArbitrageur` estimates a configurable funding horizon
but has no matching ownership/unwind lifecycle. Increasing its horizon would
therefore change an expected-income number without committing the actor to the
corresponding holding period. P3 introduces a new, opt-in **term carry
allocator** rather than extending the old actor.

The allocator represents a finite-capital treasury that can commit a bounded
cash/asset inventory to a declared 96-hour (12 × eight-hour funding interval)
hedged carry term. This is a development/calibration assumption, not a claim
that real funding is constant for four days. Its expectation is explicit:
the most recent locally delivered rate is held constant for exactly the
declared number of future intervals. Any resulting carry response is therefore
conditional on that participant prior, not an emergent forecast.

Twelve intervals is a fixed design value chosen *after* P1a as a new
representation: at the observed 1–3-bps per-interval development signal, one
interval cannot cover the registered four-leg fee/risk costs, while a named
multi-day treasury term can. It is not a retuned version of P1a and must be
labelled calibration/development in every later score.

## Local economic contract

```text
delivered local spot/perp books + delivered local funding snapshot
  → declared 12-interval funding-persistence belief
  → exact direction-specific financing + fee + balance-sheet + risk estimate
  → finite desired matched spot/perp inventory
  → ordinary non-atomic IOC entry legs
  → actual funding settlements during a fixed term
  → ordinary non-atomic IOC unwind at term end
```

For positive funding/premium the intended pair is long spot / short perp; for
negative funding/premium it is short spot / long perp. The allocator has no
global mark, index, funding, price, PnL, or cross-venue read; every source is
joined to its participant-local V2 receipt frontier. It writes neither a mark
nor a funding rate and obtains no fee subsidy, reserve exemption, forced fill,
or synthetic counterparty.

Unlike P0/P1, it has explicit states:

```text
IDLE → ENTRY_SPOT → ENTRY_PERP → ACTIVE_TERM
     → UNWIND_PERP → UNWIND_SPOT → IDLE
```

Partial/rejected/cancelled IOC legs remain in their actual state. An entry
whose first leg cannot become a matched pair follows a named `ENTRY_REPAIR` or
`ENTRY_ABORT_UNWIND` path; it is never booked as a carry holding. At term end
the actor first unwinds the perpetual, then the spot position. An unavailable
local executable price yields an observable `UNWIND_PRICE_UNAVAILABLE` defer
and deterministic retry; it does not renew the term, use a hidden last price,
or turn numeric zero into unavailable. A terminal horizon that cannot include
the declared term plus its two-leg close bars a new entry.

## Exact economics and domains

P3 replaces P1a's whole-bps annual-borrow rounding with independently
replayable rational arithmetic. Funding income, directional financing,
four-leg execution fee estimate, balance-sheet charge, margin-risk charge,
leg-risk charge, and minimum return are preserved as separately named terms.
The policy compares exact rational net carry with its declared hurdle; evidence
contains sufficient numerator/denominator or exact fixed-point components for
an independent replayer. It must not silently round a nonzero borrow cost to
zero.

Long-spot cash financing and short-spot asset borrow are distinct declared
cost inputs. A signed price is an available numeric value, but this crypto
spot/perp contract remains positive-price only; missing local touch is an
explicit unavailable result, while a present non-positive touch is a declared
domain reject. Funding/basis ratios are unavailable when their mathematical
denominator is zero, not zero-valued economic inputs.

## Evidence and independent audit contract

Each decision and lifecycle transition must persist:

- allocator/venue/client identity, finite capital limit, policy version, state
  before/after, entry time, term end, actual next funding time, and close
  deadline;
- all local book/funding identities and the exact decision frontier;
- signed target pair, actual two-leg positions, matched quantity, orphan
  quantity, entry/unwind reason, request IDs, accepted/rejected/fill/cancel
  outcomes, and every funding settlement attributed to the active term;
- exact expected-income and every cost component, with explicit availability
  and mathematical-domain status; and
- deterministic terminal-censoring reason when a pending term cannot be
  completed within the configured world.

A new analyzer, independent of the actor and multivenue package, must replay
the state machine, local source frontier, rational comparison, all non-atomic
order chains, active-term funding payments, and exactly one eventual unwind.
It must separately reconstruct closed positions and conservation; it may not
infer a pair merely because two leg intents were emitted.

Required mutations: reversed funding sign; dropped/delayed/duplicate/reordered
funding or book receipt; a valid zero incorrectly treated absent; a forged
term/exit time; a missing entry/unwind fill or IOC cancellation; a skipped
unwind; double unwind; a funding payment outside the active term; and an
evidence-on/off execution-hash comparison across fresh processes and relevant
GOMAXPROCS values.

## Staged experiments and kill criteria

1. **P3-0 deterministic fixtures.** Positive/negative rate mirrors, zero
   funding, exact fractional financing, partial entry, unavailable unwind,
   delayed recovery, one funding settlement, and exactly-one close. No market
   score.
2. **P3a five-minute A/B actor-integrity screen.** Installed/recorded actor
   with submission disabled/enabled; no funding/basis score. It must show
   legal local source use and at least one matched inventory-changing pair in
   a declared development cell, or is `NOT EXERCISED` without retuning.
3. **P3b nine-hour realization cell.** A bounded active term must survive one
   ordinary funding instant, with independently reconstructed payment and
   continued matched inventory. It does not yet test the 96-hour close.
4. **P3c term-completion cell.** A horizon long enough for the declared 96-h
   term, funding events, and close retries must show one eventual unwind,
   closed positions, and conservation. This is run only after P3a/P3b pass and
   with retained full evidence.
5. **Only then**, preregister a paired market comparison. Inventory/orders are
   scored before funding response, basis, or price metrics. No inventory/order
   change is `NOT IDENTIFIED`, regardless of an attractive basis chart.

This actor can establish a causally inspectable response to its **declared
funding-persistence belief**. It cannot by itself show that funding is an
endogenous general-market anchor, that the belief is realistic, or that
basis convergence emerges without imposed participant priors.
