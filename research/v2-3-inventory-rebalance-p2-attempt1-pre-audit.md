# V2-3 P2 attempt 1 — retained pre-audit diagnostic evidence

Status: **NOT SCORED.** These four full-evidence worlds were launched before
the required zero-valued-enum schema audit. They are retained at
`research/artifacts/historical/v2-3-p2-attempt1-pre-zero-enum-audit/`, but the
final P2 A/B screen must be rerun after the audit and rebuild.

The worlds used source revision `96484f80f14cc85024a2ceb0da73e52eb9be2b21`,
`GOMAXPROCS=3`, and multivenue binary SHA-256
`eea53cfada3a23efeb74d23c02009ad444810c7620a05ef873142f6d064f603c`.

| arm | seed | persisted events | evidence multiset digest |
| --- | ---: | ---: | --- |
| A | 101 | 367,465 | `924b744660e8c855c0a98b57880fe4305eb54847dc0e0e917b62a63d6310ebc9` |
| A | 103 | 330,147 | `ff099a21be474f7cb89050413bf164bf2c8150431e74c39cc458e612bd9bd71f` |
| B | 101 | 368,074 | `d82607b243e17016e43d6a2c42724803f0d7b5f5a0646448398812b0e2de96d6` |
| B | 103 | 331,057 | `cd70493bbf001591f00da02af365853c0ecbcbf55d7cc1db4bb3d142e1b80543` |

The source evidence is complete: all cells emitted final `greeks.json` and
`latency.json` and the extraction contract ran. The stored B replay results
show request-field mismatches only because their analyzer predated
`3eed5da`; rerunning that independent replay at `3eed5da` produced valid B
audits with zero decision/outcome/receipt/fill/fee/risk-reduction mismatches.
This establishes a diagnostic about the evidence contract, not a P2 causal
result.

The reason for replacement is procedural rather than economic: the launch
preceded the committed `fb8665d` zero-enum audit required to guard the class of
defect that invalidated attempt 0. Do not pool this attempt with the final
campaign, report its activation counts as a result, or prune its raw logs.
