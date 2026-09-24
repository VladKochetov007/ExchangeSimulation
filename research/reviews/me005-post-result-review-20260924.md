# ME-005 bounded post-result reviews — development boundary

Review target: draft report/result/navigation commit `471f35b5037992fe69d419b81582a5e44d4e18e3`.
Executable/analyzer source C: `442da046f8cc9a6b78ce3900ec13badd3fd2b74a`.
Governing protocol P: `96d240ad60bbfb9e3c73c9f0eb21acd665b156ee`.
Raw development evidence: `/home/vlad/ExchangeSimulation-me005-evidence-20260924`.
All reviews were independent, read-only Sol-6 medium contexts. They reviewed
the bounded ME-005 result, not the entire market simulator or any holdout.

| Reviewer | Execution | Substantive verdict | Scope |
|---|---|---|---|
| A1 `01a0d428-5ed5-77d2-9cea-74d843d192ae` | COMPLETED | ACCEPT_WITH_LIMITATIONS | Mechanics, accounting, provenance and opportunity denominator |
| A2 `01a0d428-b293-7030-a512-62c38d9d6208` | COMPLETED | ACCEPT_WITH_LIMITATIONS | Independent second mechanics/evidence check |
| B `01a0d428-cef9-75b0-be0a-40582f555f03` | COMPLETED | ACCEPT_WITH_LIMITATIONS | Causal, statistical and claim scope |

A1 checked that the four retained config/manifests bind to C/P, that the Go
analyzer binds complete render/report/source/config/stream and conservation,
and that the event-ordered public detector applies the one-lot, two-sided,
depth/fee policy. All four retained results show zero policy and broader
leg-side episodes, with zero accounting residuals; ON evaluations are 783 and
790 with no submitted group. A1 required a clearer distinction between
observed shell exit/resource settings and what the retained result files
independently attest. That wording is corrected in the final report/result.

A2 independently checked protocol config hashes, manifests, result hashes,
the zero public-edge counts, ON evaluation reasons and zero balance delta.
It found no numerical/accounting correction needed. It confirmed that a zero
terminal portfolio value with no route is not a zero-return trade. A2 reported
the duplicate raw directories byte-identical; the final claim deliberately
uses only the narrower independently recorded identical canonical execution
hash and byte-identical reconstructed result.

B checked the r1c `NoOpportunity` denominator, four result-file hashes,
OFF/ON descriptive zero differences, unproven random-stream coupling, absent
route response/return/convergence, two-seed uncertainty, delivery-unknown
rule and future-study authorization boundary. B required replacing the broad
phrase “bit-identical across one and seven Go workers” with the exact
hash/result identity. The final result uses that precise statement.

These are substantive acceptances **with limitations and two required wording
corrections**, not an unconditional simulator certification. Neither A nor B
reran the analyzer or simulator, independently decoded all raw frames,
verified per-process cgroup settings, or inspected holdouts. B did not audit
unrecorded attempts. The accepted claim is limited to valid, observed
no-opportunity in four five-minute development cells. Profitability,
general opportunity frequency, route behavior when an edge exists, and
trade-attributed convergence remain unidentified.

The correction commit changes only report/result prose and review status;
it does not change C, P, raw evidence, the analyzer outputs or the economic
verdict. Any material new source/protocol/estimator change would require a
successor validation, not an edit to this review record.
