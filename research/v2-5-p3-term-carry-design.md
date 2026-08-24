# V2-5 P3 design — term carry allocator with realized lifecycle

Status: **P3 core, receipt boundary, and independent lifecycle replay are
implemented; P3a's five-minute development integrity screen has completed.**
It follows P1a `NOT EXERCISED`, P2a's narrow local-hedge activation pass, P2
public-signal readiness, and lifecycle finding C-001. P3a supports only
locally informed, ordinary matched entry; it is not a funding, basis, or
realized-carry result. See
[`v2-5-p3a-term-carry-results.md`](v2-5-p3a-term-carry-results.md).

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

Partial IOC legs remain in their actual state. A locally unavailable or
sub-minimum first leg does not create a plan at all; a rejected or zero-fill
cancelled flat first leg deterministically resets to `IDLE`, so it cannot
accumulate fictional carry time. Once any leg fills, its real exposure remains
in the entry/repair path and cannot be erased. At term end the actor first
unwinds the perpetual, then the spot position. An unavailable local executable
price yields an observable `UNWIND_PRICE_UNAVAILABLE` defer and deterministic
retry; it does not renew the term, use a hidden last price, or turn numeric
zero into unavailable. A participant-known `mandate_end_at_nano` may bar a
new term that cannot include its declared close; it is serialized policy,
not the simulator's hidden run termination time. With no mandate, a short
technical screen may end while a term remains explicitly open/censored and
cannot make an economic carry claim.

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
  before/after, explicit plan-creation time, canonical first-exposure time,
  term end, actual next funding time, and close deadline;
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

### Current implementation gate

The P3-0 direct fixtures now cover one finite four-leg term, a term-end
unwind defer and recovery, directional exact financing, a valid present-zero
funding input, flat first-leg deferral, and rejected flat entry reset. The
independent evidence replay verifies source frontiers, exact financial terms,
gateway/venue/actor order chains, position continuity, active-term funding
settlement attribution, terminal perpetual position, terminal base-asset
balance delta, and closure cardinality. Its mutations catch a missing close as
an open term, duplicate close, funding outside an active term, a dropped unwind
fill, and a one-unit terminal spot-balance mismatch. Evidence ON/OFF fresh
process × GOMAXPROCS neutral checks are complete for the terminal-censored
short helper. P3a's immutable short-world config, activation contract, and
full-evidence analysis command were committed before its market cells ran.

P3a retained its v1 plan-time field because the five-minute screen only claims
ordinary matched activation. P3b and later cells emit policy version
`v2_5_p3_term_carry_v2`: `plan_created_at` identifies the pre-ingress plan and
`first_exposure_at` identifies the first actual canonical fill. A flat
rejected/cancelled plan is aborted rather than counted as an open ownership
term. The independent replay rejects a forged first-exposure timestamp and
continues to parse v1 evidence only as historical evidence.

Required mutations: reversed funding sign; dropped/delayed/duplicate/reordered
funding or book receipt; a valid zero incorrectly treated absent; a forged
term/exit time; a missing entry/unwind fill or IOC cancellation; a skipped
unwind; double unwind; a funding payment outside the active term; and an
evidence-on/off execution-hash comparison across fresh processes and relevant
GOMAXPROCS values.

The original hidden `TerminalNano` gate was removed from P3 before a market
cell: giving a participant the simulator stop time would violate the local
information contract and made a five-minute activation cell impossible. The
replacement is the declared, persisted treasury mandate deadline above. A
short P3a technical screen either uses no mandate and is terminal-censored in
analysis, or a separately declared real mandate; neither is scored as
realized term carry.

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

## P3c outcome — do not bypass this gate

P3c is complete and **falsified the current finite-term exit policy**; its
retained result is
[`v2-5-p3c-term-completion-results.md`](v2-5-p3c-term-completion-results.md).
Two locally informed 10m matched terms reached their declared end, but the
perpetual cover touch held only 16,286 (central) and 16,348 (south) raw units,
below the registered 100,000 minimum child size. The actor emitted 7,200
explicit `EXECUTABLE_SIZE_UNAVAILABLE` deferrals over the two-hour close
window, submitted no unwind order, retained both positions, and south received
one funding transfer after its term window. Information-boundary, accounting,
generic funding, and order-lifecycle gates are clean; this is a policy/ecology
failure rather than evidence corruption.

Do not rerun P3c or alter its registered parameters. A later P3 exit-liquidity
revision must be separately designed and preregistered: it needs a named
observed-depth child-order/participation policy, finite residual-risk rule,
activation metric, partial-fill and deadline mutations, and a fresh lifecycle
screen before any funding/basis market comparison.

This actor can establish a causally inspectable response to its **declared
funding-persistence belief**. It cannot by itself show that funding is an
endogenous general-market anchor, that the belief is realistic, or that
basis convergence emerges without imposed participant priors.

## P3d correction — do not score the exit-floor contrast

The separately registered P3d attempt is retained as
[`v2-5-p3d-exit-liquidity-results.md`](v2-5-p3d-exit-liquidity-results.md).
It is **INVALID / NOT SCORED**, not a successful recovery from P3c: the
assumed actor-only 100,000-unit floor was the actual ABC venue minimum. Its
zero-floor B treatment sent sub-minimum children, which the exchange rejected.
The implementation and replay now prevent that category of invalid child;
P3's remaining live limitation is future exit capacity, not numeric
price/quantity availability.
