# FQ-14B — Android public PMQC journey

Status: LOCAL_DONE
Validation: PASS

Parent d3702420 on maintained dev. B-BR-FQ-01/03/04/05/07/11/12/13/16;
BUC-FQ-03/04/09. Add canonical station quality navigation with public guest
summary, fuel selection, explicitly dated sample/assay pages, revision catalog,
sample detail and source provenance. Never interpret assay counts as samples,
absence as conformity, or nullable score as a rating. Clearly distinguish
historical/superseded/unknown/stale/unavailable facts and method limitations.

Domain models/port stay separate from application read orchestration, data HTTP
and Compose/ViewModel. One bounded page at a time gives complete forward access
without unbounded accumulated rows. Pin assays/details to the summary revision;
verify station/fuel/revision and ignore cancelled/late selection responses.
Add an additive station echo to the assay-page contract for identity checking.
Reads are anonymous, bounded and cancellable, without credentials or raw errors.
Unknown conformity stays unknown. Redirects cannot change the API origin.

Acceptance: guest/no source/score null, unknown conformity, wrong station/fuel/
revision, revision catalog/source validation, page continuation, cancellation and
late result exclusion; affected JVM/contract/lint/APK checks. No new dependency,
app ID/module/license change, device install, D or runtime source activation.
This PMQC slice does not falsely accept the still-owed enforcement/sanction live
streams or the document-free report/offline/rollout phases.

## Acceptance, 2026-10-18

Implemented domain/port → application → anonymous HTTP → Hilt/ViewModel →
Compose journey in the existing modules and station route. Eight resource sets
retain localized source dates, unknown/unavailable states, raw assay versus
sample counts and the explicit unscored explanation. `qualityUiEnabled=false`
independently disables entry and initial reads; source imports stay default-off.
Assay pages now echo station identity; OpenAPI remains additive. Links require
HTTPS www.gov.br:443 under /anp/ without credentials/query/fragment. No new
permission, dependency, media, numeric rating or device identifier was added.

Immediate checks (offline Gradle, disposable local PostGIS):

- `./gradlew --offline :application:test --tests '*ReadQualityTest' :data:testDebugUnitTest --tests '*QualityHttpClientTest' :app:testDebugUnitTest --tests '*QualityViewModelTest' :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest`: PASS; 11 initial JVM cases, no quality lint findings, both APKs compiled.
- Final HTTP-cancellation and nondefault-source-port cases: `./gradlew --offline :data:testDebugUnitTest --tests '*QualityHttpClientTest' :app:assembleDebug :app:assembleDebugAndroidTest`: PASS, five data cases (12 affected JVM cases in total); real stalled socket closes on cancellation.
- Application pinning/unavailable/mixed revision; HTTP credential absence,
  unknown enum, station/fuel/revision/cursor isolation, 1MiB bound, redirects and
  fixed diagnostics; ViewModel ignored-cancellation selection race, late sample,
  disabled clearing and invalid route: PASS.
- `go test -count=1 -race -tags=integration ./internal/modules/quality/adapters/http ./internal/platform/apicontract`: PASS (22.628s/8.410s); after adding the station-echo assertion, targeted `TestAssayTraversal`: PASS (1.831s). Affected `go vet`, whitespace, links and secret surface reviewed: PASS.
- Two native UI cases (large-font guest/no-score and unknown-conformity sample
  detail) COMPILED ONLY. Execution remains OWED under the deferred device batch;
  no emulator/device, staging origin or production acceptance is claimed.

Logs: /tmp/fq14b-android-final.log, /tmp/fq14b-final-hardening.log and
/tmp/fq14b-api.log (local, not committed). No schema change. The complete FQ-14
still owes correctly linked enforcement/sanction journeys and source streams;
this checkpoint accepts only public PMQC. Next FQ-15 document-free owner reports,
then offline/privacy, remaining source streams and scoped final acceptance.
D documents/business portal remain DEFERRED_FUTURE / NOT_AUTHORIZED.
