# V2-5 P3d — exit-liquidity preflight

Status: **PASSED (configuration/evidence preflight only).** These retained
five-minute worlds establish that both immutable P3d v3 configurations decode,
persist their explicit exit-floor policy, preserve the information boundary,
and replay independently. They do **not** reach a 96-hour term end and make no
claim about exit activation, close completion, funding anchoring, carry PnL,
basis, or realism.

The preregistration is
[`v2-5-p3d-exit-liquidity-preregistration.md`](v2-5-p3d-exit-liquidity-preregistration.md).
The compact machine-readable result is
[`preflight-107-verdict.json`](artifacts/v2-5-p3d/preflight-107-verdict.json).
Raw evidence and extracts remain retained under
`research/artifacts/v2-5-p3d/preflight-{legacy,unit}-107/` and are not
eligible for pruning.

## Attempt-zero configuration correction

The first invocation of each preflight was rejected before `NewSim` by strict
configuration decoding: an unsupported root-level `causal_arm` provenance key
produced `json: unknown field "causal_arm"`. It created no simulator, raw log,
checkpoint, sidecar, or scientific evidence. Commit `18886e6` removed only
that unsupported non-economic key; the arm identity remains in the immutable
filename, `experiment_id`, and preregistration. The subsequently run config
hashes are the ones below. This failed parser invocation is historical
configuration provenance, not a P3d observation.

## Provenance and mechanical readiness

| item | legacy A | exchange-unit B |
| --- | --- | --- |
| config / SHA-256 | `configs/v2-5-p3d/term-exit-legacy-107.json` / `cbb18d86752b59be3000998f8832851305f29f5cd2c440190d71039f9658ca76` | `configs/v2-5-p3d/term-exit-unit-107.json` / `3b5844597b5f89651888f4433183a118c4a842a6adf43dea903d37d86a97ed37` |
| sole policy delta | `unwind_min_order_size=100000` | `unwind_min_order_size=0` |
| source revision in run manifest | `6310b438f8504539926076e4072a72ddfc493ab7` | same |
| binary SHA-256 (sim / analyzer / gate) | `c68c93cbb7ee539bec239f3557b7337d648d16330730cff08c4ee7a5110bc1fb` / `f22c6f6dab5985a4867b4f41787717cfe82000280b9dd6469a6f0c4c1fda4625` / `272bef32dd0b136626e4f8b701334958c1dff1e00bf4ad518895600ce2c66282` | same |
| seed / horizon / process setting | 107 / 5 simulated min / `GOMAXPROCS=4` | same |
| final execution observations / ordered hash | 56,189 / `f8bf4844729c762b76097ac44cf664c5148c3e6a853dcb157ca9c86b0d93d1c2` | **identical** |
| persisted evidence records / multiset digest | 56,648 / `0d5540e52f9f651e584f99035fe837d09ca0634ff882b845eb4684ebce2486a5` | 56,648 / `2eda288715b04e883d0cac8877e61dc0d9f16cba387b26987b30bc27c47f02f5` |
| exact JSON-record artifact digest | 56,648 / `bcefaf13f7e82a6f202ffc9f8175b3462637fc4cff15903265994cbe64be6bda` | 56,648 / `91a4cc89f0b06e22fac03d5bd0c00d6c7e40d13137a7cda518184f5d87d3132f` |

The manifest marks the worktree modified solely because preserved, pre-existing
baseline-scoreboard artifacts were already dirty. No simulator source is
uncommitted; the P3d implementation and independent replay are committed at
`9d20c15` and `6310b43`. Commit `18886e6` changes only run-config provenance.

Both final-only sentinels (`greeks.json`, `latency.json`) are nonempty. The
P3 replay is valid in both arms: 450 decisions, four ordinary submitted and
accepted orders, five fills, two valid active terms, zero close attempts in
this intentionally pre-expiry horizon, and zero source/frontier/gateway,
policy, arithmetic, lifecycle, position-continuity, or terminal-account
mismatches. Receipt replay is valid with 2,679 schedules, 2,670 deliveries,
and four audited decision frontiers. The three allocator links each record
40-ms delivered market data and 20-ms request/response delivery.

The equal ordered execution hash shows that the new evidence policy is not an
actor-visible input before expiry. The different persisted-evidence digests
are expected: all ordinary market-event class counts agree, while the
`term_carry_decision` payloads deliberately attest the distinct explicit v3
exit-floor value. This is an evidence-domain distinction, not a pre-expiry
economic divergence.

## Admission decision

The registered 98-hour full-evidence A/B cells may now run from the committed
configurations. Their raw evidence must be retained through every listed
extract and the P3d score. P3c remains historical-only; it is not substituted
for P3d A because v3 decision evidence is intentionally versioned.
