# V2-R2-SV1D activation-gate triage

Date: 2026-09-10
Candidate: `V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY`
Predecessor: `V2-R2-SV1C-STRICT-RISK-CDF-LIQUIDITY`
Status: **INVALID ARM EVIDENCE / NON-ADVANCING GATE**

This record is append-only scientific state. It does not rewrite the closed R2
negative control, the closed SV1C negative activation, or any retained raw
evidence. It does not authorize a development cell, freeze, or holdout.

## Exact candidate and authorization boundary

The activation was attempted from:

- branch: `feature/r2-cdf-survival-successor-sv1d`;
- source revision: `0e024771863440202df5cc36d4276bfa35f4c109`;
- candidate Git tree SHA-256: `7de1eefb3b991f926d84266431afb4b5375388ccde6556226788856c60820e94`;
- worktree: clean and pushed before execution;
- registered cell: seed `659`, five simulated minutes, `evstream_v3`, full log mode;
- arms: `treatment`, same-roster `mode-off`, and `no-roster`;
- registered holdouts: `619`, `631`, and `641`, none consumed.

The accepted exact-tree review and capacity attestation authorized only clean
pinned binaries, a registered outcome-neutral capacity measurement, and this
small activation probe. They did not authorize `dev-607`, `dev-613`, `dev-617`,
freeze, or holdout execution.

The immutable activation root is:

`/home/vlad/external-scratch/v2-r2-sv1d-activation-659-0e024771863440202df5cc36d4276bfa35f4c109`

Its producer wrote `activation-provenance.json` with SHA-256
`bd5180e869642a4e7d1b673f99dd11be25400cac366954401ea32323833931ad` and
status `INVALID_ARM_EVIDENCE`. The arm exit statuses were `1` for all three
arms; all three arm validity flags were false; all three arm outcomes were
`malformed_terminal_outcome`.

## Observed failure

Each arm emitted the same structured terminal diagnostic:

```text
status: terminal_failure
code: SIMULATION_FAILURE
phase: terminal_post_mark
failure: scheduled_risk_capture at 1735689661000000000 for venue south
symbol: option ABC-OPT-U4142432f555344-1735696800-K4900000000-C
reason: option risk mark: no usable price
terminal_risk_captured: false
terminal_population_captured: false
```

The shared error occurred at simulated 00:01:01, not at the registered
five-minute endpoint. The terminal contract intentionally does not promote a
scheduled/nonterminal failure to a valid endpoint; only a completed endpoint or
a typed price failure exactly at the terminal boundary is eligible. The
producer therefore correctly refused to publish a promotable arm.

The retained rendered evidence supplies the local cause:

- south `ABC/USD` public snapshot, event sequence `32780`, at
  `1735689661000000000`, had both `asks` and `bids` empty;
- the normal cancel/requote cycle made the empty interval transient; the next
  public snapshot at `1735689662000000000` was two-sided again;
- the scheduled strict-risk mark consequently failed for the option whose
  underlying is `ABC/USD`;
- derivative evidence recorded unavailable marks for the futures and option
  chain and cross-margin liquidation records citing unavailable marks.

This is not evidence that the strict valuation rule is wrong. It is evidence
that the current registered seed/configuration reaches an unpriceable scheduled
risk state before the activation endpoint.

## CDF activation result

The treatment did not exercise the registered mechanism. In the retained
treatment rendering:

- no decision had `local_book_mode == "one_sided"`;
- the observed CDF supplier decisions remained two-sided decisions;
- no treatment-vs-mode-off causal activation comparison was eligible;
- treatment and mode-off binary evidence had identical execution-stream hash
  `265eecd061ba2c85420037e41852b90af7d3b1fce50775f644e87dfd786b9cb4` and
  canonical stream hash
  `c5cca9974d48285a620c5ff6787ca275bda88fd08071c3d301ea8e188656c3a3`;
- the no-roster arm differed as expected for topology, but failed at the same
  shared strict-risk boundary.

The CDF hypothesis is therefore **untested**, not supported and not falsified.
The all-twelve-supplier activation predicate cannot be evaluated from these
arms.

## Independent review

Lagrange (`gpt-5.6-sol`, xhigh; agent
`01a08be2-5f68-7b62-b3b4-59033bd93b7c`) independently reviewed the exact
activation root and returned:

> repair-required producer/activation-fixture defect, yielding invalid,
> non-advancing evidence—not a valid activation or valid negative activation.

The reviewer confirmed that the terminal contract rejects the scheduled
failure, that the raw treatment evidence records the empty/invalid underlying
book, and that treatment and mode-off did not exercise one-sided liquidity.
The reviewer explicitly rejected weakening strict valuation, fabricating a
fallback price, forcing one-sided activation, or altering SV1D economics. No
separate report file was materialized because the bounded review was finalized
in the agent response; the response is retained here as the independent review
record.

## Adjudication and classification

| Item | Classification | Reason |
| --- | --- | --- |
| Strict mark failure on an empty underlying book | Not a bug on this evidence | Strict risk is required to fail closed when a live option cannot be marked. |
| Scheduled failure emitted as generic `SIMULATION_FAILURE` | Contract-correct | A nonterminal failure is not a valid terminal endpoint. |
| Missing complete arm artifacts after the early failure | Repair-required activation producer/fixture path | The gate cannot produce a valid registered endpoint or complete arm status for rerun; this must not be papered over by scoring partial evidence. |
| SV1D one-sided supplier economics | Untested | No one-sided decision, accepted restoration, fill/PnL transition, or withdrawal was observed before failure. |
| R2/SV1C historical claims | Unaffected | No historical trajectory was modified and this successor produced no valid scientific endpoint. |

The next work is a source-level diagnosis of the activation producer/fixture
path and the shared empty-book reachability. Any correction must preserve the
registered seed, config, strict risk semantics, and SV1D economic contract. A
change to risk ordering, fallback valuation, warm-up, roster, or seed would be
a new scientific amendment rather than a repair of this gate.

## Decision and next gate

The candidate is stopped at the activation boundary. Do not run the 24-hour
campaign or any registered development cell from this evidence. Do not rescore
the partial trajectory as activation-negative, and do not rerun old R2/SV1C
experiments.

The only permissible continuation is:

1. diagnose and minimally repair the producer/activation fixture if a
   non-economic defect is demonstrated;
2. add a focused regression and rerun the full mechanical/evidence contract;
3. obtain a fresh exact-tree independent review;
4. rerun seed `659` unchanged only if the reviewed repair makes a complete
   activation endpoint possible.

The asynchronous performance feed was last reviewed through
`b1847ac40e8b7483e6e8a3f94b3705b4058884b`; no new performance implementation
was imported. Disk and memory guards passed, and retained activation evidence
remains preserved outside the repository.
