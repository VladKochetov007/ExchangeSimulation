# E0 repeated spot competition — design draft, NOT EXECUTABLE / NOT AUTHORIZED

This is a prospective *human-readable contract*, not a JSON/YAML accepted by a runner. Do not place it in `configs/`, infer seeds, or launch worlds from it. Source inspected at `ef61b4cb6e0124aa797f281e379ab143e847646e`; no current builder represents this roster exactly. [Readiness](../readiness.json) and [plan](../plan.md) control what must be built/tested first.

| Item | Proposed value and interpretation |
|---|---|
| Venue / contract | One ABC/USD FIFO price-time spot book, USD numeraire; 1 USD price tick; 0.001 ABC minimum order; positive-price domain for maker mid and return estimators. No perp, futures, options, index-anchored quote, funding or borrow. |
| Initial reference | $50,000/ABC, used only to locate finite initial seed orders and endowments. No post-initial fair-price oracle. |
| Seed-once liquidity | One separate finite actor funded with 1 ABC + $10,000; at t=0 posts 0.1 ABC bid and 0.1 ABC ask at ±10 bps of reference, each once, with enough cash/base for both. No refill, repricing, hidden depth or forced two-sided obligation. Seed contribution and PnL measured separately. The current `BootstrapDepth` refills; a new seed-once adapter is required. |
| Fees | Zero maker rebate/fee, 1 bp taker fee in USD; actual charged-asset postings must be checked. No transfer/financing or fee subsidy. |
| Maker slots | Four independent actor IDs, each initially 50 ABC + $2,500,000; quote cap 0.1 ABC per side, worst-case working `Q=B−50 ABC` within ±10 ABC including outstanding/in-flight orders. No recapitalization or borrowed ABC/USD. Initial risky holdings valued against passive endowment. |
| Pure maker | Fixed-distance policy (`S06`), half-spread 2 bp about **delivered** own-book mid; 5-s reevaluation. `RequoteBps` alone does not give a matched no-op/cancel policy in source; common quote-lifecycle adapter required. |
| AS-style maker | `S07`, 600-s *rolling* inventory-risk horizon, own-mid forward (`ForwardHalfLife=0`, no index anchor), initial log-variance `1e−8 s⁻¹` (at $50k, 25 USD²/s), 120-s EWMA half-life, 30-s sample spacing, prospective variance cap 4× initial estimate, minimum half-spread one tick, constant quote size (`QuoteSizeVolElasticity=0`), `InventorySkewBps=0`; 5-s reevaluation. Provisional source-relative risk aversion `50` and fill decay `20,000` produce a roughly $10 zero-inventory half-spread at the initial reference: `γ=50/50000=0.001 USD⁻¹`, `κ=20000/50000=0.4 USD⁻¹`, `γσ²τ=15 USD`, total spread ≈$19.99. These are **analytical matching values**, not an observed fit or locked executable inputs. `q_norm=Q/10 ABC` gives about $15 skew at the cap. Delivered-past fill-intensity estimation/declared prior and hard-cap adapter remain missing. |
| Demand | Four independent `RandomTaker` actors (`S28`), each 20 ABC + $1,000,000, spot-only, nominal size 0.1 ABC, 2-s period, `ImbalanceCoupling=0`, no excitation/tail. Actual size is drawn roughly 0.05–0.15 ABC and may be bounded by delivered visible depth. Two `RoundTripTrader` actors, each 5 ABC + $250,000, 0.1 ABC lot, 2-s tick, 0.05 opening probability on an idle tick, 5-min holding mandate. All finite, no forced fills. |
| Deployment | All maker policies share 1-s directed market-data, order and response delays, 5-s decision ticks and equal phase. Demand actors have the same directed link values, their own 2-s decision ticks. Compute service time is zero in this first controlled world; no geography claim. |
| Time | Fixed 10-min warm-up `[0,10 min]`, 45-min observation `(10,55 min]`; 1-s logical step/snapshot. Warm-up orders/balances remain in the ledger and terminal wealth; only selected rate/outcome windows exclude warm-up. Later 90-min robustness is not included. |

Proposed arms, all with the same demand, seed and total **maker-slot endowment**:

| Arm | Slots 1–4 | Intervention |
|---|---|---|
| P | pure, pure, pure, pure | Symmetric risk-limited quote control |
| A | AS, AS, AS, AS | Inventory-aware quote control |
| M1 | pure, AS, pure, AS | Mixed competition |
| M2 | AS, pure, AS, pure | Mirrored mixed slot assignment / queue-bias diagnostic |
| I | M1 policies; each maker +$2.5m **idle** cash | Capital-only negative control: working cap, quote size, clocks and all other active rules unchanged. Total capital intentionally rises; do not call it a fixed-total replacement. |

Future finite screen, **not authorized**: five arms × three prospectively chosen development seeds = 15 economic worlds, plus two technical determinism/evidence-neutrality controls. No seed IDs are assigned in this draft. Pairing is allowed only if role/slot RNG streams and common demand paths actually remain aligned. The two mixed arms are separate design cells, not independent seed replicates.

Primary outcome: for each maker and the four-maker class, fee-net `G=(C_T−C_0)+(B_T−B_0)P_T` relative to passive initial holdings, USD, with all fees already posted. The headline contrast is the paired AS-only vs pure-only change in this *tested* roster, accompanied by the full inventory-risk and valuation-availability map; not an annualized return or guaranteed dominance claim. If `P_T` is unavailable under the registered strict mark contract, `G` is unavailable and the world remains a reported economic/valuation outcome. Secondary: maker-specific fills, cash/base/fees, inventory distribution/max drawdown, resource-denied decisions, quote survival, class resting depth, one-sided duration, spread/volume and markout with its descriptive label. Do not condition the primary estimate on only surviving/two-sided cells without reporting the changed denominator.

Readiness tests must prove: spot-only assembly; one-time seed; no undesired cancellation/replenishment; fixed policy-specific quotes on identical delivered history; exact AS normalized inventory math; shared pending/live-inclusive hard cap; equal admission/fee/deployment; no future-state reads; all account/fee transfers reconstruct; raw stream corruption fails closed. Source changes and a prospectively reviewed executable protocol are required before a world. The draft values may be changed **only before** any new outcome and with a named amendment; they are not immutable preregistration values today.
