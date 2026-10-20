# FQ-07R5D — Atomic historical fine publication and accurate public clocks

Status: LOCAL_DONE
Validation: PASS

Parent 73312985, maintained dev. B-BR-FQ-01/02/06/07/14/16;
BUC-FQ-03/04/06. One typed publication/read-contract checkpoint.

Before behavior: validate the typed source context/counts/resources/observations/
notes before I/O. Stage context, minimized rows and exact public source notes in
one existing edition/provenance transaction; verify staged counts. Frozen source
context joins publication identity only when present, preserving all legacy/2025
keys. Replay/live guards/correction/failed-stage rollback/current/watermark order
remain unchanged. Empty snapshots retain source precision and counts. Never
infer station links, decision dates, paid zero or source-year splits from the group.

Public frozen leases/page/detail disclose the actual source period/contract/clock,
original reference label/value, nullable source-declared transit/paid and unlabelled
fact without CNPJ/names. Source notes are distinct bounded allowlisted payment
notes; metadata records remain separately counted and never become fine events.
Source age applies the existing180-day rule independently of local publication,
including actual DAY precision and empty snapshots; no invented extraction time.
Native2025 and legacy public contracts remain additive/compatible. Current and
explicit historical revisions, keysets/ETags/cursors and kill switch must remain
correct. Immediate real PostGIS replay/concurrency/live-guard/rollback/history/
empty-clock and actual HTTP/OpenAPI/privacy/cache acceptance is mandatory.

No source runner/discovery/image activation, Directory station association,
production/native-device acceptance, D work or deployment follows from this slice.


## Delivered and verified, 2026-10-19

HistoricalFineInput validates the exact source context/resource/clock, bounded
nonnegative reconciled counts, original observation/note locators and footer
ordering before I/O. Context record count equals input; metadata is an explicit
subset of quarantine, never extra events. Unknown paid/department, original AI/
DUF labels, source-declared transit and raw unlabelled values survive unchanged.
No station links or numeric quality scores are inferred.

PublishHistoricalFines reuses the existing edition lock, verified immutable
provenance/publication key, correction/current-order/replay and live guard. One
transaction stages source context, all observations and allowlisted source-note
annotations, checks both event and note counts, then publishes. Failed IDs/rows/
notes roll back the entire revision/context/source manifest/pointer operation.
Historical source context joins the publication key only when present; explicit
byte-compatibility tests preserve old memberless and native2025 key serialization.

Frozen public leases/page/detail expose actual source period/input contract,
source clock precision/value, reference label/value, nullable source transit/
paid and raw unlabelled facts, without company IDs/names. Native2025 gets additive
unified source/reference fields while retaining all previous year/month/DUF fields;
legacy exact-day snapshots remain compatible. Public payment caveats are distinct,
allowlisted and bounded to at most two texts per source; blank separators remain
in metadata counts without display text/events. Source record/metadata counters
remain separate from the actual page event denominator.

The existing180-day policy now also accepts actual source DAY precision. Its
midnight boundary is used only as a conservative age bound, never displayed as an
extraction instant. Both DAY and MONTH source age remain independent of local
publication time, including empty snapshots. Current versus explicitly selected
historical leases, source notes, ETags, keysets and stale-cursor refusal preserve
frozen revision identity. Source fields/OpenAPI are additive; reads remain anonymous
and the existing kill switch still contains every route.

## Validation

- Pure application/domain RED→GREEN: valid/empty source contexts, strict resource
  identity and original locators, count/profile/note mismatch and balanced negative
  counts refuse before publication; DAY calendar/future/unknown/180-day boundary
  checks PASS. Legacy/native2025 publication-key byte compatibility PASS.
- Actual PostGIS: replay converges, live guard runs on replay, same-period
  correction freezes the predecessor and refuses superseded resurrection. Failure
  after the first staged row rolls back context/revision/pointer; a failing note
  trigger after all rows rolls everything back and a later retry succeeds.
