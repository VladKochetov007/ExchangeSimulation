# Go 1.20–1.27 engineering notes for the market simulator

**Research date:** 2026-08-29  
**Working toolchain:** `go1.27.0 linux/amd64`  
**Module directive:** `go 1.25`  
**Scope:** changes relevant to deterministic simulation, evidence-preserving
analysis, concurrency, profiling, testing, and data-intensive execution. This
is a project decision record, not an exhaustive catalog of every package
change.

The version facts were surveyed independently with Luna-medium and checked
against the official release notes. Recommendations below are engineering
judgments for this repository, not claims made by the Go project.

## Release summary

| Version | Relevant changes | Project consequence |
|---|---|---|
| **1.20** | `errors.Join`; `context.WithCancelCause`; `unsafe` slice/string helpers; architecture-feature build tags; `go -C`; program and integration-test coverage; PGO preview; global `math/rand` became randomly seeded. Runtime and GC improvements were also made. | Continue using explicit private seeded RNGs. Existing multi-error handling and architecture-independent builds are compatible with this release. PGO is a separately measured optimization, never part of a scientific semantic change. |
| **1.21** | `min`, `max`, and `clear`; standard `slices`, `maps`, and `cmp` packages; deadline/cancellation-cause helpers and `sync.OnceFunc`/`OnceValue`; PGO became generally available and can devirtualize hot interface calls; more precise initialization and module/toolchain behavior. | Existing use of `slices`, `maps`, `cmp`, and explicit cancellation/error paths is appropriate. Do not rely on package initialization order or global RNG state. |
| **1.22** | Per-iteration `for` variables; integer ranges; `math/rand/v2`; improved GC metadata; enhanced `ServeMux`; workspace vendoring. | The loop change removes a common actor/test capture bug. Do not use top-level `math/rand/v2` calls for golden reproducibility: its stream is intentionally not a compatibility contract. |
| **1.23** | Function iterators in `range`; `iter` and iterator-oriented `slices`/`maps` APIs; `go.mod` `godebug`; `go vet` `stdversion`; improved trace recovery; timer/ticker channels became synchronous and stale `Stop`/`Reset` values are prevented; deeper pprof stacks. | Iterator APIs are suitable for offline analysis only when profiling supports them. Timer semantics must be treated as part of the pinned runtime contract; domain simulation time remains owned by the simulator clock. |
| **1.24** | Generic type aliases; `tool` directives and `go tool` module tools; structured build JSON; VCS build metadata with dirty suffixes; `os.Root`; `testing.B.Loop`; experimental `testing/synctest`; Swiss-table maps and runtime/memory improvements; `runtime.AddCleanup` and `weak`; default linker build IDs. | Use VCS metadata and structured build output in provenance tooling where useful. `testing.B.Loop` is a safe benchmark modernization. `os.Root` is relevant to future untrusted evidence readers. Avoid weak references/finalizers in financial state. |
| **1.25** | No language change; `sync.WaitGroup.Go`; `testing/synctest` became available; trace flight recorder; container-aware and periodically updated default `GOMAXPROCS`; experimental Green Tea GC; DWARF5; experimental `encoding/json/v2` behind `GOEXPERIMENT=jsonv2`; `go version -m -json`; new vet checks. | Explicit `GOMAXPROCS` is mandatory for controlled comparisons. Flight recorder and `synctest` belong in diagnostics/tests, not model semantics. JSON v2 must remain an isolated analyzer experiment. `go version -m -json` can strengthen binary attestation. |
| **1.26** | `new(expr)`; recursive generic constraints; modernized `go fix`; Green Tea GC became default; experimental `simd/archsimd`; experimental `runtime/secret`; `crypto/hpke`; `io.ReadAll` and other library/runtime improvements; experimental goroutine-leak profile. | Runtime improvements are welcome but require a new profile after the freeze candidate is accepted. Experimental SIMD is not suitable for the deterministic core. Keep leak detection as a harness diagnostic, not an evidence field. |
| **1.27** | Generic methods; `encoding/json/v2` and `encoding/json/jsontext` are available; experimental portable `simd` plus revised `simd/archsimd`; cheaper small allocations; generally available `goroutineleak` profile; `uuid`; ML-DSA support; timer `asynctimerchan` compatibility setting removed. `compress/flate` output may differ from Go 1.26. | Build and measure the candidate with pinned Go 1.27. Keep the scalar implementation and standard `encoding/json` API as the reference. Any JSON-v2 or SIMD trial needs differential artifacts and a separate provenance/build identity. Never compare gzip/zip bytes across Go versions without pinning the encoder. |

## Decisions for this repository

### Adopted or already in use

- Use Go 1.27.0 for clean V2 candidate binaries, with `-trimpath`,
  `CGO_ENABLED=0`, explicit `GOMAXPROCS`, and recorded VCS metadata.
