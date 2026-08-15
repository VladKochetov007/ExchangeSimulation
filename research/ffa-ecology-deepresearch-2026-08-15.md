# FFA Ecology Research Basis

## Primary design constraints

- Avellaneda and Stoikov derive inventory-shifted reservation prices and
  spreads under a finite-horizon, CARA, Brownian-mid, exogenous Poisson-fill
  model. In this project it is a quote-control benchmark, not a fill oracle or
  an optimality claim. The experiment must measure empirical fill hazard by
  offset, queue position, and regime; a non-declining hazard or no inventory
  control falsifies the intended mapping.
  [Source](https://math.nyu.edu/inmemoriam/avellaneda/HighFrequencyTrading.pdf)
- Stoikov and Saglam distinguish hedgeable delta from gamma/vega risk under
  illiquidity and volatility uncertainty. Consequently the option experiment
  must record *portfolio* delta, gamma, vega, hedge fill latency, hedge cost,
  and terminal marked wealth, rather than only an option dealer's delta.
  [Source](https://people.orie.cornell.edu/~sfs33/StoikovSaglam.pdf)
- A realistic fragmented simulator should be message/event driven with
  participant-specific communication delays, rather than synchronizing agents
  on a global mid. ABIDES is a relevant design reference, not a dependency or
  validation source for this implementation.
  [Source](https://arxiv.org/abs/1904.12066)

## Venue mechanism constraints

- Allocation should be a venue rule, not a random fill. CME documents FIFO,
  pro-rata, top, split, leveling, and hybrid sequences. Pro-rata requires a
  displayed-quantity denominator, minimum-allocation policy, rounding, and a
  residual rule; a simple random allocation would study a different mechanism.
  [Source](https://cmegroupclientsite.atlassian.net/wiki/spaces/EPICSANDBOX/pages/457218521/CME%2BGlobex%2BMatching%2BAlgorithm%2BSteps)
- Fragmented markets require separate dissemination, order, cancel, and
  execution latency. A claimed latency edge must disappear when those
  latencies are equalized and must be insensitive to agent labels and IDs.
  The narrow router and latency-lab controls already enforce parts of this
  condition; the FFA campaign must extend it to every affected agent family.
  [Source](https://research-information.bris.ac.uk/en/publications/agent-based-model-exploration-of-latency-arbitrage-in-fragmented-/)

## Ecology and non-transitivity

- Market ecology is an appropriate framing for heterogeneous limit- and
  market-order strategies, but it does not turn a simulated Sharpe ranking
  into an equilibrium. Strategy performance is frequency-dependent and needs
  explicit population composition.
  [Source](https://pmc.ncbi.nlm.nih.gov/articles/PMC6296528/)
- A rock-paper-scissors claim requires a directed invasion experiment, not
  three unconditional mean PnLs. With equal initial capital and risk limits,
  estimate each candidate's risk-normalized USD marked-equity-share growth
  when introduced into the other population. Accept a cycle only when
  `g(A|B) > 0`, `g(B|C) > 0`, and `g(C|A) > 0` have held-out confidence
  intervals excluding zero and survive labels, IDs, seed, and mixture
  perturbations. Spatial/venue heterogeneity can stabilize or destabilize
  cycles and therefore must be an experimental factor, not accidental noise.
  [Source](https://arxiv.org/abs/0709.0217)

## Implications for the first build

1. Start from a tiny exact graph: three assets, three venues, one direct and
   one cross quote graph, and only a small option/future surface. Do not start
   with dozens of assets; it would hide contract, valuation, and event-order
   defects.
2. Make the information contract executable: public schedule/listing/feed to
   agents; private execution reports only to their owner; exchange-owned
   valuation only to the reporter after a run.
3. Parameterize a `VenueProfile` so FIFO and pro-rata differ by venue while
   keeping all quantities, fees, latency, and allocation settings in a signed
   scenario manifest.
4. Add strategies one family at a time. The first population is noise plus
   liquidity providers. Then add triangular/cross-venue arbitrage, then
   funding/basis and option-dealer hedging. No evolutionary selection occurs
   before each addition has a null/control ablation.
5. Treat capital selection as an outer-loop experiment. Bankruptcy,
   recapitalization, mutation, and entry rules change the game and must be
   versioned rather than smuggled into a strategy score.