- Eight concurrent publishers for each of all six historical profiles converge
  to one revision per profile, with original fields/public notes intact. Empty
  grouped snapshots retain DAY clock/counts and cannot move a newer2025 current
  pointer. Source history is explicitly selectable without pretending it is current.
- Actual HTTP/PostGIS/OpenAPI for group DAY,2020 notes and2023 unknown paid PASS
  (`/tmp/fq07r5d-http.log`,4.657s): page/detail agree, no CNPJ/name key exposure,
  exact null/reference/transit semantics, complete two-page traversal, stable304,
  source-age boundary changes ETag/stale state, correction invalidates old cursors,
  current old-row404 and explicit superseded-row200 retain the old source clock.
- Complete actual source acceptance, with private pinned source copies and real
  trusted Rust CLI→Go→PostGIS publication/replay→all-record keyset traversal:
  all32,237historical observations from32,516records PASS32.453s under race
  (`/tmp/fq07r5d-full-source.log`). Every visible field is compared to the verified
  native batch;1,303unknown paid,159unknown departments and34unlabelled SIM values
  survive. Six contexts/32,237events/six note annotations/zero station links remain.
- Actual2025 trusted native runner→Go→publisher/replay→complete traversal PASS3.872s
  (`/tmp/fq07r5d-2025-source.log`):3,395records=3,375accepted+0duplicate+20CPF.
  Its earlier pinned source hash/independent oracle remains unchanged. Combined
  seven-resource denominator is35,911=35,612accepted+0duplicate+293CPF+6metadata.
  This is complete traversal of the verified published resources, not guaranteed
  national case coverage or station association.
- Final ten-package critical race/PostGIS matrix PASS
  (`/tmp/fq07r5d-final-critical.log`): adapters37.624s, HTTP19.538s,
  jobs14.825s, native1.152s, prepared1.124s, read9.584s,
  application1.016s, domain1.015s, migrate5.406s, apicontract5.727s.
- Affected vet/staticcheck, API/worker build, SQLC vet/regeneration stability,
  OpenAPI actual-response validation, gofmt/diff/secret/new-file/link review PASS.
  No dependency, migration, private source artifact or production data committed.

```sh
# backend/, documented trusted CLI env paths/disposable local PostGIS
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/absolute/trusted/prepare_quality ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/absolute/trusted/prepare_sanctions ANPFUEL_TEST_PREPARER_PATH=/absolute/trusted/prepare_registry go test -race -tags=integration ./internal/modules/quality/... ./internal/platform/migrate ./internal/platform/apicontract -count=1
# Explicit private-source acceptance, no network; both history and2025 env paths
# are required when running both named tests.
ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH=/absolute/trusted/prepare_fine_history ANPFUEL_TEST_FINE_HISTORY_SOURCE_DIR=/private/source/directory ANPFUEL_TEST_SANCTIONS_PREPARER_PATH=/absolute/trusted/prepare_sanctions ANPFUEL_TEST_SANCTIONS_SOURCE_PATH=/private/pinned2025.csv go test -race -tags='integration qualitysource' ./internal/modules/quality/adapters -run '^TestOfficialHistoricalFineRustGoPostGISFullTraversal$|^TestOfficialNative2025FinePublicationFullTraversal$' -count=1 -v
```

## Recovery and next

Keep imports/discovery disabled. No runtime producer currently publishes the new
historical profiles. No worker image/staging/production was deployed, no source
activation or main integration is certified, and native Android execution stays
OWED under the existing deferral. Applied storage migrations are unchanged.

Next FQ-07R5E: trusted bounded history runner, strict seven-resource selection,
confirmed raw cleanup before publication, shared source lease, actual href-only
history discovery and static worker image with the existing default-off controls.
Then safe Directory/current-company association, live FQ-06 XLSX, linked Android,
FQ-16/17 acceptance and protected final delivery/overview. D remains held. Full
plan/runtime/station-history completion is not claimed by this checkpoint.
