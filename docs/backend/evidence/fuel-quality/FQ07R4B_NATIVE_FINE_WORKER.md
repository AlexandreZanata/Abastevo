# FQ-07R4B — Native fine worker, bounded discovery and image

Status: LOCAL_DONE
Validation: PASS

Parenta1cce81e, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. One bounded runtime-composition prerequisite.

Before behavior: reserve the native fine import/discovery job kinds even when
disabled, with no claim/attempt charge by the general dispatcher. Install native
handlers only under the existing explicit import switch; an independent default-off
fine discovery schedule cannot activate PMQC, registry or source imports. Ship the
locked static fine preparer at an immutable worker-image path, nonroot/mode755.

Parse actual annual CSV hrefs only from the exact verified official listing page.
Never manufacture missing years or pretend grouped2016–2019/2021 XLSX are supported
annual CSVs. These coverage gaps remain owed. Inspect one listed resource per run
through the existing guarded HTTPS fetch/byte/hash policy, confirm private raw-file
cleanup, then enqueue a frozen source hash. Serialize the cursor walk through the
existing separate-connection source lease. Advance only after successful planning/
enqueue; retain failure position, converge existing/dead jobs without resetting
attempts, wrap after the complete supported listing and detect correction hashes.

Immediate acceptance: parser URL/size/label/unsupported-profile cases; actual
PostGIS cursor/retry/correction/queue budget and disabled claims; real trusted
Rust→worker→PostGIS import; config/build/static checks; actual static worker image
preparation as nonroot. No new dependency, source activation/deployment, station
association, D scope or production/native-device acceptance is implied.


## Delivered and verified, 2026-10-19

The exact official listing yielded five published annual CSV hrefs:2020,2022,
2023,2024,2025. Grouped2016–2019 and the2021 XLSX remain unsupported profiles;
no annual URLs are invented for them. Native listing parsing bounds bytes,
resolves quoted relative hrefs, ignores labels/duplicates/foreign resources and
sorts verified references. Inspection checks a private regular pinned file and
fetch identity, without parsing or logging private rows. Listing/source cleanup
must succeed before returning a discovery result.

FineDiscovery holds the existing separate-connection source lease, inspects
exactly one reference per run, verifies its exact year/URL/hash/byte budget,
compares the ledger and durably enqueues the frozen native import. Cursor moves
only after successful planning/acknowledgment and wraps after all references.
Lost cursor acknowledgment replays one existing enqueue. Existing dead jobs retain
their status/attempts; a changed resource hash creates one correction intent.
Three real concurrent runs inspected successive resources in order. PMQC cursor,
watermark, Directory identities and unrelated queues are untouched by discovery.

Worker reserves both fine kinds even when disabled and adds independent serial
slots/real typed native composition only under QualityImportsEnabled. Fine daily
scheduling has its own default-off switch, independent of PMQC/registry imports.
All six quality kinds remain excluded from the general dispatcher. Actual SQL
claim tests prove disabled jobs stay queued with attempts0 while ordinary/safety
work proceeds. The existing source fetcher supplies TLS/public-IP/redirect/time/
streamed-hash/private-file controls; executable/file paths never come from jobs.
No dependency or SQL/migration change.

## Validation

- Parser/inspection: misleading labels, quoted relative links, deduplication,
  exact page origin/path, foreign/unsupported/oversize/empty resources, source
  mutation/hash, cancellation, symlink and private-permission refusal PASS.
- Actual PostGIS: cursor loss after enqueue/retry convergence; complete bounded
  walk/wrap; dead budget preservation; correction hash; three concurrent runs;
  eight negative listing/inspection/enqueue failures without cursor/queue writes
  or private error leakage PASS.
- Actual worker discovery→queue dispatch→Rust→PostGIS: repeated discovery creates
  one import, stable published bytes create none, correction creates one second
  import/revision, both jobs done at attempts1, original frozen history retained,
  zero station links inferred and all owned temporary files removed PASS.
- Final scoped race/integration matrix PASS (`/tmp/fq07r4b-final-critical.log`):
  worker4.348s, jobs12.066s, native1.295s, config1.009s, source5.602s.
  Commands use all three documented trusted CLI env paths and
  `go test -race -tags=integration ./cmd/worker ./internal/modules/quality/adapters/jobs ./internal/modules/quality/adapters/native ./internal/platform/config ./internal/modules/official/adapters/source -count=1`.
- Manual live-source smoke (separate integration+qualitysource build tag):
  `TestOfficialFineWorkerNetworkBoundary` PASS5.251s. Actual secure worker fetch
  reads the official listing and2025 CSV, verifies450,504 bytes/SHA256
  `277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96`,
  runs the trusted Rust preparer and reconciles3,395=3,375accepted+0duplicate+
  20CPF quarantine, extraction month2026-01, with confirmed raw-file cleanup.
  A changed remote hash fails the smoke and requires a newly reconciled candidate.
- Locked pinned worker Docker build PASS. Local image
  `sha256:a3dd8d5de3fc6ce39c90ad0d6e8ff3d37f6a762a7fedff4dc657063ae3c1e958`
  contains static-PIE `/prepare_sanctions` mode755. Actual container with
  networknone/user65532:65532 prepared a fully synthetic CP850 record and exited0,
  manifest1input/1accepted/0duplicate/0quarantine. Container removed afterward;
  no registry publication, deployment or source activation occurred.
- Scoped vet/build/staticcheck, gofmt, diff/secret/new-file/link review PASS.

## Recovery and next

Keep imports and the fine discovery schedule disabled until selected deployment
acceptance. Source failures retain cursor position; failed/dead jobs remain
bounded and require reviewed operator recovery, never automatic budget reset.
The worker image can be rolled back independently; immutable source revisions
remain historical facts. Native paths/profile guards cannot be changed by job
payloads. No source activation or staging/production/native-device certification.

Next FQ-07R5: reconcile all actually listed historical source profiles, including
annual CSV variants, grouped2016–2019 and2021 XLSX; then complete safe Directory
association/source clocks, live enforcement FQ-06, linked Android journeys and
FQ-16/17 acceptance/protected delivery. D remains held. This is a local runtime
composition checkpoint, not complete enforcement/fine history or full plan closure.
