# Independent SV1 renderer publication review

Date: 2026-09-02  
Exact reviewed HEAD: `362ebe4d75b1fe4638e79e48b3a074bdf6dbf741`  
Branch: `feature/r2-cdf-survival-successor`  
Reviewer: independent Sol-xhigh review  
Verdict: **ACCEPT WITH NARROWER CLAIM**

## Review scope

The review covered the c80 streaming binary evidence renderer and the
filesystem hardening in `362ebe4`. It did not treat the result as a market
cell, survival result, freeze authorization, or holdout authorization.

The reviewer found no blocker for the registered Linux/amd64 controlled
workflow. The prior conditional findings are resolved:

- input and output paths are canonicalized and existing path components are
  inspected with `Lstat`; symlinked output aliases and parents are rejected;
- manifests, events, attestations, and sidecars must be regular non-symlink
  files;
- the venue tree and nested symlinks are rejected during traversal;
- the output directory is revalidated immediately before publication;
- an existing rendered destination is refused; and
- Linux/amd64 publication uses atomic `renameat2(RENAME_NOREPLACE)`.

The exact sequence, CRC/trailer, frame-count, projected execution hash, raw
canonical hash, sidecar byte preservation, route validation, and staged
publication checks remain intact. The reviewer found no change to simulator,
actor, exchange, CDF supplier, RNG, scheduler, configuration, evstream writer,
extractor, verifier, scorer, or parity behavior.

## Evidence and limitations

The memory claim is accepted only for a fixed registered route universe:
memory is independent of event count and scales with one current sidecar
record/scanner and one output buffer per route, plus the stream reader state.
There is no general untrusted route-count/file-descriptor cap.

Publication is intentionally supported only on Linux/amd64; other platforms
fail closed. The controlled scientific workflow assumes the retained evidence
and private temporary namespace are quiescent while rendering, because an
`Lstat` followed by `open` is not a defense against a hostile same-UID race.
The reviewer did not run the complete repository suite and did not claim
24-hour market survival, scientific validity, or capacity sufficiency.

## Independent checks

The reviewer reported passing focused renderer, binary evidence, evstream,
exchange, race, and vet checks. The local clean full `GOMAXPROCS=4 make test`
at the same exact code tree also passed before this review record was appended.

The review therefore clears only the renderer/mechanical promotion boundary.
A subsequent documentation commit changes the exact provenance for the
current capacity measurement and must be included in the pinned rebuild.