- Retain explicit `rand.New(rand.NewSource(seed))` instances. A simulator
  seed is part of the model contract; package-global randomness is not.
- Keep `encoding/json` as the semantic reference for persisted event evidence.
  The current Go 1.27 implementation may have different internal behavior and
  error text from older releases, so exact raw-record hashes are valid only
  within the pinned build contract.
- Continue using `slices`, `maps`, `cmp`, `errors.Join`, and typed atomics where
  they improve clarity without changing ordering, RNG consumption, or domain
  arithmetic. The current source already uses these APIs in the appropriate
  places.
- Treat Go's runtime and compiler gains as execution-environment properties,
  not evidence of economic realism. Re-profile after the signed-price and
  accepted V2 candidate gates, as required by the research objective.

### Conditional experiments, not current production changes

- **PGO:** collect a representative profile only after the accepted candidate
  exists; build a separately named PGO binary and record the profile hash,
  toolchain, CPU target, flags, and binary hash. Compare mechanics, exact
  determinism, evidence digests, and performance before considering it.
- **JSON v2:** test only in offline analysis first. Compare every derived
  artifact and malformed-input decision against the standard API. Do not put
  `GOEXPERIMENT=jsonv2` into a simulation or freeze binary without a
  preregistered compatibility gate.
- **`testing/synctest`:** use for isolated tests of real-time goroutine
  coordination, timers, and cancellation. It must not replace the explicit
  simulated clock or make wall-clock behavior part of an actor's information
  contract.
- **Trace flight recorder and `goroutineleak`:** add optional harness-level
  diagnostics for rare failures or long-running jobs. They must not be enabled
  in a way that changes scheduling, RNG consumption, logging, or evidence.
- **`testing.B.Loop`:** migrate benchmark loops when touching those benchmarks;
  benchmark setup and teardown must remain outside the measured body.
- **SIMD:** consider only behind an external implementation boundary after a
  profile identifies a numerical hot loop. Preserve a scalar implementation,
  compare outputs at the required numeric tolerance, and keep SIMD builds out
  of the canonical cross-platform evidence path until stable APIs exist.

### Explicitly deferred

- No C++ rewrite: the current bottleneck and long-term benefit must first be
  established by a fresh profile.
- No third-party JSON replacement: Sonic/jsoniter or similar libraries may be
  measured in offline analysis, but `encoding/json` remains the reference until
  semantic, malformed-input, ordering, and evidence-equivalence tests pass.
- No use of generic methods, generic aliases, `weak`, `runtime.AddCleanup`, or
  experimental runtime packages merely because they are new. They are not
  needed to express current market mechanics and would increase the semantic
  review surface.

## Reproducibility cautions

1. Pin the Go minor/patch version, OS/architecture, compiler flags,
   `GOEXPERIMENT`, `GODEBUG`, `CGO_ENABLED`, CPU target, and `GOMAXPROCS` for
   every performance or evidence comparison.
2. A newer compiler/runtime can improve speed while changing allocation,
   scheduling, trace shape, debug information, or compressed bytes. None of
   those changes may be interpreted as a model effect.
3. A binary build ID or VCS stamp strengthens provenance but does not by itself
   prove an immutable or cryptographically signed archive. Retain the exact
   binary hash and raw evidence under the existing V2 contract.
4. Go 1.27's `encoding/json` compatibility promise preserves the API's intended
   marshal/unmarshal behavior, but error text and implementation details are
   not a cross-version evidence contract. Keep the version in every run's
   provenance.
5. New runtime facilities are diagnostics unless explicitly declared as model
   semantics. Instrumentation must not consume RNG, alter event ordering, or
   change participant-visible information.

## Official references

- [Go 1.20 release notes](https://go.dev/doc/go1.20)
- [Go 1.21 release notes](https://go.dev/doc/go1.21)
- [Go 1.22 release notes](https://go.dev/doc/go1.22)
- [Go 1.23 release notes](https://go.dev/doc/go1.23)
- [Go 1.24 release notes](https://go.dev/doc/go1.24)
- [Go 1.25 release notes](https://go.dev/doc/go1.25)
- [Go 1.26 release notes](https://go.dev/doc/go1.26)
- [Go 1.27 release notes](https://go.dev/doc/go1.27)
- [PGO user guide](https://go.dev/doc/pgo)
- [Go module reference](https://go.dev/ref/mod)
- [`testing/synctest` package documentation](https://pkg.go.dev/testing/synctest)
- [`runtime/trace` flight recorder documentation](https://pkg.go.dev/runtime/trace#FlightRecorder)
- [`encoding/json/v2` package documentation](https://pkg.go.dev/encoding/json/v2)
- [`encoding/json/jsontext` package documentation](https://pkg.go.dev/encoding/json/jsontext)
- [`simd` package documentation](https://pkg.go.dev/simd)
