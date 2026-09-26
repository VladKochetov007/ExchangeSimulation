# ME-002-B development execution lock — 2026-09-26

Status: **LOCKED PROSPECTIVELY; NO ECONOMIC RESULT IN THIS DOCUMENT**.
The governing [protocol](protocol.md) is P2 commit
`18dcb77147fa15b5c8880456aaa1dac8906de3cb`, file blob
`96884a3be38d0e82c2d0f828143604bf462edabb`. The two bounded
independent pre-run acceptances and their limits are in the
[review record](../../reviews/me002b-prerun-20260926.md).

The executable source is C `404091350440daf8ee0d1299730a932b1bc105af`
(tree `0e947767e723f48c872b1945136fa9ccad57f61e`) in a clean detached
checkout. Both simulator and analyzer use the clean Go 1.27.0 `mecadence`
binary with SHA-256
`09eb76c255f633164e033fa7b777ef4ac11abdcedd6252c11814f9366e221421`.
The separately built resource-measurement adapter has SHA-256
`6df920137e7c16211e7ee03f0691889d879785f8ecc816ee2f39505b5f5c3267`.
The binary's embedded VCS revision is C and `vcs.modified=false`.

The following typed plans were created from C before technical controls or
economic worlds. Paths are relative to the external, retained root
`/home/vlad/ExchangeSimulation-me002b-development-4040913/plans/`. Each plan
binds C, source tree, binary/toolchain/evidence schema, effective world,
typed-plan digest and raw-plan digest. Its SHA-256 is a byte-identity check;
it is not an economic outcome.

| Execution arm | Seed | Plan file | SHA-256 |
|---|---:|---|---|
| P1-N1 | 13001 | `P1-N1-s13001.json` | `865e0263da301caa3d8c94b36c7bfac4016727c5b7f7b082c38508803ae792b5` |
| P1-N1 | 13011 | `P1-N1-s13011.json` | `09691bf2577a7f2a8c6bc96577819db5b601f9973ad8d52e9ab72901c27386f4` |
| P1-N1 | 13017 | `P1-N1-s13017.json` | `c4f2a76edd617ce9186000ec2f3209be6f898b2c17574a2f8f3dcb9e30738013` |
| P80-N1 | 13001 | `P80-N1-s13001.json` | `8f6a1968c803b8ca6e984b59ec9b0bc76d98c6a378cc30aac6b37ddd06e06ce9` |
| P80-N1 | 13011 | `P80-N1-s13011.json` | `f5ecdc1d6c07a0281f731f3869b8e6cac3789919a5cb55008ad9c10411c17b4c` |
| P80-N1 | 13017 | `P80-N1-s13017.json` | `69aa33c730eb41786d7606f3ce4f3a2af7cc7232f95d29cf4d1f8fc24f2f052d` |
| P1-N90 | 13001 | `P1-N90-s13001.json` | `990cfa352170a3b20c9975fd4793e48bad24e1368e0948a1bee89c25ab3a7d81` |
| P1-N90 | 13011 | `P1-N90-s13011.json` | `9313496dad2716695eb01fb2939fbc5ed850ecb542a4ea014a027874529cd226` |
| P1-N90 | 13017 | `P1-N90-s13017.json` | `61e8e5c4efdfbc66b101e03d16ca20024e5b913bca49965525af50b6e94f32a4` |
| P80-N90 | 13001 | `P80-N90-s13001.json` | `60c2980dca7dae6b4f671b428089272c47944b333797fd6447db2ed8dd395d3d` |
| P80-N90 | 13011 | `P80-N90-s13011.json` | `cafe31c9611374a2a8df60c72199713fff9e18b6760355f306e40a4e9c44260b` |
| P80-N90 | 13017 | `P80-N90-s13017.json` | `0b047c1bdf7dab182ea4d1dfb73ff085d990b78a7a5d35332a538bd29c3dc2d3` |

Run **two technical controls first**, independently repeating P1-N1/13001
at `GOMAXPROCS=1` and `7` in fresh output directories. They test fresh-process
determinism/evidence neutrality, not economic replication. Only if their
canonical evidence and reconstructed results agree, run the twelve economic
cells in table order. Every valid assigned cell remains in the sample,
including zero fill, partial fill and loss. No additional arm or seed is
authorized by unused budget.

Use one child process at a time in a freshly verified finite cgroup with
`MemoryMax=4G`, zero swap and `CPUQuota=700%` or tighter. Verify child placement,
limits, exit and complete output before interpreting a result. Require at
least 8 GiB host-available RAM and 25 GiB free disk before every cell; cap
fresh logs/staging at 20 GiB, whole batch at 60 minutes and each world at
two minutes. Preserve all attempts and stop on an infrastructure/evidence
defect; a valid unfavorable economic result is not a stop condition.

Later report or diagnostic commits do not move C or P2. A source, effective
configuration or governing-protocol change requires a named successor and
appropriate renewed review. No holdout, ME-002/003/005 rerun, ME-006/007 or
confirmation condition is part of this lock.
