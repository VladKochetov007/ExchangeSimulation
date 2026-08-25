# V2-5 P4 funding/carry causal result

Verdict: **FALSIFIED** at the registered market basis endpoint. Links 1–5 are
identified and reproducible in both development seeds; link 6 has exact zero
effect in every qualifying venue. This is not a claim that funding is generally
irrelevant. It says the registered funding incentive and carry inventory are
not a marginal basis anchor in this V2 ecology at the frozen scale.

The protocol is
[`v2-5-p4-funding-carry-causal-preregistration.md`](v2-5-p4-funding-carry-causal-preregistration.md),
the numeric addendum is
[`v2-5-p4-funding-carry-numeric-addendum.md`](v2-5-p4-funding-carry-numeric-addendum.md),
and the mechanical gate is
[`v2-5-p4-preflight.md`](v2-5-p4-preflight.md). Machine-readable verdict and
raw paired components are in
[`p4-verdict.json`](artifacts/v2-5-p4/p4-verdict.json) and retained
`research/artifacts/v2-5-p4/pair-{107,109}.json`.

## Provenance and evidence integrity

All four cells used simulator revision `2d36b90`, multivenue binary SHA-256
`63e97b2f1e5d1b1f77e3197ce2063b9473ad251911c7236ce650e8735c40d67d`,
98 simulated hours, full evidence, and `GOMAXPROCS=4`. The analyzer SHA-256 was
`b6b58f1ffa1ea993df97dc0dd71c03be303e3d24905ce9fc857d1ef215cd7771`.

| cell | execution observations | execution hash | evidence records | evidence digest |
| --- | ---: | --- | ---: | --- |
| A-107 | 50,602,888 | `9c55552354456014991c72cbb06c7b8efb01257ac4d2635d9d1d91c665fa8567` | 51,132,088 | `78eb2a6903f43996481dc34c2610b2adfde37b905ac2ce7b9b2e3c66e0811129` |
| B-107 | 50,636,477 | `74bfa6774d6e536f7951271c148a5e65b3100b2a482561e68e735bfb5c015899` | 51,165,698 | `c587c88f1c3c941d2d1bd995326a00963347f39845d6cd826d880a3d0428cd65` |
| A-109 | 50,971,432 | `242134eb6deefc0763af4db19c506c509de46ae89f5b21321221088a4c10ec50` | 51,500,632 | `208e8cc908fcec59909159bd1697d2e07cbae70fbc41677e898a89136620a6f8` |
| B-109 | 51,011,869 | `9bff308249df8a35bf776787f713b398dd60789efef4929fb0de14f1e3261b4c` | 51,541,090 | `b9c654f9eb8637b5a4c2fce6c73298506cbd9fa99e07d9b2e7b48f07a124d843` |

For every cell, offline evidence count/digest equals the runtime artifact
exactly. Receipt/frontier replay, exact arithmetic, canonical gateway/order/fill
chains, derivative funding semantics, conservation, positions, order
lifecycle, and finite-term lifecycle all pass. Controls own no terms;
treatments independently own, activate, and close two terms per seed, with zero
terminal residual.

## Six-link result

The paired treatment clock comes only from B's first independently verified
target crossing and is applied unchanged to A. In all four qualifying
venue-seed observations, the delivered local books and funding identity are
otherwise exactly comparable.

| seed | venue | rate A→B (bps/interval) | expected funding A→B (bps/term) | exact net carry A→B (bps) | matched ordinary exposure | links 1–5 |
| ---: | --- | --- | --- | --- | ---: | --- |
| 107 | central | 1→3 | 12→36 | -16.478580→7.521420 | 0.1 ABC | pass |
| 107 | south | 1→3 | 12→36 | -16.478485→7.521515 | 0.1 ABC | pass |
| 109 | central | 1→3 | 12→36 | -16.478548→7.521452 | 0.1 ABC | pass |
| 109 | south | 1→3 | 12→36 | -16.478548→7.521452 | 0.1 ABC | pass |

A remains at zero target and submits no term. B changes to long spot/short
perpetual target, receives canonical admission and fills on both non-atomic
ordinary legs, and reaches the registered matched one-lot exposure. Thus this
is neither `NOT EXERCISED`, `NOT IDENTIFIED`, `FALSIFIED AT ACTIVATION`, nor
`FALSIFIED AT EXECUTION`.

## Registered basis endpoint

Every 30-second pre window and 300-second post window passes its coverage gate.
For every arm, venue, and seed, the exact mean oriented premium is unchanged:

```text
pre  = 20000/9999 bps
post = 20000/9999 bps
delta_A = delta_B = 0
paired_convergence = delta_A - delta_B = 0
```

Both exact equal-weight seed statistics are zero. Zero was preregistered as no
support, and neither seed has the positive convergence sign. The classification
rule therefore yields **FALSIFIED**.

The negative result is unusually clean: it is not caused by absent funding,
unchanged arithmetic, an inert participant, rejected legs, orphan inventory,
missing basis data, or numerical roundoff. The registered 0.1-ABC carry flow
changes participant inventory but does not move the canonical midpoint over the
event-study window. The V2 ecology's other quote/reference channels dominate
this marginal order flow at this scale. That diagnosis is local to this frozen
experiment and does not authorize increasing capital, shrinking depth, changing
spreads, or selecting a larger funding intervention after seeing the result.

The untouched seeds 127/131/137 are not consumed. The protocol promotes only a
complete identifiable mechanism with the registered market effect; a fully
activated but falsified basis endpoint is recorded rather than retuned.

Raw evidence remains retained. No pruning was performed.
