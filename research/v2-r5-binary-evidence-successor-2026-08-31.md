# V2 r5 successor: binary evidence promotion checkpoint

Date: 2026-08-31  
Scientific branch: `autoresearch/ffa-ecology-gen0`  
Code/evidence source revision at checkpoint: `ae739027ee8a6d4882948c415c3b4956ca9b8ef9`  
Pre-correction review target: `779564d7ecd1b00b486c9f18b12c19a16ce496e2`

## Scope

This note records the evidence-representation successor candidate after the
R2 calendar/lifecycle amendment. It does not revise historical JSON run
identities, hashes, verdicts, or retained artifacts. The R2 economic semantics
are unchanged: calendar expiries identify one instrument by underlying, type,
and expiry; listing families are separate policies; overlapping requests are
deduplicated and all family schedules continue advancing.

The binary format is a new representation contract, not an economic tuning
change. No registered development cell or holdout has been run under this
successor yet.

## Selective integration

Only the binary evidence representation, routing, reconstruction, and its
promotion gates were taken into the scientific branch. The independent
performance branch was not merged wholesale. The scientific commits in this
integration are:

- `13e000f` registers `evstream_v3` in the R2 successor configurations.
- `61b7933` versions binary evidence manifests and attestations.
- `4c95c6a` replaces the obsolete JSON capacity floor with a measured binary
  capacity attestation requirement.
- `f885e11` adds binary full/no-log parity controls.
- `6429e99` aligns the archive fixture with the binary route.
- `ba9aa71` adds strict binary-to-JSON reconstruction for analysis.
- `433f36d` proves binary execution streams are log-mode neutral in-process.
- `66f35b5` restores the established public `Logger` `OrderFill` map payload
  while retaining binary coverage through the opaque JSON fallback; it also
  removes a tracked system-temp literal rejected by the repository guard.
- `f170d19` adds the fresh-process binary determinism/neutrality test.

## Evidence contract

`evstream_v3` writes one terminated `events.evs` stream. Frames carry the
simulation timestamp, client identity, venue reference, schema/version, and
the canonical event envelope. Per-venue sequence numbers are shared by
`LogEvent` and `LogEvidenceOnly`; evidence-only JSON sidecars are merged by
that sequence during reconstruction. The stream trailer records frame count
and the SHA-256 digest of canonical uncompressed frames. CRCs, dictionary
references, frame counts, trailer digest, trailing bytes, and termination are
validated fail-closed.

Known typed exchange payloads have deterministic codecs. Unknown event families
remain covered by a canonical opaque-JSON payload inside the same binary
framing, so coverage is uniform and there is no hidden mixed JSON/binary
execution stream. Unencodable payloads are replaced only to preserve sequence
continuity, counted in the attestation, and make production rendering or
promotion invalid.

The renderer requires a binary manifest, a valid terminated stream, valid
route/event references, and a matching binary attestation. It reconstructs the
legacy JSON analysis layout deterministically, merges evidence-only sidecars,
rejects duplicate/missing sequence numbers, and refuses unsafe/nonempty output
destinations. The legacy JSON artifact hash is not emitted for binary runs;
the binary attestation and, for full logging, the separate evidence-only
attestation name their distinct domains and ordering semantics.

## Differential and neutrality evidence

The following passed on the clean tree after the correction at `ae73902`:

- typed evstream round trips, corruption/fail-closed mutations, and the
  unrepresentable client-ID regression;
- exchange and multivenue binary render/reconstruction tests;
- differential rendered payload comparison against the legacy JSON logger for
  opaque, balance, fee, trade, and venue-ledger families;
- production-path binary render of a short simulation;
- byte-identical binary execution streams for full and no-log modes in the
  in-process parity test;
- fresh child-process execution and binary stream identities for
  `GOMAXPROCS=1` and `4`, each in full and no-log modes;
- legacy public logger behavior, including realized perpetual PnL observers,
  after restoring the `OrderFill` map payload.

The clean verification results were:

```text
GOMAXPROCS=4 make test                         PASS
GOMAXPROCS=4 go vet ./...                      PASS
go test -race ./analysis ./cmd/mvanalyze ./cmd/prunegate ./tests  PASS
binary-focused Go tests                         PASS
R2 contract, archive, and config checks         PASS
```

The full test gate took approximately 194 seconds in the multivenue package;
resource monitoring observed no OOM or swap activity. The current host had
approximately 27 GiB available RAM and 33 GiB free disk after the gate.

## Independent review and correction

The first fresh Sol-xhigh review examined exact revision `779564d` and
returned `REJECT`. Its blocking finding was confirmed locally: the registered
launcher still applied the obsolete JSON-derived free-space floor before it
consulted the binary capacity attestation. That contradicted this successor
contract and could reject a binary-authorized launch for a historical storage
reason.

Commit `ae73902` removed that stale launcher floor, made the binary capacity
attestation the sole launch-capacity authority, and added a contract test that
constructs a valid binary attestation at a test path and verifies that it is
accepted while the old JSON-floor symbols are absent from the launcher. The
correction did not change simulation economics, ordering, or evidence format.

The clean full suite was rerun after the correction and passed. A new fresh
independent review of the corrected exact tree remains required; the rejection
of `779564d` is retained and is not silently upgraded.

## Performance feed status

The independent branch `origin/autoresearch/v2-performance-research` was
fetched at the natural checkpoint. The last reviewed revision remains
`c4434ad`; there were no newer commits. Its binary prototype remains a source
of future performance work, not a source-tree merge. No sparse-holder or
preview optimizations were imported with this evidence successor.

## Remaining promotion gates

The candidate is mechanically complete but not scientifically authorized for
execution. The following remain open:

1. obtain one fresh independent Sol-xhigh review of the exact successor tree,
   including R2 calendar semantics, correctness hardening, and binary evidence;
2. build clean provenance-pinned Go 1.27 binaries with `-trimpath` and
   `CGO_ENABLED=0` after review;
3. run a separately identified, non-cell 24-hour binary capacity probe and
   publish its measured attestation only after its measurement contract passes;
4. verify the resulting capacity against current disk/RAM without deleting
   retained historical evidence;
5. only after review acceptance and capacity authorization, run registered
   `dev-607` and extract/review it before `dev-613` or `dev-617`;
6. do not inspect or consume reserved holdouts `619`, `631`, or `641` before
   explicit freeze authorization.

There is currently no binary capacity attestation. Consequently, the
registered R2 runner remains fail-closed and cannot launch `dev-607`.

## Scientific status

This is a successor candidate and promotion checkpoint, not a freeze result.
The R2 population amendment and correctness hardening remain subject to the
fresh independent review. Historical claims remain attached to their original
JSON revisions. Any later semantic rejection or correction must create a new
successor revision and must not repair old trajectories offline.
