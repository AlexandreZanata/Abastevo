# MC-A2 official registry harvest and normalization

Status: LOCAL_DONE. Date: 2026-10-27. Base: `6ade4a00` plus this task.
Time: ~35 minutes (parser, tests, release build, VPS run, rerun).

## Input and politeness

The 2026-10-26 coordinate snapshot already on the VPS was reused:
`/opt/abastevo-temp/map-national-20261026/inputs/api-uf/` (31 JSON pages,
27 UFs including SP p1/p2). No new ANP request was made, so D13's 20-second
rule and the stop-at-first-challenge rule were not exercised or bypassed.
The CSVs from MC-A1 remain the denominator; GLP has no coordinate fields, so
the API is not a coordinate source for that kind.

## Parser

New `tools/station-prep/src/anp_registry.rs` + `prepare_anp_registry` binary:

- parses the wrapper `data` array, both string and array `produtos` encodings;
- normalizes CNPJ (numeric/alphanumeric, 14 chars) and UF; keeps authorization,
  dates, legal name, address, municipality, distributor, class, products,
  latitude/longitude, ANP4C text, validation, accuracy, SRID/datum, obtained
  date, origin, observed situation and SIGAF status;
- writes sorted `anp-registry.ndjson`, a quarantine NDJSON with reasons and a
  manifest with input bytes/SHA-256/counts and output hashes;
- requires an explicit `--fetched-at`, so reruns are byte-identical.

Tests: 110 library tests pass (2 new parser tests: reconcile counts and
byte-identical rerun). `scripts/check-rust-fast.sh` -> `check-rust-fast: ok`
after `cargo fmt`.

## VPS run

Command (binary built locally, sent to the private VPS root; raw inputs stay
there):

```sh
prepare_anp_registry --input .../inputs/api-uf \
  --output .../work/anp-registry --fetched-at 2026-10-27T00:00:00Z \
  --kind automotive-fuel --source anp-revendedores-api
```

- 45,807 input, 45,807 accepted, 0 quarantined, 27 UFs.
- 39,017 rows carry a latitude/longitude; 6,790 have no point and fall to the
  address tiers/queue in A3/A6.
- First run 11 s, rerun 5 s; output SHA-256
  `b1a27a65ff21aec941d40a51fff0743c31720deb9134722ea6c49201d0aec247`
  identical on both runs; quarantine SHA-256 is the empty-sha
  `e3b0c442...b855`.
- Per UF: AC 193, AL 568, AM 858, AP 145, BA 3,722, CE 2,027, DF 390,
  ES 799, GO 2,071, MA 1,795, MG 5,042, MS 632, MT 1,177, PA 1,756, PB 885,
  PE 1,634, PI 1,240, PR 2,957, RJ 1,987, RN 756, RO 542, RR 151, RS 3,182,
  SC 2,097, SE 339, SP 8,384, TO 478.

No raw file entered Git; only aggregates and hashes are recorded here.
