# V2-8 analyzer optimization: cross-venue target prefilter

Status: **merged; analyzer-only, semantics-preserving optimization** at commit
`c2670d0`.

## Change and contract

`analysis/crossvenue.go` now checks the already decoded event symbol (or the
filename-derived symbol fallback) against the requested cross-venue symbol
before decoding the `BookSnapshot` payload.  The change does not alter event
ordering, accepted records, timestamp handling, stale-quote policy, midpoint
arithmetic, or output serialization.  The derivative and filename symbol
fallback paths remain intact.  Simulator code, runtime evidence encoding,
RNG, scheduling and economic state are untouched.

## Differential validation

The optimization branch was based on `be717d3` and passed `go test ./...`,
`go test -race ./analysis`, and `go vet ./analysis` before merge.  Focused
tests cover:

- physical and derivative books that share the requested symbol;
- an irrelevant malformed snapshot whose payload must never be decoded;
- byte-equivalent report output against the prefilter implementation.

The retained workload artifact is
`research/artifacts/v2-8-analyzer-crossvenue-target-prefilter-20260827.json`.
It records source-run content hash, binary hashes, canonical output hash and
profile hashes.  The candidate output is byte-identical to the baseline:

    eb03100f367ab28669490f01a08cfb7cf8539be1b2fe393798ef33e3fef990e4

The candidate retained the same 1,797 evaluated observations, 5,391 quote
updates, and 0 skipped observations.

## Measured workload result

Five alternating warm-cache runs over a 684,592,532-byte retained evidence
workload reported:

| metric | baseline | candidate | change |
|---|---:|---:|---:|
| wall seconds | 0.86 | 0.61 | −29.07% |
| CPU seconds | 3.90 | 2.91 | −25.38% |
| peak RSS KiB | 66,568 | 61,704 | −7.31% |
| sampled allocation MB | 394.53 | 319.07 | −19.13% |

The gain is from avoiding irrelevant payload unmarshal, not from changing the
scientific evidence domain.  The optimization is restricted to offline
analysis and can be reverted independently if a future parser-contract test
finds a discrepancy.

## Review boundary

Configured Sol-xhigh performance reviewers were unavailable due to the model
usage limit.  I performed a fresh adversarial review before merge; it is not an
independent Sol review.  The review found no semantic change in the target
path and no reason to modify persisted evidence or simulator code.  Broader
JSON decoder replacement remains unadopted pending differential tests on
malformed records, duplicate keys, integer boundaries, `RawMessage`, invalid
UTF-8/escapes, trailing garbage and complete registered analyzer outputs.
