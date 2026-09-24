# ME-003 technical-control release — 2026-09-24

This is a pre-development operational release for the *locked* ME-003 matrix,
not an economic result or a confirmation/freeze authorization. The owner
conditionally authorized at most 12 development cells; the reviewed candidate
is `ae0fc333f51b2975827116d2c72db00b4f282ad9`, source tree
`f15f2c750786792b0083fbe0bfe3b5a0cd3ee32d`. It is checked out clean
at `/home/vlad/ExchangeSimulation-me003-candidate-ae0fc33` and built with
Go 1.27.0, `-trimpath -buildvcs=true`. Both binaries report this exact VCS
revision and `vcs.modified=false`:

- simulator `meinstructionpilot` SHA-256
  `b36ba97df08540fc2d65a10e121acff026bb784c9e22d9c07816d787e3682bf0`;
- analyzer `meinstructionanalyze` SHA-256
  `1f24aa42c56eface199a4acd73845d05aa2a9af31f724760d5ecb62d190601a5`.

The generated IOC/0.5 ABC/seed-14001 control plan has raw SHA-256
`1867918f294788da5856c4dda6b045b4531cc11c4a321a302158a63ba50652e0`,
typed-plan SHA-256
`8c4050fb6096bbf028dc7d90b7ef9237f81e335a202b2fe3b87075b0d6e429d3`,
the exact source/binary identity above, instruction `LIMIT/IOC`, and cap
`5_010_000_000` quote units. The new versioned evidence schema is
`instruction-pilot-opaque-v1`.

Two distinct fresh simulator processes, `GOMAXPROCS=1` and `7`, completed
with exit status 0. Each emitted 17,794 canonical frames and execution hash
`a3a12c051fbccf5f2679ded14a7ee12c46f40b6713de3de343b5d89a1fdc9a3c`.
Their evidence files are byte-identical (SHA-256
`bb84efd3043c3f1b402b33bcbbe29096991260cb3250646dcf6a8d8c494ee849`),
as are their actor reports (SHA-256
`154cfca8b45cd964a19e837bd5d2e3e28c3300467f6a3afbf815f3794cec9467`),
manifests (SHA-256
`42a47f561b8ca7c5f8a7fa3c7578b45bf112557b243dc71fe1b627c5ee9f9531`),
and independently reconstructed result files (SHA-256
`8ab8a92a78e81df464666831d46c5ba37fc27e3975c5a6faa0b59d46c1c87a30`).
The control outcome is FULLY_FILLED; it is a technical parity control, not a
registered economic comparison.

Measured `/usr/bin/time -v`: GOMAXPROCS=1 wall 0.40 s, peak RSS 41,344 KiB;
GOMAXPROCS=7 wall 0.33 s, peak RSS 33,024 KiB; each run directory is 4.9 MiB.
Available disk was ~77 GiB and RAM ~30 GiB. Even a conservative 12 × 4.9 MiB
plus these two controls is far below the 1-GiB retained-evidence limit; the
observed peak RSS is far below 4 GiB, and 12 × 0.40 s is far below 15 minutes.
These are screening estimates, not permission to disregard per-cell 30-s,
4-GiB and whole-batch limits; monitor actual usage and stop on breach.

The clean full `make test`, `go vet ./...` and targeted race suite passed on
the executable source at `3012682`; `ae0fc33` added only protocol/review
documentation. The Sol-6 medium prospective rereview in
[the review ledger](../../reviews/me003-prospective-20260924.md) accepted
this exact candidate for technical controls and conditionally for development.
Those conditions are now satisfied. Retained external artifacts are under
`/home/vlad/ExchangeSimulation-me003-development-ae0fc33`; they are not
committed or disposable. The only released next action is the 12 registered
development cells in [protocol order](protocol.md). No ME-003 economic cell
had started when this release was recorded. No holdout or confirmation is
authorized.
