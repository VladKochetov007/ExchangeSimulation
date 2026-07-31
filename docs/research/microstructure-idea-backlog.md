# Idea backlog: microstructure and market effects worth simulating

Generated while the latency-race experiment ran. The existing
[realism roadmap](realism-roadmap.md) is organized around *mechanism
fidelity* — making the exchange behave like Binance or Deribit. This list is
organized around a different question: **which participant assumption
produces which market effect**, and what the cheapest experiment is that
would show it. That framing is the one the eventual write-up cares about, and
several of these are now unblocked by work that has already landed.

Each entry states the mechanism, the effect it should produce, and the
falsification condition — the observation that would say the mechanism is
wrong, not merely absent.

## Unblocked by the reactive-actor work

The latency race turned the actor loop from "decide on a timer" into "decide
on an event." That change is a prerequisite for a whole family of effects
that were previously unreachable, because anything that depends on *who
reacts first* was previously decided by ticker phase.

**1. Quote fading / mechanical liquidity erosion.** Give market makers the
same reactive treatment the arbs just got: cancel on adverse trade prints
rather than on a refresh timer. Effect: depth should evaporate ahead of a
large metaorder, so measured impact exceeds what the pre-trade book implied.
This is the mechanism behind "the liquidity was never really there," and it
is the honest counterpart to the square-root-impact experiment already on the
roadmap — impact measured against a book that cannot flee is not the impact
real executions face. *Falsified if* depth ahead of a sweep is unchanged when
MMs go reactive, which would mean the MM cancel path, not its cadence, is the
binding constraint.

**2. Adverse selection as a measurable quantity, not an assumption.** With
reactive MMs, tag every MM fill with the mid movement over the following N
seconds and report the distribution. Effect: a quantified markout curve, so
the MM's spread can be compared against the adverse selection it actually
suffers rather than against a configured constant. This is the number that
makes "is this spread economically justified" answerable, and it feeds
directly into the fee/rebate experiments, which are currently uninterpretable
because nothing in the ecology knows anything.

**3. Latency-race concentration and the batch-auction comparison.** Now that
capture orders by speed, the Budish–Cramton–Shim experiment is one config
away: run the same speed-tiered ecology under continuous matching and under a
periodic uniform-price auction, and compare arb capture, spread, and MM
markouts. Effect: if the theory holds, batching should collapse the speed
advantage to roughly nothing while tightening quoted spread. *Falsified if*
capture stays speed-ordered under batching, which would point at the sim
leaking a timing edge the batch is supposed to erase.

## Confirmed while testing the latency race

**0. Correlated hedging demand (crowded second leg).** Measured, not
proposed. When competing arbitrageurs leg into a trade sequentially, they all
react to the same print, all take the first leg, and all turn to the second
book at the same instant. Residual exposure runs 9.8% of gross for a lone
arbitrageur and 26.7% with four — competition costs more than the delay it
was supposed to avoid. No coordination is required; a shared signal is
enough to align the demand. Worth generalizing: the same experiment with
delta-hedging option market makers should show the gamma-feedback channel
amplified by exactly this mechanism, since short-gamma dealers all need the
same side at the same time.

**0b. Queue position beats patience against a resident market maker.**
Measured. A passive order joining the touch fills 16% of the time in one
second; the same order posted one tick inside fills 44% — the same rate that
five seconds of waiting achieves, at half the directional exposure and five
times faster. Better on both axes, because joining the touch queues behind
the market maker's resting size while improving steps in front of it. The
generalization worth testing: sweep the improvement in ticks and find where
the marginal tick stops paying for itself, which is the empirical price of
queue priority in this ecology and should scale with how much size the
resident MM keeps at the touch.

## Cheap effects the current engine can already produce

**4. Inventory-throttle saturation (in progress).** Already observed: capture
peaks at the 0.2× tier rather than the fastest. If the `MaxPosition` sweep
confirms the inventory explanation, the general claim is that *risk limits,
not speed limits, decide who wins a mature race* — which is a more
interesting statement than "faster is better" and is testable against the
real-world observation that HFT capture is bounded well below what pure speed
advantage would predict.

**5. Epps effect.** Measure cross-asset correlation as a function of
sampling interval across the ABC/Q pair. Effect: measured correlation should
rise with the sampling window purely from asynchronous trade arrival, with no
change to the underlying dependence. Nearly free — it needs an analyzer
metric and no new agent — and it is a good validation target precisely
because no parameter was tuned against it.

**6. Order-flow imbalance as a predictor.** Regress short-horizon returns on
signed order-flow imbalance. Effect: a positive, decaying coefficient. Also
free, also out-of-sample, and it doubles as a sanity check that the price
formation is not degenerate.

## Requires the spoofing agent (not yet built)

**7. Spoofing against a herding population.** The ecology already has an
imbalance-coupled taker whose flow tilts toward visible book pressure, and
that coupling was previously shown to be a death-spiral amplifier. A spoofer
that posts large non-crossing depth to bias the visible imbalance, then pulls
it, is therefore attacking a mechanism known to exist here. Effect: the
spoofer should profit *only* when the herding coefficient is non-zero, and
profit should scale with it. This is the cleanest available demonstration of
"a participant assumption creates an exploitable effect," because the
exploited assumption is a single configurable number.

*Falsified if* the spoofer profits with the herding coefficient at zero,
which would mean it is exploiting something else — most likely the MM's own
book-derived fair value, which is itself worth knowing.

**8. Spoof detection as an ecology response.** Once spoofing pays, give MMs a
cancel-rate-aware fair value that discounts depth from clients with a high
cancel-to-fill ratio. Effect: an arms race with a fixed point — the spoofer's
edge should decay as detection strengthens. This is the first genuinely
*adaptive* interaction in the ecology and the closest thing to the
predator-prey cycling the market-ecology literature predicts.

## Structural, higher cost

**9. Cross-exchange listing.** The `Mount` abstraction already supports N
venues with independent latency; no simulation has ever assembled two. Effect:
genuine cross-venue basis, and a latency-arb strategy whose edge depends on
the *relative* latency of two connections rather than one. This is also the
setting where index construction starts to matter, since a basket index over
two venues is exactly what the `BasketIndex` work was for.

**10. Fragmentation and venue choice.** With two venues live, give takers a
routing rule and vary the fee schedules. Effect: order flow should migrate,
and the price discovery share of each venue should follow — the classic
fragmentation result. This is the experiment that most directly answers "what
is profitable to adopt given what everyone else is doing," because the
routing rule is a strategy and its payoff depends on the population's routing.

## Notes on what would make results trustworthy

Two recurring hazards, both already encountered:

- **Tuned metrics are not evidence.** Fano factor and kurtosis were produced
  by tuning the excitation parameter, so reporting them as realism is
  circular. The out-of-sample facts — impact exponent, sign-ACF decay,
  markout curves, the Epps shape — are the ones worth quoting.
- **Aggregate invariants hide their own mechanism.** The conservation scare
  during this experiment took far longer to resolve as "the number is wrong"
  than it would have as "instrument the per-trade deltas and read what
  happened." Any effect claimed below should come with a per-event trace that
  shows the mechanism, not only an end-of-run statistic.
