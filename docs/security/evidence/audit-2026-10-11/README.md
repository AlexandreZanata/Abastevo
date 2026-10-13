# Local audit evidence

Pinned source: `954a8d800576fec24acc9edfd3b93aad2ac15450`. See [audit](../../SECURITY_AUDIT_2026-10-11.md) and [machine-readable summary](summary.json).

Only synthetic local data. No production secrets, photos, contributor GPS or VPS operations. The unique local Compose project was `abastevo-security-audit-20261011-edvapj`; its tmpfs PostGIS and network were removed after all eight integration packages passed.

## Reproduction semantics

Files under `probes/backend/` end in `.go.txt` intentionally. They are **audit characterization evidence**, not permanent safety tests or runtime code. A PASS means the named behavior was observed on the audited source. The remediation agent must turn them into negative safety regressions. They reuse existing test helpers, so use the pinned source or reconcile helpers explicitly.

Create a private temporary snapshot with `git archive` of the pinned commit. Copy each preserved `.go.txt` into the matching path without `.txt`. Do not overwrite source/test files in an occupied checkout. All providers, email addresses, tokens, keys and IDs in probes are synthetic; no network target is an actual service.

Run from that snapshot's backend with a writable temporary Go cache:

```sh
GOCACHE=/tmp/abastevo-audit-go-cache GOMAXPROCS=2 go test -race -p 2 -count=1 -v -run '^TestAudit' \
  ./internal/modules/account/application ./internal/modules/account/adapters/oidc \
  ./internal/modules/evidence/application ./internal/modules/evidence/adapters/storage \
  ./internal/modules/identity/adapters/http ./internal/modules/community/application
```

The broad prefix also runs the existing `TestAuditOverdueSignalsFailingPolicy`; this was not newly written by the audit. The eight custom unit cases are listed in the preserved probe files. `audit-probes.txt` records the first six; `audit-extra-probes.txt` records the final corrected retention case and synthetic-location case. The earlier hypothesis that a live job alone prevents original deletion was discarded after tracing `QuarantineCandidates` SQL and is not retained as a finding.

The two real-SQL probes require a **new disposable local** PostGIS project. Never inherit a production/development DSN. Use the repository's `infra/compose.validation.yml` with a newly unique `ANPFUEL_VALIDATION_PROJECT`, inspect its local Docker endpoint, select only `db`, obtain its newly published loopback port, and explicitly set `ANPFUEL_TEST_DATABASE_URL` to that synthetic database. The tests create/drop their own uniquely named databases in that isolated server. Then:

```sh
GOCACHE=/tmp/abastevo-audit-go-cache GOMAXPROCS=2 go test -race -p 2 -count=1 -tags=integration -v \
  -run '^TestAuditRealSQL' ./internal/modules/evidence/adapters ./internal/modules/feedback/adapters
```

Remove only that unique project after the tests. Reproduction results are [reservation concurrency](audit-real-sql.txt) and [comment growth/deletion](audit-feedback-real-sql.txt). The broader eight-package run is recorded in [integration results](security-postgis-integration.txt). Its exact package selection is in the audit and remediation plan; it was not a full production acceptance matrix.

## Kotlin probes

`PhotoAudit.kt.txt` and `ParserAudit.kt.txt` are small standalone JVM entry points. Compile unchanged snapshot sources with a local Kotlin compiler, stdlib and the project's existing JSON/kxml test jars; no new project dependency was installed. Photo compilation used PortablePhoto, PortablePhotoPorts, PortablePhotoFlow, PhotoBlobSeal and AndroidPhotoCache, plus `PhotoKeyProviderAudit.kt.txt` (the same interface declaration, substituting an in-memory AES key for Android Keystore). Parser compilation used SheetRow, XlsxCellReference and StreamingXlsxParser.

The audit used Kotlin compiler/stdlib 2.2.21 from an existing local Gradle installation, org.json 20240303 and kxml2 2.3.0. These harness versions do not certify the app's Gradle build or Android runtime. Execute the parser child JVM with **`-Xmx32m`**; its expected caught OOM is deliberate and must not be run with an unlimited/default large heap. It writes/removes a 748-byte synthetic ZIP in a temporary directory. No XLSX bomb or photo binary is committed.

[Photo outcomes](photo-audit.txt), [parser outcome](parser-audit.txt). Missing/expired/corrupt-envelope behavior and real Keystore/device behavior remain distinct verification scopes.

## Scanner and evidence interpretation

[summary.json](summary.json) retains Go scanner version/database freshness, zero finding events, RustSec metadata and SHA-256 of each Go probe. A large govulncheck stream may include unrelated/fixed advisory metadata; never count its `osv` objects as reachable vulnerabilities. Both advisory scans exited zero. Existing [static gate](security-static.txt) and [tracked secret scan](secret-scan.txt) passed, with their limited scope preserved in the report.

No stress/OOM test targeted the backend/VPS or phone. No real R2/S3 upload, full API HTTP matrix, Android dependency scan, device restore/Fake GPS or 24h physical-copy observation was performed. Reproductions with injected collaborators state that limitation rather than claim an end-to-end exploit.
