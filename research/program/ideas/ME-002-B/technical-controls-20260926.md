# ME-002-B operational gate — finite resources and technical controls

Executed after the [prospective lock](development-lock-20260926.md) and before
any economic cell. This is a reproducibility/resource gate, **not** a third
economic replicate or an estimate of the poll/network effect.

The fresh `systemd-run --user --scope` preflight placed the measured child in
the user scope with `memory.max=4294967296`, `memory.swap.max=0` and
`cpu.max=700000 100000`. The complete, zero-exit measurement is retained at
`/home/vlad/ExchangeSimulation-me002b-development-4040913/preflight/resource.json`
(SHA-256 `90d310e2dbb93bc66b93a298e68fca3453e16bf362683f3d65c21fdc98002aaf`).
It recorded no OOM or swap use. The scope settings were then applied to
the separate control commands, with the first control restricted to 100%
CPU and the second to 700%; both used 4 GiB/zero swap.

Two independent clean-C processes repeated the P1-N1/13001 locked plan:

| Control | Go workers | Measured run | Measured analysis | Outcome |
|---|---:|---|---|---|
| g1 | 1 | `measurements/control-g1.json` | `measurements/control-g1-analysis.json` | complete, zero exit, finite cgroup, no swap/OOM |
| g7 | 7 | `measurements/control-g7.json` | `measurements/control-g7-analysis.json` | complete, zero exit, finite cgroup, no swap/OOM |

Every counterpart file matched byte-for-byte: evidence SHA-256
`8c3a4eb14032f41c8ab6170d0cefa073b8abdace9f341d91910132143943a577`,
actor report `643fc7fd14ebf9e3159c484565546d41ab75a0cad27673222f54cc5f9776c547`,
run manifest `46eca47368f2372893e3430c3aca928fcc5042fa75ae7c55b35d8e80145532cc`
and independently reconstructed result
`83b7973cc05ad06456350ef05d8413955493f68c7565ddc019b3e237fffefa4e`.
The canonical evidence execution hash was
`995fe0cc47c0125b55a5feceff4469c803d444fdcb5d5f8356c578be1d849b86`
with 18,050 frames in each control.

The control run/analysis resource records are external immutable artifacts
under the same root; their SHA-256 values are, respectively, g1
`3e203c6cfb8b6894ebfeb44f3da7067888757ca9106d2c6dcd159691c76c36e5`
for the run and `7801860c2229a3c68efb85341f913a6ffdc8ca626fae39a96d9e951528f6b472`
for analysis, g7 `72bd1bcc57022bfc2d3178e182589cbeb7368462a99fb7853befcf1772162544`
for the run and `33ecd843c5705e5b0008bc3672852d1cbe86ce3c95aa883a885bf0a8cd23fcff`
for analysis. The resource verifier checked sample-trace digests,
child placement, finite limits, process completion and OOM counters.

At the gate, the external tree occupied about 21 MiB, the host had about
29 GiB available RAM and 66 GiB free disk. Thus the protocol's 8-GiB RAM,
25-GiB disk, 20-GiB fresh-evidence and 60-minute ceilings permitted the
registered 12-cell matrix. Each cell still requires its own preflight checks.
