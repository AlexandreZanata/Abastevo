# FQ-02C — Preserve historical native source collection clocks

Status: LOCAL_DONE
Validation: PASS

Completed 2026-10-16 on maintained dev, parent ecf02f73.

Before behavior: B-BR-FQ-02/03/04/12, BUC-FQ-01/02/03. Actual January 2025
private count-only source census: 5,263 of 96,239 rows use a 19-character
`YYYY-MM-DD HH:MM:SS` source collection clock; the other rows use dates.
The source contract keeps the original clock verbatim and derives its calendar
day for dated Directory resolution/querying, without timezone conversion.
Only real Gregorian day and closed 00–23:00–59:00–59 fields are accepted;
unverified formats/offsets/fractions/invalid calendars remain quarantined.

Version the extended native input contract while keeping assay JSON native
`observed_at` verbatim. The logical fingerprint and opaque sample scope include
the full source clock; differing source times never silently collapse. Store the
optional original collection clock alongside SQL observed_on, expose it
additively in source assay details, and preserve NULL for old date-only rows.
No legacy clock backfill or applied-migration edits; append schema101 with
upgrade/replay/rollback tests. Date-only artifacts/publication identity remain
unchanged. No score/chemical-result reinterpretation, worker activation, D,
contributor GPS, iOS or unrelated infrastructure.

Acceptance: native Rust RED→GREEN for valid/invalid clocks and distinct time
identity; Go calendar/clock parity and lossless bridge; actual archive counts and
zero date-format quarantine for the verified January member; real PostGIS
100→101 upgrade, idempotent date-only replay, frozen clock read and failed
correction rollback; actual HTTP/OpenAPI optional field; affected static checks.


## Result and validation

The CLI emits additive `pmqc-native-19-v2026-10b`; Go accepts this and the prior
native date-only contract. Native JSON observed_at remains verbatim, Rust/Go
fingerprinting and sample scope use the complete clock, and Directory receives
its validated calendar day. No guessed timezone, fractional-second/offset/T-form
support, reinterpretation of results or date-only identity drift.

Migration101 adds optional source_observed_at with exact 19-character/time/day
CHECK. Historical/date-only rows remain NULL. Immutable revision facts retain the
clock; source assay page and sample detail expose the original string only when
present. Actual sample response was also brought into its existing required
sample_ref schema while testing the newly extended wire contract.

RED: Rust accepted 0 of two valid native clocks; Go lacked SourceCollectionDay.
GREEN: native clock identity/replay and calendar/time/version negatives pass.
Real PostGIS100→101 retains date-only revision replay, refuses mismatched day/time,
freezes the original clock and preserves published facts after SQL-failed
correction; migration replay is empty. No applied migration was edited.

Commands/results (private temporary logs outside Git):

- `cargo +1.99.0 test --locked --manifest-path tools/station-prep/Cargo.toml --target-dir /tmp/fq-native-rust`: complete crate113 tests PASS, zero failures.
- Pinned cargo fmt/clippy with warnings denied: PASS.
- `ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_quality go test -count=1 -race -tags=integration ./internal/modules/quality/...`: eight packages PASS against disposable local PostGIS. `/tmp/fq02c-quality-tests.log`.
- Final upgrade101 test PASS; full affected HTTP package rerun after correcting a test's schema name: PASS, including actual source-clock page and sample OpenAPI responses. `/tmp/fq02c-http-final.log`.
- Prepared native-version guard, source clock/calendar/domain/application race tests: PASS. Affected vet/staticcheck, API/worker/migrate/ops build, apicontract, sqlc vet/regeneration stability, Go/Rust format, whitespace and secret/PII surface checks: PASS.
- Rebuilt locked static Rust release; native clock and actual official offline source tests PASS with this binary. No network/database writes in source tests.

Actual official January2025 selected member (same hash/23,808,189 bytes as W2):
96,239 =44,544 accepted +51,695 duplicate +0 quarantine, replacing W2's explicitly
recorded unsupported-clock quarantine. All 5,263 affected input rows reconcile;
no silent truncation/merge. September2026 remains66,225 =34,158/32,067/0 and keeps
logical manifest hash4cfde2093b610127112880c79c0dd82e2c79876ea7564e994441035bb82119ba.
The opt-in manual test now pins these exact counts and unchanged date-only hash.

Only the annual inventory and January/September members were prepared; this does
not certify all months/years, linkage, scores, national coverage or deployment.
Next: remaining FQ-05W Directory identity fencing and file-backed native worker
composition/image, then FQ-12–17 and protected final delivery. D remains held.
