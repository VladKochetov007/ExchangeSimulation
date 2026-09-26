# ME-005 exploratory public-stage diagnostic — retained evidence only

Status: **OFFLINE EXPLORATORY DECOMPOSITION; REGISTERED VERDICT UNCHANGED**.
No simulator world, seed search, fee/capital change, new route or holdout was
run. The original [protocol](protocol.md), four-cell [reviewed report](report.md)
and [machine result](result.json) remain authoritative. Their registered
finding is valid evidence with zero positive one-lot fee/depth-adjusted
two-sided public opportunities, no router orders, and unestimated arbitrage
profitability/convergence.

## Where the nested public funnel empties

The new Go diagnostic applies the registered two-sided public-book requirement
in canonical event order. For each of the two directional buy/sell routes it
then asks: are both policy-required books present with positive executable
leg prices; is sell bid above buy ask before fees; do both displayed legs
have at least the registered 0.01-ABC lot; and is the fixed-point,
quote-fee-adjusted edge positive? Episodes are maximal directional intervals;
same-timestamp starts/ends count as zero-duration episodes. Durations sum
over *directions*, so they are not elapsed wall time of one 300-s world.

| Seed | Arm | Event-ordered public transitions assessed | Two-sided leg-price directional episodes / summed duration | Gross crossings | One-lot-depth crossings | Fee-positive crossings | Actual ON evaluations / submissions |
|---:|:---:|---:|---:|---:|---:|---:|---:|
| 1109 | OFF | 800 | 2 / 596 s | 0 | 0 | 0 | N/A / N/A |
| 1109 | ON | 802 | 2 / 596 s | 0 | 0 | 0 | 783 / 0 |
| 1117 | OFF | 809 | 2 / 596 s | 0 | 0 | 0 | N/A / N/A |
| 1117 | ON | 811 | 2 / 596 s | 0 | 0 | 0 | 790 / 0 |

The first zero is **gross bid/ask crossing conditional on the router's
two-sided public-book requirement**, not fee insufficiency, insufficient lot
depth or lack of router capital. No observed state reached those later filters.
The 596 s is the sum of two route-direction exposures to an eligible
two-sided quote state, not a 596-s market interval. A gross crossing that
would involve a one-sided non-policy book was not the denominator measured
here; the accepted registered result separately reports zero broader
*fee-positive* leg-side episodes. This diagnostic does not assert zero gross
crossings under every alternative public-state definition.

With zero registered fee-positive public episodes, conditional funding
feasibility, delivery of a qualifying episode, actor response to such an
episode, and per-attempt profit remain **not estimated**. The 1,573 actual
ON evaluations and zero submissions are observed activity, not a denominator
of 1,573 economic arbitrage opportunities. No opportunity disappearance
cause or counterfactual convergence effect is identified. This decomposition
does not strengthen ME-005 into a general claim that cross-venue arbitrage
opportunities cannot occur.

## Identity, method and reproduction

The original simulator/analyzer source is still ME-005 C
`442da046f8cc9a6b78ce3900ec13badd3fd2b74a`; protocol P is
`96d240ad60bbfb9e3c73c9f0eb21acd665b156ee`. The accepted four result
SHA-256s and execution hashes are in [result.json](result.json). Their exact
files were rehashed and matched before this diagnostic. The *new exploratory
analysis code*, not a simulator source replacement, is commit
`09f6f59dffa754ae28933de9156c2e5d42cd02e4`; its clean Go 1.27.0
`me005stages` binary SHA-256 is
`81be712e391979ade974f569f2cbf0d012d2b8836bee2aa3e875b1129181e8ff`.
The external diagnostic is
`/home/vlad/ExchangeSimulation-me002b-me005-diagnostic-20260926/me005-public-stages.json`,
SHA-256 `1e2392220cd639ca021424ba0ccef79541b5420dcd61d75990040f6319851c09`.
This external path is a local artifact pointer, not a remote-backup claim.

The one bounded batch reused the ON cells' already accepted result-embedded
public replay. For the OFF cells, it scanned the earlier rendered book logs
once in global frame order. It checked each accepted result digest and
raw/rendered/config/source binding, rejected duplicate/out-of-order/malformed
public transitions, and checked the fee-positive stage against the accepted
zero-episode result. It did **not** re-render or independently rehash every
raw binary frame. The original ME-005 analyzer had already verified those
streams; the present diagnostic is a versioned additional calculation, not
a replacement audit. Synthetic tests include nested gross/depth/fee stages,
same-timestamp zero-duration episodes, and malformed sequence/quote rejection.

From a clean checkout of analysis commit `09f6f59`, build the Go binary and
run only against the retained namespace into a *new* output path:

```bash
go build -trimpath -o <new-binary-path> ./cmd/me005stages
<new-binary-path> \
  -root /home/vlad/ExchangeSimulation-me005-evidence-20260924 \
  -contracts research/program/ideas/ME-005 \
  -summary research/program/ideas/ME-005/result.json \
  -out <new-diagnostic-json-path>
```

No old result file is overwritten. ME-005's registered opportunity count,
status, reviewer scope and exhausted run budget remain exactly as before.
