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
