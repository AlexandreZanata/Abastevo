# FQ-05C — Verified Rust-to-Go preparation bridge

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING.
Opened: 2026-10-15. Completed: 2026-10-16.
Behavior parent: 5437f762.

Before behavior: B-BR-FQ-02/03/04/05/07/13/16, BUC-FQ-01/02/03.
Consume the existing quality-prepared-v1 file-backed Rust contract through bounded
read-only ports. Require the expected CSV hash/bytes from the trusted preparation
runner; hashes are integrity evidence, not proof that an arbitrary external
producer actually ran Rust. The subsequent composition must execute the bundled
trusted preparer on fetched/pinned bytes before publishing.

Verify strict manifest/schema/count balance, three fixed filenames, all stream
bytes/hashes/line counts, accepted ordering/uniqueness, duplicate twins, quarantine
shape and Rust's exact manifest hash before any Directory resolution. Reuse
100 MiB / 500,000-row / 1 MiB-line ceilings; total prepared output is bounded to
100 MiB, not three independently unbounded buffers. Errors contain closed reasons,
not source records, CNPJs or raw text. No dependency, SQL write or network here.

Derive an opaque public sample reference from source/native sample/full CNPJ/date/
fuel; identical numeric sample IDs at another establishment/date/fuel cannot mix.
Retain native assay/result/unit/evidence verbatim. Fingerprints mirror Rust's
separator hash; reject embedded separator/NUL controls before hashing to avoid
ambiguous identities. Validate DB field bounds and closed fuel/conformity enums.

After full integrity validation, resolve once per full sample through a read-only
port. Exact accepted decisions become active links; unmatched/ambiguous/succession
or wrong-branch decisions remain inactive with explicit reason. Never create a
station, reinterpret operational identifier timestamps as establishment dates,
infer identity by proximity/name, or fabricate historical validity. Production
Directory composition and native archive/worker scheduling remain a separate
bounded checkpoint, with no live publication from this phase.

Immediate acceptance: actual Rust CLI round trip, hash/bytes/count/manifest/schema/
path/encoding/duplicate failures; no resolution on corrupt streams; empty results,
Unicode/comparators, full sample isolation, deterministic replay, failure propagation,
existing raw native September artifact reconciliation and measured preparation
bridge memory/runtime before scale. Public/live history remains unchanged on error.

## Validation and measured limits

Pure RED→GREEN: missing bridge port initially refused compilation. A separate
calendar regression reproduced acceptance of 2026-02-30 in domain ValidDay; it now
uses real Gregorian date validation, matching the native Rust contract (including
leap centuries and year-zero refusal). Existing quality domain/application and
all real-PostGIS/race suites remain green.

- Strict stream/schema/hash/count/ordering checks and 18 corruption cases PASS:
  changed input identity, unknown/duplicate fields, path traversal, budgets,
  logical hash, unbalanced/absent counts, truncated/CRLF output (including
  recomputed artifact metadata), duplicate without twin, accepted duplicate,
  unknown row/fuel, invalid UTF-8/calendar. Errors expose no source content.
- Empty native result/unit, accents, Unicode line separator and comparators
  survive the real Rust CLI. Sample/CNPJ/date/fuel distinctions, deterministic
  replay, resolver failure/cancellation and inactive review candidates PASS.
  Stream integrity is fully checked before any resolver call.
- Actual Rust → Go → PostGIS → public reader integration PASS: synthetic 12-row
  source reconciles to seven accepted; eight simultaneous publishes converge to
  one revision, four matched station determinations remain readable, and no
  station/extra publication is created. No production data enters fixtures.
- Complete quality `-race -tags=integration` current matrix PASS in seven packages.
  Rebuilt the pinned Rust binaries and restarted only the documented local
  disposable PostGIS service after prior /tmp/runtime state expired; tests were
  repeated against that reconstructed environment, not assumed from old logs.
- Affected vet/staticcheck, API/worker/ops builds, gofmt, workflow YAML/build/env
  wiring, git diff --check and staged/new secret/PII surface review PASS. No SQL,
  dependency, API or migration addition. CI cadence/protection unchanged.

Commands (repository root; local disposable PostGIS only):

```sh
cargo +1.99.0 build --locked --manifest-path tools/station-prep/Cargo.toml --bin prepare_registry --bin prepare_quality --target-dir /tmp/fq-native-rust
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/fq-native-rust/debug/prepare_quality GOCACHE=/tmp/fq-completion-go-cache go -C backend test -race -tags=integration ./internal/modules/quality/... -count=1
```

The existing integration workflow now builds/exports prepare_quality alongside
prepare_registry; a missing trusted test binary fails explicitly, never skips.
Logs outside Git: `/tmp/fq05c-quality-current.log`, `/tmp/fq05c-bridge-current.log`.

Native measurement was reconstructed from the [official PMQC source register](https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/pmqc-programa-de-monitoramento-da-qualidade-dos-combustiveis)
on 2026-10-16, verified TLS/no redirects/bounded download. September ZIP remains
838,040 bytes, SHA256 98721074799569affbbcbd99e173f42e26cb1107fceed707af7fe4975e95c6ab;
CSV remains 15,937,906 bytes, SHA256 cecf8ac7d4eadddc65c7238f19c7a63498af3fc0273abbfc9828803a2cf67b7d.
Re-executed Rust preparation reconciles 66,225 input = 34,158 accepted + 32,067
duplicates + 0 quarantined, across 4,164 full sample scopes. Logical prepared hash
remains 4cfde2093b610127112880c79c0dd82e2c79876ea7564e994441035bb82119ba.
No raw business identifiers/coordinates/rows or source files were committed.

Repeatable explicit manual benchmark (not a default CI production-data input):

```sh
GOCACHE=/tmp/fq-completion-go-cache go -C backend test -c -o /tmp/fq05c-prepared.test ./internal/modules/quality/adapters/prepared
ANPFUEL_NATIVE_QUALITY_PREPARED='<private verified prepared directory>' ANPFUEL_NATIVE_QUALITY_CSV='<private verified September CSV>' /usr/bin/time -v /tmp/fq05c-prepared.test -test.run='^$' -test.bench='^BenchmarkNativePreparedBridge$' -test.benchtime=1x -test.count=1
```

Current measurement: Linux/amd64, Intel i7-13620H, ordinary non-race Go test binary;
302.432 ms bridge time, 187,071,976 total allocated bytes/op, 45,800 KiB peak RSS
for the whole measured process, 0.32 s wall time. This is one local verified
monthly artifact; larger editions/worker concurrency/runtime still need their
own resource proof. No production performance or source coverage certification.
Log: `/tmp/fq05c-native-current.log`; source bytes remain only in private /tmp.

Recovery: reject the whole prepared batch and retain the existing publication;
read-only resolution does not create identity. Trusted preparer execution,
archive/fetch limits, Directory chronology composition, native worker scheduling
and actual staging import remain FQ-05W, followed by FQ-12 onward. No deployment,
score, Android acceptance, PR merge or release is claimed. D stays future-only.
