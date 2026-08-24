# V2-5 P2a — physical-exposure hedger activation preregistration

Status: **preregistered before P2a simulation or P2a market-data inspection.**
This is an actor-integrity and local-order-flow activation screen, not a
funding-anchor, basis-convergence, price-stability, or realism test.

## Question and design

P1a was `NOT EXERCISED`: its fee-aware four-leg carry policy found no
economically justified entry. P2 deliberately changes representation rather
than lowering that hurdle: each venue receives one bounded physical-ABC
producer/consumer exposure and may hedge that exposure with a local
`ABC-PERP` IOC. The actor has no spot leg, no funding or index read, no
cross-venue input, no privileged state, and no synthetic counterparty.

```text
A: actor installed; same physical state, account, public feed, timer,
   recorder, and evidence; order submission disabled
B: identically installed actor; submission enabled
seeds: 101, 103
horizon: 5 simulated minutes
logs: full, retained
```

`B - A` identifies only permission for the declared local hedge order. It does
not identify funding, carry, or a causal price effect.

## Fixed contract

Each venue-local actor has a reflected, deterministic external physical ABC
state with a 10,000,000 raw-ABC step every 10 seconds, a 100,000,000 bound,
and target perpetual position `-physical_exposure`. Every two seconds it
observes only its delayed local `ABC-PERP` public snapshot and, if permitted,
sends a capped 10,000,000 raw-ABC IOC at its locally delivered executable
touch. Its USD account starts with 20,000,000,000,000 raw USD wallet balance
and 10,000,000,000,000 raw USD perp margin. It pays the ordinary configured
five-bps taker fee.

The latency contract is 40-ms public market data and 20-ms request delivery:
the role profile uses a 20-ms base delay with `market_data_scale = 2`. No
other population, maker, demand, funding, price, clock, or capital setting is
changed between A and B.

## Required evidence and activation gates

Every cell must retain final `greeks.json` and `latency.json`; both are the
only completion sentinels. Before any interpretation it must also retain raw
venue logs, manifest, checkpoints, V2 receipt sidecars, and:

1. `perpexposurehedger`: independent policy, source-frontier, source-message
   fingerprint, gateway, venue outcome, fill, fee, counterparty, and IOC
   terminal replay;
2. `observationreceipts`: V2 inbox delivery audit;
3. `streamhash` and `evidenceartifacthash` provenance;
4. `conservation` and `positions` terminal mechanical checks; and
5. direct inspection of the simulator's compact `latency.json` confirmation
   of the delivered 40-ms / requested 20-ms contract.

The evidence gate is valid only if all required metrics parse and their audit
failure counts are zero. Any missing P2 decision, receipt, source fingerprint,
gateway decision, venue outcome, actor fill attestation, exact fee, external
counterparty, or required IOC terminal invalidates that cell. Raw evidence is
not eligible for pruning.

For activation, A must make physical-state transitions but submit zero hedge
orders. B must have at least one accepted hedge in **each venue** and at least
one fill-qualified hedge in **each paired seed** whose individual fill reduces
`abs(target_perp - filled_perp_position)`. Rejected or unfilled local orders
remain measured evidence, never inferred fills.

If B produces no accepted hedge in a paired seed, P2a is `NOT EXERCISED` for
that seed. If evidence is valid but B fills without reducing its actor-local
gap, the P2 mechanism is `FALSIFIED`. Neither result authorizes changing
fees, exposure size, clocks, latency, makers, demand, or funding under this
registration.

## Adversarial evidence gates already passed

The pre-run implementation gate independently catches reversed target sign,
future cached book, off-touch/cap violations, forged cached-book identity,
and dropped actor fill evidence. Receipt drop/duplication/reordering remain
covered by the V2-0 receipt auditor, which P2a requires as a hard dependency.
The short P2 fixture did not generate a partial IOC, so the specific
dropped-IOC-cancellation mutation is explicitly `NOT EXERCISED`; full P2a
logs must report whether that path occurs rather than treating its absence as
coverage. Evidence off/on across fresh processes and `GOMAXPROCS=1/4` gave
the same ordered execution hash.

## Interpretation fence

P2a can support only this narrow claim: a finite, economically named physical
exposure reaches a locally informed, ordinarily admitted, independently
replayable perpetual hedge path. It cannot support a claim about funding,
basis, carry profitability, market stability, or endogenous price discovery.
P2b is prohibited unless P2a records usable public mark/funding variation and
its own actor activation passes in both paired seeds.
