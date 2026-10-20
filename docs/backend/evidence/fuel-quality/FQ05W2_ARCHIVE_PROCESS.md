# FQ-05W2 — Bounded official archive and trusted preparer process

Status: LOCAL_DONE
Validation: PASS

Date: 2026-10-16. Maintained dev parent 299a0bac. B-BR-FQ-01/02/03/04/07/12/13/16,
BUC-FQ-01/02/03/04. Prebehavior scope: [FQ-05W](FQ05W_NATIVE_RUNTIME.md).
This checkpoint owns archive/process handling; Directory, fetcher and deployed
worker composition remain separate required FQ-05W obligations.

## Behavior

Exact HTTPS www.gov.br PMQC file URLs only; no credentials, query, fragment,
encoded paths, custom ports, wrong source/year/month or inferred February file.
Verify the regular parent file against planned SHA256/bytes. Inspect the ZIP end
record and ≤64 KiB central directory before archive/zip allocates entries; ≤32
entries, ≤100 MiB compressed, ≤1 GiB declared aggregate, ≤30 MiB per CSV.
ZIP64/multidisk, duplicate/unknown/traversal names, encryption, symlinks and
multiple CSVs for one month fail closed. CSV members are distinct editions;
unused JSON is budget/shape checked, never claimed as imported content.

Extract only the selected source/edition CSV into a mode-700 per-attempt workspace,
mode-600 file. Verify exact selected bytes and CRC plus parent descriptor again.
Run the absolute operator-owned non-writable-by-group/others Rust executable with
an explicit ≤2 minute context, minimal environment, no shell and discarded
stdout/stderr. The path is configuration, never job/source data. Verify the
manifest/three regular prepared streams with the existing bridge before any
resolver. The internal eight-column compatibility format is refused by native
intake. Preserve raw result/unit/conformity and source/member hashes; calendar
period claims derive from the explicitly selected filename. Success, failure and
cancellation clean temporary files; cleanup failure returns a redacted error and
no result. No raw rows, paths or subprocess/Directory error bodies in errors.

## Acceptance

- RED: missing archive/process APIs; GREEN: inventory, monthly identity and closed
  source URL contract. Meaningful negative cases cover entry/central-directory
  allocation budget, selected declared size, encryption, CRC, truncation,
  symlink/duplicate monthly scope, cross-period, changed hash and cancellation.
- Real Rust process succeeds on native-layout synthetic data with unchanged text
  and duplicate reconciliation; unsupported headers/internal contract, missing
  executable, resolver outage, process failure/timeout and cleanup are verified.
- `ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_quality go test -count=1 -race -tags=integration ./internal/modules/quality/adapters/native ./internal/modules/quality/adapters/prepared`: PASS.
- Affected go vet/staticcheck, gofmt, whitespace and changed/untracked secret/PII
  review: PASS. CI builds the pinned preparer and explicitly sets safe executable
  permissions; required binary missing fails rather than skipping integration.
- Existing MIT station-prep crate/Cargo.lock reused; no dependency additions.
  Rust 1.99.0 locked static build with `RUSTFLAGS='-C target-feature=+crt-static'`
  and explicit `--target x86_64-unknown-linux-gnu` succeeds. readelf confirms no
  interpreter/NEEDED shared library. Actual native runner tests pass with that
  release static binary. Worker image packaging is still owed.

## Offline real-source observation, no publication

Opt-in `integration,qualitysource` manual test uses explicitly acquired private
archives; no downloads/database writes/raw identity logs. Both debug and static
release preparer paths PASS. Exact source hashes match prior acquisition:
September parent 98721074799569affbbcbd99e173f42e26cb1107fceed707af7fe4975e95c6ab
(838,040 bytes), selected member
cecf8ac7d4eadddc65c7238f19c7a63498af3fc0273abbfc9828803a2cf67b7d
(15,937,906 bytes): 66,225 input =34,158 accepted +32,067 duplicate +0 quarantine.

Annual 2025 parent c25d8dcf0c93560085572c49327a077202c6ac33d24c87303c4bb34b60d383c2
(24,513,334 bytes), twelve monthly CSV editions; January member
de013f3d7455236ec0c2846426e4beee072d3a0dc7f38c5c3a421da69302cafd
(23,808,189 bytes): 96,239 =42,185 accepted +48,791 duplicate +5,263 quarantine.
Private count-only census explains all 5,263 as ISO date-time values unsupported
by the current date-only parser, not bad chemical results. Complete explicit
source date-time compatibility as FQ-02C before historical activation. No coverage
or score calibration claim from this partial historical preparation.

Next: FQ-02C, then Directory identity fencing, file-backed official fetch/discovery,
serial worker slots and static image packaging in FQ-05W. FQ-12–17 and protected
final delivery remain pending. D receipts/business portal remain NOT_AUTHORIZED.
