# Economic ecology audit

For every participant class: what it is for, what it actually did, what it
earned, and whether it exists for an economic reason or only to keep a book
alive.

**Provenance.** The activity table is measured on a 30-minute run of the
frozen population and the profit table on the same run. Both predate the
re-freeze at `01c9ceb`, so they are provisional under V-003 and are being
repeated on the deterministic re-runs. The classifications and the questions
they raise do not depend on the exact numbers.

## What each class did

Measured with `mvanalyze -metric roleaudit`: liquidity provided against
liquidity taken, how many books the class touched and how concentrated it was,
its net position as a share of its gross volume, and how often the venue
refused its orders.

| class | n | maker fills | taker fills | taker share | books | top book | net/gross | rejected |
|---|---|---|---|---|---|---|---|---|
| spot_maker | 4 | 56,894 | 500 | 0.9% | 2 | 0.99 | 0.009 | 0 |
| cdf_spot_maker | 2 | 15,665 | 0 | 0.0% | 1 | 1.00 | 0.008 | 0 |
| abc_cdf_spot_maker | 2 | 18,396 | 1,030 | **5.3%** | 1 | 1.00 | 0.060 | 0 |
| perp_maker | 1 | 4,962 | 2 | 0.0% | 1 | 1.00 | 0.000 | 0 |
| futures_maker | 2 | 7,814 | 1 | 0.0% | 2 | 0.52 | 0.112 | 0 |
| fixed_distance_maker | 6 | 16,557 | 28 | 0.2% | 3 | 0.74 | 0.021 | 0 |
| imbalance_maker | 8 | 13,060 | 158 | 1.2% | 4 | 0.70 | 0.006 | 0 |
| option_dealer | 3 | 14,714 | 3,609 | 19.7% | 21 | 0.20 | 0.159 | **512** |
| noise_flow | 6 | 0 | 38,157 | 100% | 4 | 0.40 | 0.026 | 1 |
| latent_liquidity | 6 | 0 | 14,622 | 100% | 1 | 1.00 | 0.053 | 0 |
| elastic_supplier | 8 | 0 | 11,382 | 100% | 1 | 1.00 | 0.361 | 1 |
| metaorder_trader | 6 | 0 | 9,633 | 100% | 1 | 1.00 | 0.037 | 0 |
| round_trip | 8 | 0 | 612 | 100% | 1 | 1.00 | 0.002 | 0 |
| option_flow | 4 | 0 | 11,766 | 100% | 22 | 0.07 | 0.420 | 0 |
| option_value_taker | 4 | 0 | 2,279 | 100% | 13 | 0.25 | **0.869** | 0 |
| future_flow | 3 | 0 | 5,136 | 100% | 2 | 0.50 | 0.364 | 0 |
| triangle_arb | 2 | 0 | 42,126 | 100% | 3 | 0.52 | **0.320** | 0 |
| carry_arb | 2 | 0 | 1,782 | 100% | 2 | 0.55 | **0.000** | 0 |
| dated_carry_arb | 2 | 0 | 2,711 | 100% | 3 | 0.57 | 0.110 | 0 |
| parity_arb | 2 | 0 | 993 | 100% | 15 | 0.36 | 0.158 | 0 |
| vanna_volga_desk | 3 | 0 | 1,535 | 100% | 10 | 0.47 | 0.249 | 0 |

## What each class earned

Change in marked equity over the run, against starting equity. Marked equity
depends on prices, so a class holding an asset whose price fell loses without
having traded badly; the column is a description of outcomes, not of skill.

| class | return | starting equity |
|---|---|---|
| abc_cdf_spot_maker | **+0.0199%** | 6.78e14 |
| triangle_arb | **+0.0109%** | 2.22e14 |
| option_dealer | +0.0005% | 5.85e14 |
| round_trip | −0.0046% | 2.65e13 |
| fixed_distance_maker | −0.0105% | 8.88e14 |
| imbalance_maker | −0.0127% | 8.88e14 |
| carry_arb | −0.0154% | 2.42e14 |
| parity_arb | −0.0155% | 5.10e14 |
| dated_carry_arb | −0.0166% | 5.10e14 |
| latent_liquidity | −0.0175% | 1.35e16 |
| cdf_spot_maker | −0.0222% | 6.78e14 |
| futures_maker | −0.0223% | 6.78e14 |
| perp_maker | −0.0233% | 3.39e14 |
| spot_maker | −0.0232% | 1.36e15 |
| option_value_taker | −0.0243% | 5.18e14 |
| future_flow | −0.0249% | 3.80e14 |
| option_flow | −0.0251% | 5.06e14 |
| metaorder_trader | −0.0267% | 1.85e14 |
| elastic_supplier | −0.0349% | 3.61e15 |
| noise_flow | −0.0881% | 7.60e14 |

