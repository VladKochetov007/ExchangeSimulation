# Intermediate V-020 derivative artifacts

These 21 ae13f9a derivative-analysis outputs were generated after the initial
funding-boundary correction but before the final stable tie ordering in
`c1a8357`. They are retained because equal-timestamp position points had been
collected concurrently and sorted by timestamp alone. The final analyzer
orders ties by timestamp, file, and physical record ordinal, and the live
artifacts were replayed once more under that final rule.

No economic simulator state changed. See V-020 in
`research/validation-audit.md` for the complete provenance.
