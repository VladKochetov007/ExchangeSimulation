# Deployment, delivery and clocks

Read only for a timing or ordering claim. The current ME-001 composition pilot
does not require a geography engine or a finer clock merely to use this skill.

## Reconstruct the actual path

Exchange event/publication -> market-data transport -> actor receipt/callback
eligibility -> decision wait/computation -> order send/transport -> venue
processing/matching -> acknowledgement/fill transport -> actor receipt.

Record both configured and realized intervals, simulation resolution, periodic
decision cadence/phase, quote lifetime and opportunity lifetime. A nominal
latency change can disappear under quantization or scheduling. Test neighboring
phases/resolutions with appropriate future authorization before declaring
latency irrelevant. An earlier execution delivered after cancellation retains
its earlier match time and later receipt time.

## Layers and progression

A directed path can contain propagation, serialization, queueing, gateway
processing and additional transport. Do not double count components already
included in a measured total.

- L0: equal/zero deterministic delay as a construction control.
- L1: fixed asymmetric actor-to-venue profiles for a clean comparison.
- L2: declared jitter, shared-link bursts and processing queues.
- L3: geographical or measured directed paths when the claim needs them.

Policy, actor and deployment are separate. Compare identical policies/resources
under swapped or randomized deployment assignments. Bind stable RNG namespaces
and deterministic ties; expose changes in actor IDs and queue priority.
Keep order instructions, venue allocation and routing as separate interventions.

## Geography and measurement

Use actual venue/data-center endpoints if known. A remote owner may deploy a
colocated process; corporate headquarters and home addresses are not endpoints.
Distance/speed is a propagation lower bound, not measured total latency.
RTT/2 assumes symmetry; it is not an observed one-way delay.
Label synthetic colocated/regional/interregional profiles as synthetic.
Do not assert measured London/New York/Tokyo connectivity from guessed distances.

Respect nonnegative delays, route correlations, transport ordering and
head-of-line effects where modeled. Independent per-message jitter is unsuitable
when it violates the declared transport. Use logical events, not real sleeps.
If host computation profiling provides a delay input, pin that measurement
separately; do not use live wall-time variation as an economic clock.