Nobody makes much and nobody is destroyed. The spread between the best and
worst class is about one basis point of equity over thirty minutes. No class
has a guaranteed edge, which is one of the things the plan asks to establish —
though it is established here only over a short window, and the arbitrage desks
are on the profitable side of it.

## Findings

**A market maker that crosses the spread.** The cross-pair maker takes
liquidity on 5.3% of its fills, against 0.0–1.2% for every other maker. That is
the signature of the instability behind V-001 and V-002: its reservation price
moves far enough against inventory that its own requote crosses the standing
book. It is also the most profitable class in the run, which means the
population's best performer is the participant whose quoting rule destabilises
its own book.

**An arbitrageur that is not flat.** The triangular desk carries a net position
equal to 32% of its gross volume. An arbitrageur closing three legs should end
near flat; a third of its turnover is directional risk it did not intend to
take. Its legs do not fill in equal size — 2,584 firings produced 2,549, 2,606
and 2,559 fills across the three books — so the desk is a directional trader
wearing the name of a hedge. The carry desk, by contrast, is exactly flat
(0.000), which shows the measurement can distinguish the two.

**A value taker that only ever takes one side.** The option value takers run
87% net directional. They are supposed to buy what they think is cheap and sell
what they think is dear; a share that high means their model disagrees with the
dealers' in one direction almost always, which is a statement about the two
volatility models rather than about value. Their SABR view against the dealers'
realised-volatility estimate is a systematic bias, not a disagreement that
cuts both ways.

**The only class the venue ever refuses is the option dealer** — 512
rejections against 0 or 1 everywhere else. A dealer quoting a chain of 21 books
against finite collateral is the one participant whose strategy the venue's
margin rules actually bind.

**Nothing is ever liquidated.** No participant approaches insolvency in
24 hours (V-005), so none of the classes faces a real capital constraint. Their
"finite capital" is finite in principle and unbinding in practice.

## Motive classification

The plan asks whether each liquidity-demand class trades for an economic reason
or exists to keep a book alive.

| class | motive | verdict |
|---|---|---|
| elastic_supplier | downward-sloping demand: sells as the price rises, buys as it falls | economically legitimate, and the V-002 ablation shows it is what damps the maker's feedback loop |
| round_trip | opens a position and unwinds it after a fixed hold | legitimate: mean-reverting demand in quantity, which is what returns a maker's inventory |
| metaorder_trader | executes a parent order over time under a participation cap | legitimate: an execution objective |
| carry_arb, dated_carry_arb, parity_arb | relative value between two instruments on the same underlying | legitimate |
| triangle_arb | relative value across three books | legitimate in intent; see the finding above on what it actually does |
| option_dealer, vanna_volga_desk | quote and hedge, earn spread against inventory risk | legitimate |
| option_value_taker | trades its own valuation against the dealers' | legitimate in intent; the one-sidedness above weakens the claim |
| latent_liquidity | posts and takes size that is not otherwise represented | **suspect**: it exists to add depth, and its motive is not stated in terms of any exposure or objective |
| noise_flow | uninformed random-side flow | **simulation support by construction.** It is the standard modelling device for uninformed demand, and it is not derived from any objective |
| option_flow, future_flow | uninformed flow restricted to a contract class | **simulation support**, and `future_flow` was added precisely because the dated ladder was otherwise starved (L-006) |

Three classes — `noise_flow`, `option_flow`, `future_flow` — plus
`latent_liquidity` exist to produce activity rather than to pursue an
objective. `future_flow` is the clearest case: it was introduced during the
liveness campaign because the dated books were dying, which is exactly the
pattern the plan warns about. That the benchmark then passed is not evidence
about the market; it is evidence that the flow was added.

## What this audit still owes

- The same tables on the deterministic re-runs at the re-frozen commit.
- Wealth share over time rather than at a single horizon, and the long-run
  attractor over many more cycles.
- Per-class PnL decomposed into spread capture, adverse selection, fees and
  funding, rather than the single equity delta reported here.
- Vanna-volga desks measured on whether their vega, vanna and volga exposures
  actually fall after a hedge cycle, which is the only thing that would show
  they do what they are named for.
