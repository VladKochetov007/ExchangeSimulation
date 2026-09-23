# ME-002 prospective review and repair ledger

This is a development-readiness record, not a result. No registered ME-002
economic world had run when it was written. The design remains the 24-cell
factorial in [the draft protocol](protocol.md).

An independent read-only Sol-6 medium preliminary code review of the diff from
`da06b6d` found four gaps: no pinned ME-002 run path; zero-processing arms had
no completion event; replay could accept a completion after its same-time
decision tick if the displayed quote was unchanged; and the opportunity funnel
was not reconstructed. The exact `fcb7ee8` tree then received an independent
read-only Sol-6 medium prospective review. It found the 2×2 design and
retained-timescale rationale defensible, but issued **NOT ACCEPTED FOR
EXECUTION** because action-latency/opportunity-duration outputs, several
discriminating fixtures, and v4 corruption demonstrations were incomplete.
Neither review ran a world or certified realism.

Subsequent candidate repairs are prospective and require a new substantive
review before any assigned cell:

- The v4 replay now exposes a joined publication→receipt→processing→decision→
  arrival decomposition, plus separately validated admission-response delay.
  Processing-to-decision includes the fixed decision gate, not only a poll.
- Only snapshot-observed qualifying depth episodes with both observed
  boundaries receive a duration and ratio; left/right censoring is explicit.
- Fixtures cover an exchange ask removed before a 90 ms order arrives, exact
  neighboring processing ticks, identity independence for otherwise identical
  focal actors, four arm reconstruction, ME-001 C0 seed 1009 economic parity,
  and evidence-on/off neutrality.
- Rehashed v4 mutations now omit, duplicate, misorder and alter processing
  events, and corrupt request/response, fill timing, fees, ledger and terminal
  evidence. The accepted ME-001 v3 schema remains separate.

These are test/readiness claims only. A clean exact candidate, full test/race
and static checks, prospective review, pinned binaries and finite resource
preflight remain ahead of ME-002 development execution. A rejection is not
converted to acceptance by this ledger.

## Technical-control failure after first acceptance

A new Sol-6 medium reviewer substantively **ACCEPTED `7c7b7e6` for prospective
ME-002 development only**, subject to the remaining gates. Clean full tests,
`go vet ./...` and targeted race checks passed. Pinned Go 1.27.0 binaries from
clean `7c7b7e6` had hashes `ffbe8f3d…e9a1` (simulator) and `2c45f5eb…eca58`
(analyzer). The two authorized F/F quantity-0.5, seed-12001 technical controls
at `GOMAXPROCS=1/7` produced byte-identical 4.9 MB evidence files (SHA-256
`6493d699…2c5e1`, canonical hash `3361f126…ddb40c`, 17,858 frames). Their
observed wall times were 0.40/0.32 seconds, peak RSS 40,832/32,128 KiB, and
both simulator processes exited zero. Exact artifacts remain under
`/home/vlad/ExchangeSimulation-me002-development-7c7b7e6/`.

The analyzer then exited nonzero before creating either result: `json: cannot
unmarshal string into Go struct field ExecutionReport.Side of type types.Side`.
This is an **analyzer-wire defect**, not an economic result or a failed
determinism comparison. The 7c7 controls are retained as an incomplete
analysis attempt; they are not silently promoted. A successor correction
decodes a strict projection of the actor-report wire format, cross-checks the
independent replay, and has a fixture using the actual JSON serialization.
Because executable analyzer code changed, a new pinned candidate/review and
fresh controls are required before the 24 economic worlds. The existing 7c7
acceptance does not authorize its successor.
