# PR 150 cumulative integration — 2026-10-25

Status: LOCAL_DONE
Validation: PASS (local cumulative source acceptance)
Integration: INTEGRATION_PENDING

Scope: preserve PAY through `1948035c`, native partial MAP through `e9af87dd`,
enterprise/static portal through `d74334dc`, including FQ-PORTAL-READY.
Primary dirty checkout and occupied dev remain untouched; integration uses
an isolated clone with maintained dev and protected PR 150 -> main.

B-BR-INTEGRATION-01: payment inbox, export and refresh retain independent
serial slots; the general dispatcher cannot steal these jobs.
B-BR-INTEGRATION-02: a fresh CI clone consumes the exact admitted AAR (not
substitute Java stubs), verifies its digest and both ARM cores, and performs
real app/module unit compilation. Public source references, patch, notices
and SBOM accompany the binary; no MWM packages or keys are bundled.
B-BR-INTEGRATION-03: map paths select full quick/mobile guards; unknown areas
remain refused. No required check is suppressed.

Previous Quick failure: real HTTP stream size test returned EOF on an oversized
chunked request after the harness's 30-second timeout. Local pre-change repeat
10 passed; no deterministic production defect is asserted. Isolate connections
and drain response bodies without changing exact admission/status assertions.

Map minimum acceptance remains PARTIAL. PAY-21 sandbox proof, source rights,
precision policy and public portal certification remain owed. Staging origin
`https://teste.abastevo.com.br` is explicitly authorized for subsequent scoped
updates; public-data admission remains unchanged.

## Cumulative candidate validation

Validated source checkpoint: `2707c218` (record commit follows).

- Preserved source ancestry: PAY `1948035c`, MAP `e9af87dd`, ESP/FQ
  `d74334dc`; merge commits `df9612af` and `0284128f`.
- Full backend `go test -race -count=1 -tags=integration ./...` ran against
  an exclusive tmpfs PostGIS with all seven pinned Rust preparers, modes 755.
  It exposed three local acceptance-test incompatibilities. All other packages
  passed. After fixes, the full affected mapdata/official-adapters/payment-
  adapters packages passed again under `-race -tags=integration`.
- Migration chain from empty applied all 131 revisions. The map upgrade now
  asserts the exact cumulative 000122–131 sequence; the payment upgrade retains
  each 000123–130 assertion and adds the appended official 000131 migration.
  Replay/table/PostGIS assertions remain in place.
- The official Stage-vs-Finish test previously reused chronological publication
  history across four races: an earlier 4-row winner made a later 3-row winner
  correctly fail the >20% row-drop gate. Each race now owns a fresh importer/DB;
  the same published/frozen/3-or-4-row assertions remain, with no domain waiver.
- Fresh-CI failure found a second app-side hardcoded SDK path. Root Gradle now
  resolves and provisions once, before ABI selection; app and engine consume
  that identical path and both ARM cores remain mandatory.
- `./gradlew --offline test :app:lintDebug :app:assembleRelease
  :app:verifyReleaseApkSize --max-workers=2`: PASS using the vendored AAR,
  without a comapsSdkDir override. Domain 675, application 454, data 609 per
  debug/release (2 unrelated external skips per variant), engine 32 per
  debug/release, app 535 per debug/release/mapProbe: zero failures/errors.
  Lint and unsigned release PASS; 78,121,062 bytes / 74.502 MiB (<80 MiB).
- Worker/config race tests and HTTP size/route-cap repeat 20 PASS. Classifier
  cases 3/3 and native provisioning/license/resource Python tests 13/13 PASS.
- `git diff --check` and tracked/new-file secret scan PASS. No required check,
  size/security bound or production source policy was relaxed.

Local logs remain in `/tmp/pr150-*.log`; exact-head remote checks/reviews and
merge ancestry are finalization evidence, not inferred by this source record.
No live provider call, sale, new source admission or contributor-data fixture.

## Staging execution disposition

The owner explicitly selected the existing `https://teste.abastevo.com.br`
origin and authorized the existing VPS credentials for scoped updates after
merge. Target is the existing Abastevo namespace only. Keep payment sales off,
keep new import/export automation opt-in, preserve the private-storage sidecar
and other workloads. Snapshot DB/deployment/edge first, rehearse append-only
migration/restore, then deploy matched static API/worker/migrator binaries.

The static official-only slice keeps noindex and its source admission. Update
publisher/retained-asset routing, verify exact manifest, links, health/API and
A→B→A/withdraw/recovery under the maintained release contract. A test origin
remains staging; full public portal, source precision policy and G09/PAY-21
certification are not accepted by an integration/deployment alone.
