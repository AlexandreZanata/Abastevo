# Current execution state

- C1 app public contributors list 2026-10-27: LOCAL_DONE /
  INTEGRATION_PENDING. Branch `codex/adm-c1-app-contributors` from
  `origin/main` `d4dcf716`. Anonymous cursor-paged list: domain model,
  OkHttp client, mapper, use case, ViewModel states, ContributorsScreen,
  Premium-accent entry as last Profile item + route. Tested revision:
  branch head; domain 781 / data 629 / application 484 / app 561 unit
  tests, 0 failures; lint/assemble/androidTest-compile green;
  instrumented tests compile but need a device/CI run (attached device
  refuses installs). [Evidence](../../backend/evidence/admin/C1-APP-CONTRIBUTORS.md).
  PR #173 to `main`. Next: C2 landing page, then C3 self-service.
- CD-04 staging k3s executor 2026-10-27: LOCAL_DONE / INTEGRATION_PENDING.
  R2 captured the live `abastevo-temp` objects into `infra/k8s/staging/` with
  placeholders; the rendered namespace passed `kubectl diff` with exit 0 after
  ignoring only server-managed metadata. R7 ships the landing layer in the
  signed OCI artifact. R8 adds namespace-scoped RBAC and a no-token-print
  kubeconfig helper, not applied. Apply mode adds the schema gate, verified
  backup, migration Job, one shared image rollout, smoke, landing symlink,
  `current.json` last, undo/failure receipts and a quota-gated surge default
  off. Review fixes add the `baseline` subcommand, require `current.json` for
  apply (no live-annotation gate), write the `schema-floor` annotation, add
  `migrate -check`, and pass the R5 two-dispatcher overlap integration test.
  157 harness cases, Quick, shellcheck and actionlint PASS. Part B applied the
  RBAC, generated the mode-600 kubeconfig, installed the agent/env under the
  authorized host paths, staged the unit/drop-in, and ran the baseline with
  SHA `d4dcf716` (DB/floor `000139`, running `stg-7c7d776f` images) plus a
  plan-only receipt. No apply or timer. Persistent `/etc/systemd/system`
  install, service-user ownership and the two-merge staging acceptance remain
  OWED; no production action.
  [CD-04 evidence](../backend/evidence/cd/CD-04.md).

- CD-03 schema compatibility window 2026-10-27: LOCAL_DONE / INTEGRATION_PENDING.
  Phase 3 prerequisite: embedded `schema_min_supported` (`000132`),
  `migrate.Check` accepts a newer additive schema while rejecting missing,
  modified and older-than-floor ledgers; the release manifest publishes and
  validates the floor, and the Phase 2 agent carries it in receipts. Unit and
  PostGIS integration tests, 36 agent tests and Quick PASS. The rollout
  executor remains OWED because staging is Compose while Phase 3 assumes k8s,
  and the landing archive still needs an anonymous distribution path. No VPS,
  k8s, nginx or production action.
  [CD-03 evidence](../backend/evidence/cd/CD-03.md).

- DRV-00..25 driver suite 2026-10-27: PR #152 integration into `dev`, local
  slices only and `ANPFUEL_DRIVER_ENABLED` off by default. The open work that
  closes DRV-18..25 is listed in
  [DRIVER_SUITE_EXECUTION_PLAN section 10](DRIVER_SUITE_EXECUTION_PLAN.md#10-remaining-work-after-the-pr-152-integration-2026-10-27).
  Owner decisions D1-D3 come first. DRV-26..35 are not started.

- DRV-18/21 continuation: IN_PROGRESS / partial, on `driver-suite/drv-00-02`.
  [Evidence](../backend/evidence/driver-suite/DRV-18-recovery-continuation.md).
  After shift/GPS pause/end/recovery integration, focused JVM tests, APKs,
  app lint and mobile static guard passed; combined synthetic screen/Room and
  GPS instrumentation passed 8/8 on owned API35 6 GiB. Earlier failed and
  crashed attempts remain recorded. The local 60 s/1 s checkpoint values come
  from test vectors, not an admitted operational policy. Session/logout and
  Room commit races, GPS account/permission changes without callback, quality,
  field/battery, lifecycle, accessibility and complete workday remain open.
  Last recorded global domain coverage was 81.81% versus the required 90%;
  it was not remeasured. No DRV-18/21 acceptance or batch integration claim.

- DRV-22..25 2026-10-26: IN_PROGRESS / local slice, parent 3fd99ee9,
  behavior 757eb61f. [Evidence](../backend/evidence/driver-suite/DRV-22-25.md).
  Room schema 32 adds reconciliation, goals and maintenance; work screen adds
  basis/odometer/GPS/paid km reconciliation, local result/per-km, daily/weekly
  goals and km/day maintenance plans with service advance. JVM calculations,
  Room 31→32 migration on API35, app/data compile and static guards PASS.
  Break-even, cost-window coverage, reserve/vehicle reset, Compose/accessibility
  and field GPS/battery proof remain open; no five-phase acceptance.

- DRV-21 2026-10-26: IN_PROGRESS / explicit FGS slice, behavior 313e5c63.
  [Evidence](../backend/evidence/driver-suite/DRV-18-22.md). Added
  DriverGpsService (visible notification, GPS provider/2 s, explicit start/stop,
  START_NOT_STICKY, local distance row) and updated the static location guard to
  allow only this service. Runtime permission UX, process-death/battery/
  coexistence and Play policy proof remain open. No field/device acceptance.

- DRV-18..22 2026-10-26: IN_PROGRESS (local slice only), parent c78b0f89,
  behavior 3517820b, merge c4266814. [Evidence](../backend/evidence/driver-suite/DRV-18-22.md).
  Added Room schema 31 (shift odometers, payouts, cost settings, GPS table) and
  a local work screen for manual shift start/pause/resume/end/recovery,
  earnings/payout entry, cost settings and a basic summary. Room 30→31
  instrumentation, JVM migration/parsing, app/data compile and static guards
  PASS. DRV-21 GPS/FGS and DRV-22 reconciliation are NOT implemented; behavior
  UI tests for 18–20 remain open. Published to PR #152 for incremental review,
  not as five-phase acceptance.

- DRV-17 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent a6c2837d,
  behavior 24c85e5a. [Evidence](../backend/evidence/driver-suite/DRV-17.md).
  Hub quick refuel/charge routes into the real exact ledger forms, separate
  charge fees field, fees/energy/total/SOC/tariff persistence, pt-BR and
  double-tap/restart coverage, full-partial-full/GNV/mixed/PHEV unknown
  regression. JVM ledger/domain/routes and full app unit/compile PASS; owned
  API35 10/10 instrumentation PASS; static guards PASS. No new sync/GPS/shift;
  no push/PR/merge.

- DRV-16 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent efdcce26,
  behavior d1db75ab. [Evidence](../backend/evidence/driver-suite/DRV-16.md).
  Real driver hub route from Perfil/Veículos, VERIFIED/OFFLINE/LAPSED/
  UNAVAILABLE/SIGNED_OUT states, retained vehicle/ledger/export actions,
  actionable first-use/error states and four-tab navigation regression. JVM
  route/state tests, app unit/compile PASS; owned API35 emulator 10/10
  instrumentation PASS; static guards PASS. Shift form intentionally deferred
  to DRV-18; no push/PR/merge.

- DRV-15 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent 27a0fc95,
  behavior 24aa536a. [Evidence](../backend/evidence/driver-suite/DRV-15.md).
  Explicit per-dataset cloud consent (migration 000138), four driver datasets,
  strict derived-payload validation, two-device pull/snapshot/restore,
  conflict/replay, foreign refusal and erasure epoch/old-snapshot purge. Full
  cloudsync/driver tests under -race, backend unit, vet/staticcheck/sqlc PASS.
  No Android client/UI or driver_shifts registration; no push/PR/merge in this
  checkpoint.

- DRV-14 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent b5957f53,
  behavior 72f8bd46. [Evidence](../backend/evidence/driver-suite/DRV-14.md).
  PENDING/SYNCED/CONFLICT outbox, 29→30 lease migration, owner/guest-isolated
  claims, expired-lease reuse, bounded backoff/retry-limit and exact shared
  Go→Kotlin receipt/error fixtures. Android API35 7/7 instrumentation, Go
  fixture test, JVM data tests, app compile and mobile static guards PASS.
  Disk-full test not executed (explicit gap). No live dispatcher/cloud/UI.
  No push/PR/merge in this checkpoint.

- DRV-13 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent 6d665c3b,
  behavior 062a4030. [Evidence](../backend/evidence/driver-suite/DRV-13.md).
  Room schema 29 with owner/origin-scoped shift/gap, earning/revision, distance,
  local command and draft tables; transactional writes, append-only 28→29
  migration, owner A/B/guest isolation and exact integer-unit restart. Real
  emulator API35: 4/4 instrumentation PASS; JVM migration/repair tests, full
  data unit tests, app compile and mobile static guards PASS. No UI/GPS/sync/
  cloud; DRV-14 next. No push/PR/merge.

- DRV-11 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent 568ae322,
  behavior 470e1d26. [Evidence](../backend/evidence/driver-suite/DRV-11.md).
  Revisioned DAY/WEEK/MONTH/CUSTOM projections, exact cost/distance coverage,
  stale report refusal, private snapshots, signed HTTP earnings/report routes,
  owner cursors, no-store, feature flag and bounded worker cleanup (migration
  000137). Real PostGIS/HTTP/worker/application/domain checks PASS under -race;
  full backend unit, vet, staticcheck, sqlc, OpenAPI lint and golden vectors
  PASS. No mobile/provider/staging acceptance. No push/PR/merge.

- DRV-12 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent 470e1d26,
  behavior 69bdb374. [Evidence](../backend/evidence/driver-suite/DRV-12.md).
  Real signed HTTP chain: earning save/replay/erase, shift start, cost save,
  report build/get, correction/stale, foreign-owner denial; snapshot restart,
  soft erasure and expiry cleanup. 51 top-level PASS under -race, full backend
  unit/build/vet/staticcheck/sqlc/gofmt/OpenAPI PASS. Command HTTP is limited
  to earning/shift/cost and erasure; no provider/staging/mobile claim.

- DRV-10 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent fd176a8a
  behavior 0c98c0b4. [Evidence](../backend/evidence/driver-suite/DRV-10.md).
  Signed command service, transactional professional capability hook, versioned
  summary/detail and payout/earnings decisions, and retained reads after lapse
  were added by migration 000136 after 000135. Real disposable PostGIS: 24
  adapter tests plus 19 driver domain and one application test group PASS under
  -race, including concurrent duplicate saves, cancellation after writes and
  stale epochs. No HTTP/identity wiring/cloud/mobile admission; DRV-11 next.
  No push/PR/merge here.

- DRV-09 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent babf66a7,
  behavior 9241b869. [Evidence](../backend/evidence/driver-suite/DRV-09.md).
  Versioned private cost profiles/items and maintenance schedules, exact
  transaction-bound ledger links, zero-versus-unknown values, odometer segment/
  reset context, owner epochs and record tombstones were added by migration
  000135 after 000134. Real disposable PostGIS: 21 adapter tests plus 19 driver
  domain tests PASS under -race, including concurrent ledger fencing and soft
  erasure. Full backend unit, vet, staticcheck, sqlc and formatting PASS. No
  HTTP/Premium/cloud/GPS/mobile admission; DRV-10 next. No push/PR/merge here.

- DRV-08 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent 84dfd74d,
  behavior 4026df2a. [Evidence](../backend/evidence/driver-suite/DRV-08.md).
  Durable private shifts, monotonic recovery gaps, version-fenced receipts,
  aggregate distance observations and owner/vehicle scoping were added by
  migration 000134 after 000133. Real disposable PostGIS: 16 adapter tests plus
  17 driver domain tests PASS under -race, including two-device start, replay,
  stale pause, rollback, overlap and soft-erasure cases. Full backend unit,
  vet, staticcheck, sqlc and formatting checks PASS. No HTTP/Premium/cloud/GPS/
  mobile admission; DRV-09 next. No push/PR/merge in this commit.

- DRV-03–07 batch: LOCAL_DONE / INTEGRATION_PENDING, isolated driver PR #152
  (`driver-suite/drv-00-02` → `dev`, base `507d915e`). Exact money, shifts,
  distance quality, cost allocation and private earnings storage are delivered;
  [batch evidence](../backend/evidence/driver-suite/DRV-03-07_ACCEPTANCE.md) records
  real PostGIS/Android proofs and the inherited global coverage blocker.
  API26 found an unavailable BigInteger method; checked compatible conversion
  now passes the same two runtime tests. No threshold waiver or aggregate PASS.
  Final publication/check receipt belongs to PR metadata; no merge/deploy/wiki.
  Stop at DRV-07; DRV-08 onward require another request. Other fronts preserved.

- DRV-07 LOCAL_DONE / INTEGRATION_PENDING at `7d830547`: private earnings, source identity and revision persistence. [Evidence](../backend/evidence/driver-suite/DRV-07.md). Immediate risk tests PASS; isolated driver PR #152. Next DRV-08 (not authorized); no main/dev push or merge.

- DRV-06 LOCAL_DONE / INTEGRATION_PENDING at `d21e5a34`: vehicle cost profiles and exact allocation. [Evidence](../backend/evidence/driver-suite/DRV-06.md). Immediate risk tests PASS; isolated driver PR #152. Next DRV-07; no main/dev push or merge.

- DRV-05 LOCAL_DONE / INTEGRATION_PENDING at `fadfca59`: distance sources, partitions and gps quality. [Evidence](../backend/evidence/driver-suite/DRV-05.md). Immediate risk tests PASS; isolated driver PR #152. Next DRV-06; no main/dev push or merge.

- DRV-04 LOCAL_DONE / INTEGRATION_PENDING at `6a66f414`: shifts, clock recovery and multiapp time. [Evidence](../backend/evidence/driver-suite/DRV-04.md). Immediate risk tests PASS; isolated driver PR #152. Next DRV-05; no main/dev push or merge.

- DRV-03 LOCAL_DONE / INTEGRATION_PENDING at `97b42c60`: exact earned/cash/recorded/modeled rules, twelve shared production vectors, Go/Kotlin negative and weighted-money tests PASS. [Evidence](../backend/evidence/driver-suite/DRV-03.md). Isolated driver PR #152; next DRV-04; no main/dev push or merge.

- DRV-02 completion review 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING at
  behavior `29f6c4fe`, manual oracle `566c3516`. [Evidence](../backend/evidence/driver-suite/DRV-02.md).
  Preserved owner-reviewed legacy repair; fixed post-read owner/login/consent
  races, revision drift, gap/reset transport and malformed proof coercion.
  Final application 479 + affected data 25/app 30 tests, build/lint PASS;
  actual Room 8/8, Compose 5/5 on own API35/16KB emulator, then shut down.
  Added 15 manual gain/time reference rows executed in Go/Kotlin; runtime
  earnings/shifts remain future work. Original financial goldens unchanged.
  Foundation gate PASS locally; DRV-03–35 PLANNED. Isolated maintained dev;
  other branches/checkouts preserved, no push/PR/merge/wiki/deploy.

- DRV-02 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent d4223b5c.
  [Evidence](../backend/evidence/driver-suite/DRV-02.md): legacy odometer units
  are UNVERIFIED until an owner-reviewed, CAS-fenced, receipt-backed choice;
  derived consumption/cost/km blocked meanwhile, purchases kept; local envelope
  never sent. Real Room 6/6 and Compose 5/5 on owned emulator-5568; Go+Kotlin
  unit oracle (19 rows) and unchanged financial-v1 PASS; final Gradle matrix PASS.
  **DRIVER_FOUNDATION_GATE: PASS locally** (DRV-00/01/02), not production or
  full-suite acceptance. DRV-03–35 not started.

- DRV-01 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent e9f1ce25.
  [Evidence](../backend/evidence/driver-suite/DRV-01.md): real form RED→GREEN,
  exact km/metres with localized edit, BRL/km from rational centavos and
  overflow refusal. 18 focused tests, app debug build and final label/lint PASS.
  No schema/backend/provider/device change. Next DRV-02 legacy owner review;
  other primary/MAP branches preserved, no delivery/deploy authorized here.

- DRV-00 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING in isolated planning
  clone, parent42250934/runtime f1e1d0db. [Evidence](../backend/evidence/driver-suite/DRV-00.md)
  freezes Premium, exact km/money, legacy review and manual/GPS/owner boundaries,
  five driver scenarios and existing schema/limits. Remote main eee1b33f and
  dev f1e1d0db verified; foreign primary/MAP edits preserved. Next DRV-01 RED
  and actual existing-entry fixes; no DRV-03+, push/PR/merge/device/VPS.

- Driver suite planning 2026-10-26: PLANNED / IMPLEMENTATION_NOT_STARTED.
  Owner confirmed professional Premium. [DRV-00–35](DRIVER_SUITE_EXECUTION_PLAN.md)
  defines 36 phases/108 tasks for existing financial-unit repair plus real
  private shifts/earnings/distance/cost/report mobile journeys. [Contract](../product/DRIVER_SUITE_CONTRACT.md)
  explicitly amends named prior exclusions without changing financial-v1,
  free/public behavior or raw-GPS cloud exclusions. [Planning validation](DRIVER_SUITE_PLAN_VALIDATION.md)
  checks identifiers, dependencies, links and 20 synthetic worked vectors only.
  Isolated planning source f1e1d0db; foreign-dirty primary/MAP checkouts preserved.
  No runtime/device/provider/VPS/push/PR/merge/wiki/production change.
  Next, only on implementation request: DRV-00 → DRV-01 → DRV-02.
- Map panel-collapse follow-up 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING from
  dev `e64b655f`, existing PR #154. A visible 48 dp chevron hides/reopens the action
  container; attribution remains visible and saved-state restoration retains the
  local choice. Existing overlay measurement updates recenter inset in both states.
  APKs/lint (zero errors), API30 native 13/13 (including restore, callback, geometry
  and 200% font), reviewed captures, diff/secret checks PASS. New hide/show labels
  cover eight locales. [Evidence/previews](../backend/evidence/maps/MAP-UI-REFINEMENT-20261026.md#follow-up-minimal-action-panel-collapse-control).
  Original occupied checkout/emulator preserved; no new physical/16 KB, deployment
  or release acceptance. Next: normal dev push/update of #154; leave PR open for
  current-head CI/review without merge.

- Map action-panel follow-up 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING from
  dev `c04e8ac4`, existing PR #154. OpenStreetMap attribution is a separate map
  overlay above/right of the action panel. Filters/Search/Download/Saved reuse
  shared action cards in an adaptive grid; measured overlay height and reserved
  clearance keep recenter accessible. Updated skeleton; existing callbacks kept.
  APKs/lint (zero errors), full API30 UI 12/12, light/dark/200% captures,
  diff and secret checks PASS. [Evidence/previews](../backend/evidence/maps/MAP-UI-REFINEMENT-20261026.md#follow-up-map-attribution-and-standard-action-cards).
  Original occupied checkout and emulator preserved; no new physical/16 KB or
  staging/release acceptance. Next: normal dev push/update of #154; current-head
  CI/review pending, leave PR open without merge.

- Map city/fuel follow-up 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING from
  remote dev `cb111741`, existing PR #154. City aligns to the header start and
  truncates visually with ellipsis; the fuel field reuses the shared vehicle
  selector/modal instead of adjacent chips, retaining Apply/Cancel drafts.
  Debug app/test APKs, lint (zero errors), diff/secret review and API30 native
  10/10 PASS, including actual city text-layout assertions and eight choices at
  200% font. Physical phone disconnected; API35 16 KB emulator attempts crashed
  and are not accepted. [Evidence](../backend/evidence/maps/MAP-UI-REFINEMENT-20261026.md#follow-up-left-aligned-city-and-shared-fuel-selector).
  Original occupied checkout/dev preserved. Next: normal push/update of #154;
  current-head CI/review pending, no merge or staging change.

- Map UI refinement 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING from remote
  dev parent `6a689e58`; delivery target [PR #154](https://github.com/AlexandreZanata/Abastevo/pull/154),
  `dev` -> `main`. Compact expandable map controls; grouped filter sheet with all
  fuel icons and pinned Apply/Cancel; actual state/municipality location search
  preserves filter drafts. App JVM 92/92, engine 37/37 (cached), final APKs,
  phone 9/9 (including dark/200% font), lint (zero errors), diff and secret review
  PASS. [Evidence/screenshots/tested fingerprints](../backend/evidence/maps/MAP-UI-REFINEMENT-20261026.md).
  Final debug APK installed without clearing data. Original occupied checkout/dev
  preserved; no staging change or merge. New-head CI remains pending publication;
  next: normal push/update of PR #154, leave open for current-head CI/review.

- Map UI isolation 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/map-ui` (parent `fe6ca851`). Native map moved to the shell and kept
  alive across tabs; no engine/surface reinit on return; tab transitions are
  instant (no crossfade ghost); full-size map with overlay controls; skeleton
  gates on the first rendered native frame; FlowRow filters and stable 48 dp
  action bar; map screen instrumented tests 5/5, targeted unit/engine tests and
  lint PASS. Installed locally on the authorized phone; not pushed/merged.
  [Evidence](../backend/evidence/maps/MAP-UI-ISOLATION-20261026.md).
  Staging `nearby` empty/data update remains separate.

- C2 landing contributors page 2026-10-27: LOCAL_DONE /
  INTEGRATION_PENDING. Branch `codex/adm-c2-landing-contributors` stacked
  on C1. Static `/contribuidores/` shell + `contributors.ts` progressive
  enhancement (meta-gated API origin, textContent-only cards, CTB-08 link
  allowlist), per-path CSP, sitemap/llms entries. Tested revision: branch
  head; landing gate green (69/69). [Evidence](../../backend/evidence/admin/C2-LANDING-CONTRIBUTORS.md).
  PR #174 stacked on #173. Next: C3 self-service.

- CD-02 staging deploy planner 2026-10-27: LOCAL_DONE / INTEGRATION_PENDING.
  Pull-based dry-run agent in `infra/deploy/agent/` verifies the signed
  `staging` manifest (keyless identity/issuer), checks main ancestry through
  the anonymous GitHub API, compares `current.json` and writes a mode-600 plan
  receipt without deploying or changing state. `make test-deploy-agent` 33/33,
  shellcheck, a real-artifact dry run and real tamper/foreign-identity refusals
  PASS. VPS install/timer and Phase 3 rollout remain OWED.
  [CD-02 evidence](../backend/evidence/cd/CD-02.md).

- CD-01 staging release artifacts 2026-10-27: INTEGRATED / LOG_CLEAN_RUN_PENDING.
  PR #158 (`ef86cc93`) and the ORAS pin fix PR #159 (`2cd8f0ac`) are merged.
  Run `40494565772` published the signed manifest and images; cosign,
  attestations and anonymous GHCR pulls passed. `setup-node` was replaced by a
  preinstalled-Node check so runner cache paths stay out of logs; the
  log-clean rerun is pending. No VPS, deploy, production or Phase 2 work.
  [CD-01 evidence](../backend/evidence/cd/CD-01.md).

- Landing LH09 / portal shared chrome 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING.
  User-authorized branch `codex/landing-portal-visual` from `origin/main` (dev
  carries unrelated open PR #151) with a PR to main. Uber-style light-only home
  with hero search hand-off to `/consultar/?uf=..&q=..`, black explore band,
  mirrored features and owner-supplied open-data art; portal pages share the
  landing header/footer via `portalgen --landing-url`, editorial methodology and
  approved-question cards. [Contract](../product/LANDING_CONTENT_CONTRACT.md#lh09-uber-style-home-amendment--2026-10-26).
  Landing typecheck/build/check and 61/61 tests, enterpriseexport/portal cmd
  race tests, vet, gofmt, secret scan and `git diff --check` passed locally.
  No deployment, DNS or public portal publication; required Quick verification
  and review gate the merge.
- MAP-33 cumulative delivery: **INTEGRATED / WIKI_SYNCED** for source only.
  Existing `dev → main` PR #151 (not PR #150; PR #152 driver untouched) merged
  at `bbd96568180563ced975b96f8f89f7a3c242522e` (2026-10-26T19:28:55Z), checked
  head `04e9a6b3`. Quick verification, fast, integration and test all passed;
  no failed/skipped/cancelled required check and no review was requested.
  `main` and retained `dev` both matched the merge SHA; wiki published from
  `bbd96568` as commit `3c08ac4` (778 pages, independently verified source
  SHA). This post-merge docs note is on dev only. MAP-29 PARTIAL, MAP-30
  EXTERNAL_PENDING, MAP-32 STAGING_PENDING and the external/manual obligations
  remain. [MAP-33 evidence](../backend/evidence/maps/MAP-33.md).

- MAP-32 local checkpoint: LOCAL_DONE (runbook + local restore rehearsal) /
  STAGING_PENDING. Frozen candidate manifest, staging-scoped apply/backup/
  rollback/smoke/stop-condition runbook; disposable PostGIS dump/restore matched
  132 migrations and 163 public tables. The staging origin answered read-only,
  but no health contract at `/`, `/health`, `/healthz`, `/v1/health` or
  `/api/health` (all 404) and no authenticated operator/SSH/DB access was
  available: no staging update or PASS. [MAP-32 evidence](../backend/evidence/maps/MAP-32.md).
  Production/G09/store/pilot/payments/portal remain out of scope.

- MAP-31 local checkpoint: LOCAL_DONE for the local source union;
  external/device cases OWED. Kotlin 2,466 tests (0 failures, 4 explicit opt-in
  skips), Rust 38 tests + fmt/clippy, Go unit race all green, real disposable
  PostGIS integration all packages green after 132 migrations, backend
  vet/gofmt/sqlc/staticcheck/govulncheck, Android lint and release size guard
  PASS. Final APK 78,396,538 bytes / 74.76 MiB, SHA-256 `feadd9f3…`, both ARM
  ABIs and 16 KB LOAD alignment. Phone guidance/off-route and EV real-engine,
  emulator UI/accessibility, and local 132/163 restore also pass. The four
  opt-in Go/API/TLS Kotlin wire tests, two-account/two-device restore, manual
  TalkBack, API26/clean ARMv7, frame pacing, physical battery, national and
  staging remain OWED. [MAP-31 evidence](../backend/evidence/maps/MAP-31.md).

- MAP-30 local disposition: EXTERNAL_PENDING, no adapter delivered. AndroidX
  Car App Library would be Apache-2.0 but no exact revision is admitted; no
  closed map/Google-location SDK or external-intent substitute was added. No
  authorized DHU/head unit/distribution surface was available, so host
  lifecycle/start-stop/reconnect cannot be claimed; the phone remains fully
  usable without a car host. iOS/CarPlay stays archived.
  [MAP-30 evidence](../backend/evidence/maps/MAP-30.md).

- MAP-27–29 local batch: LOCAL_DONE / INTEGRATION_PENDING. Phase commits `805353b8`
  (MAP-27), `ae0699c5` (MAP-28) and `1a6a1add` (MAP-29) on `codex/map-ev-27-29`;
  no PR/remote CI/merge/wiki. Evidence/docs revisions after those commits do not change code.
- MAP-29 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING, **PERFORMANCE_GATE
  PARTIAL**. Versioned local proxy matrix and raw sanitized samples:
  release universal APK 78,396,538 bytes (74.76 MiB, 16 KB LOAD alignment and
  zipalign PASS), engine init 1149/752/754 ms, real 1.8 km route 75–410 ms,
  PSS 349–351 MiB, cancel 0 ms with no stale callback, cold start
  1218/1384/1242 ms on the owned API35/16 KB translated ARM emulator. MAP-27/28
  real-engine/UI/accessibility tests and targeted domain/application/data/app
  suites plus lint PASS. A physical 4 KB arm64 phone run added init 354–439 ms,
  route 46–125 ms, PSS 275,743–277,633 KiB and cold start 1125–1250 ms (not a
  16 KB proof). Frame pacing/content time, current-head network bytes,
  PostGIS load/EXPLAIN, API26, clean ARMv7 runtime, physical battery and
  national capacity remain OWED; no PASS/national/SLA claim.
  [MAP-29 evidence](../backend/evidence/maps/MAP-29.md). No PR/merge/deploy.

- MAP-28 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING. EV section in the
  existing native route sheet (vehicle/SOC/reserve/consumption/target/PHEV
  share, comma/dot parsing, typed unknowns), primary/alternative/arrival/cost/
  provenance, failure replan from the failed charger arrival SOC with stale-result
  invalidation, MAP-25 save and Saved→Map reopen via private `map/plan/{tripId}`.
  Real native/app UI flow with synthetic Room/vehicle fixtures, saved-plan
  reopen, 200% font/320 dp semantics, ViewModel/reopen tests and lint PASS.
  Only one emulator/profile: two-account/two-device backup, manual TalkBack and
  airplane-mode process-death journey remain OWED; no live/charger guarantee.
  [MAP-28 evidence](../backend/evidence/maps/MAP-28.md). No PR/merge/deploy.

- MAP-27 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING. Typed charging
  candidates, real native road legs, bounded corridor/Room lookup, reserve and
  target exactness, site/equipment/connector failure scopes, duplicate-site
  refusal, reachable fallback from the failed-primary arrival SOC (no failed
  charge), hours/timezone/tariff/power/access/provenance gaps, budget/
  cancellation/generation limits. Independent failure vector 60,000 Wh/30%/
  10%/180 Wh/km gives 900 bp at 20 km (below reserve) and 1,200 bp at 10 km
  (reachable). Domain 9/9, application 10/10, router 3/3, mapper 3/3 and a real
  native route/primary/alternative/replan instrumentation proof PASS.
  Synthetic charging fixtures only; no live/national coverage or arrival
  guarantee. [MAP-27 evidence](../backend/evidence/maps/MAP-27.md).
  No PR/merge/deploy.

- MAP-26 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING. Pure Kotlin EV
  energy model (usable energy, PHEV electric-portion scope, segments with an
  explicit margin, reach, arrival SOC, charging-time lower bound, sum-first
  calibration), conservative rounding and Known/Unknown results. 53 vectors
  from an independent Python/Fractions oracle plus monotonic/property tests and
  mutation checks PASS. No backend/UI/device change; no feasibility claim.
  [MAP-26 evidence](../backend/evidence/maps/MAP-26.md). MAP-27–33 excluded;
  no PR/merge/deploy.

- MAP-25 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING (cloud backup cases
  not delivered). Owner-scoped private favorites and saved route plans (Room v28),
  strict KML/GPX/KMZ exchange (XXE/bomb/traversal/limit refusal, all-or-nothing,
  idempotent import, tombstones), Saved screen and an explicit recorder service.
  Real Room 9/9 + migration + exchange 2/2, UI 3/3 and recorder service on the
  owned 16 KB emulator; two defects found and fixed (lost discard on service
  teardown; MapScreen tests broken since MAP-23). Signed backup/restore needs the
  Android favorites sync client and is NOT claimed.
  [MAP-25 evidence](../backend/evidence/maps/MAP-25.md). MAP-26 next.

- MAP-24 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING. Wi-Fi-aware map
  downloads, safe inactive-generation deletion, navigation generation lease and a
  real airplane-mode clean-restart journey (real MWM, map render, native search,
  route preview, guidance, missing-region refusal) with proof-server request
  count 9 -> 9 PASS on the owned 16 KB emulator; offline-distribution suite
  stabilised (HTTP/1.0 pooled-socket race, off-by-one request count) and extended
  with an interrupted-update resume test. OFFLINE_GATE PASS for this regional
  journey only; one unexplained first-run process crash is disclosed.
  [MAP-24 evidence](../backend/evidence/maps/MAP-24.md). MAP-25 next; MAP-27–33
  excluded; no PR/merge/deploy.

- MAP-23 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING. Explicit foreground
  guidance with native follow/progress, private owner-bound recovery, local TTS
  only and real guidance UI PASS on the owned phone `VKCQGAUCGYAMR4W4`
  (Android 16/arm64, 4 KB). Two-ABI explicit-guidance revision
  `7ad828883c4aca8a` audited and vendored; JVM/lint and release 74.58 MiB PASS
  at `31b31763`. Current-head revalidation: GuidanceInstrumentedTest 3/3,
  GuidanceServiceInstrumentedTest 1/1, GuidanceEngineInstrumentedTest 1/1 with
  a fresh 1 Hz off-route replay reporting `RouteRebuilding` and
  `rerouteRecoverable=true` after >60 s. Field wrong-turn timing, 16 KB phone
  runtime, two-account/device and rendered TalkBack remain OWED.
  [MAP-23 evidence](../backend/evidence/maps/MAP-23.md). New protected PR pending.
  MAP-24–33 excluded.

- MAP-22 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING. Native offline
  search/car route/stops/options/preview, one-way detour, missing-neighbor refusal,
  cancellation and real Map-tab rendering PASS on owned 16 KB ARM64 emulator.
  Two-ABI privacy revision `9cdd3cda00e1025a` removes SDK GPS/route log payloads;
  source/resource/license audit and affected JVM/lint checks PASS. Posterior ARMv7
  bridge proof recorded in the evidence. [MAP-22 evidence](../backend/evidence/maps/MAP-22.md).
  Next was MAP-23 explicit foreground guidance/local voice, now done above.

- MAP-21 local checkpoint: LOCAL_DONE / INTEGRATION_PENDING. Dated operational
  events, exact replay receipts, owner-fenced FIFO/outbox, connected replay and
  startup recovery implemented. Real PostGIS/race + Go HTTP/Kotlin wire proof,
  four owned 16 KB emulator worker tests and Android/lint/static checks PASS.
  [MAP-21 evidence](../backend/evidence/maps/MAP-21.md). Next: MAP-22 native
  planning/stops; MAP-23 explicit guidance follows. No new PR/rollout yet.

- MAP-21–23 selected continuation 2026-10-25: IN_PROGRESS, new protected PR
  pending. Owner merged PR 150 at `eee1b33f`; maintained remote dev matches
  main, including the CI worker-serialization correction `e848bf26`. This
  isolated clone inherited OpenCode minimum gate `b45a0a2c` and retained main's
  atomic Official retry/terminal-verdict contract. Focused real PostGIS and
  Rust suites and engine/data/app JVM suites PASS for that reconciliation.
  [Contract](../product/MAP_21_23_CONTRACT.md). Implement only MAP-21 community
  operational events, MAP-22 native offline routes/stops and MAP-23 explicit
  foreground guidance/voice. MAP-24–33 excluded. No new staging data/source
  admission, provider calls or release certification inferred. Previous PR 150
  notes below are historical; its source integration is complete.

- Cumulative PR #150 integration 2026-10-25: IN_PROGRESS / CI_PENDING.
  User authorized publishing the partial map, PAY and enterprise/static portal
  source union and merging protected dev -> main after current checks pass.
  Isolated clone `/tmp/abastevo-pr150-integration-20261025`; occupied dev and
  primary dirty checkout remain untouched. Includes PAY `1948035c`, MAP
  `e9af87dd`, ESP/FQ-PORTAL-READY `d74334dc`, normal merge commits `df9612af`
  and `0284128f`, and tested integration repairs `4b029756`.
  [Cumulative evidence](../backend/evidence/PR150_CUMULATIVE_INTEGRATION.md).
  Dedicated payment/export/refresh slots preserved; exact admitted native AAR
  vendored with SHA/source access/notices/SBOM for fresh CI; map paths select
  full guards. Worker/config race tests, HTTP size repeat 20, Python provisioning
  (13 cases) and classifier checks PASS. Android full test/lint/release PASS;
  131-migration disposable PostGIS union plus affected-failure reruns PASS at
  source `2707c218`. Three cumulative test assumptions and app SDK path fixed;
  no domain/security gate relaxed. Remote exact-head CI still required.
  Map minimum gate remains PARTIAL; PAY-21 provider proof stays PENDING; portal
  rights/precision/public-release obligations remain. User authorized the known
  `https://teste.abastevo.com.br` staging origin and scoped VPS update after
  integration. Next: finish acceptance, current CI/review checks, guarded merge,
  verify ancestry, retain dev, then staging rollback/smoke. Earlier entries below
  are historical checkpoints and do not override this integration scope.

- Stripe/Asaas payment PAY-22 2026-10-25: INTEGRATION_PENDING.
  `codex/payments-stripe-asaas` fast-forward merged into `dev` in the
  isolated `pay22-dev` worktree (no conflicts, no foreign code; the
  occupied local `dev` branch at `5c9f5204` untouched, all work in
  detached worktrees plus a separate `pay22-full` clone for the helper).
  Pushed `721582b8..6e5e8518` without force; cumulative protected PR
  `AlexandreZanata/Abastevo#150` (dev → main, ready) carries 56 commits
  (maps, Android Premium behavior/UI, PAY-00–21). Candidate corrections
  after review: Pix concurrent root cause (lenient order double vs
  durable timestamps) fixed with deterministic rendezvous regression
  plus PostGIS proof (`dbff7917`, repro 50/50 green); Stripe 401 mapping
  and PAY-21 harness rework around real/operator resources (`c9ea4aa4`,
  `8313283b`, PENDING kept); worker dispatcher maps completed
  (`d149e092`, remainder byte-identical to pristine dev). Android
  Premium suites re-run on the candidate: 20 tests, 0 failures.
  [PAY-22 evidence](../backend/evidence/payments/PAY-22.md). Follow-up
  corrections: provider credential failures reclassified as recoverable
  (`ErrProviderUnauthorized`, no conflict marking, single-convergence
  proof on PostGIS with real adapters) and Stripe SDK raw log emission
  silenced with capture-test proof. Worker integration fully green
  locally after the README-prescribed `chmod 755` on pinned preparers
  (earlier failures reproduced identically on pristine dev and PR-base
  main and root-caused to binary permissions, not code; no quality
  code touched). CI `test` failure root-caused to the missing Room v25
  export (parallel debug/release KSP raced creating it); the
  machine-generated `25.json` is committed (`dcc7c143`), full
  `./gradlew test` passes locally, intermediates 19–24 stay owed by the
  owning front. Remote CI on the corrected head
  pending; merge awaits green CI via guarded finish. No other work
  branch touched.
  Next: push corrections, refresh PR #150, guarded merge on green CI.

- Stripe/Asaas payment PAY-21 2026-10-25: PENDING on
  `codex/payments-stripe-asaas`, parent PAY-20 `45e90604`. No sandbox
  credentials were supplied, so per the phase contract no provider API
  was touched and no sandbox row closes. Landed: gated sandbox harness
  (`-tags=sandbox`, skips without creds, refuses live keys, logs counts
  only), no-leak guard, and sandbox-relevant fault fixtures (retry
  windows, decline terminality, revocation, overdue, reversal closure).
  [PAY-21 evidence](../backend/evidence/payments/PAY-21.md): fixtures,
  harness gates and full backend/static/secret suites PASS; real journeys
  enumerated and awaiting credentials. Next: supply credentials and run
  the harness; PAY-22 only after authorization.

- Stripe/Asaas payment PAY-20 2026-10-25: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-19 `4ef85fe3`. Secure operations
  packaging: owner proofs verify one live session per action, switch records
  bind the old source's merchant, portal configuration admitted optionally,
  maintenance-off darkens a channel with money and reads preserved; staging
  example plus operations runbook, no migration, no new route, no live
  activation. [PAY-20 evidence](../backend/evidence/payments/PAY-20.md):
  config/switch/composition/kill-switch/proof suites plus real PostGIS
  journeys PASS (`-race`), full backend/static/secret suites green (one
  pre-existing Pix 6-racer timing flake, green alone and on re-run).
  Operator proofs, catalog admission and sandbox proof stay gated.
  Next: PAY-21 only after maintainer authorization.

- Stripe/Asaas payment PAY-19 2026-10-25: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-18 `3d529a8c`. Real API/worker
  runtime composed from validated config: gated provider clients, webhook
  intake only, single-owner inbox drain on an isolated `payment-inbox`
  worker slot, settlement/lifecycle/dispute/cycle processing, billing exits
  and refunds — sales impossible (empty production catalog, flags off),
  proofs/switch-merchant/portal fail closed by construction, no Play/cookie
  surface. [PAY-19 evidence](../backend/evidence/payments/PAY-19.md):
  callback→drain→grant/revoke/exit journeys, restart/outage convergence and
  fail-closed composition suites PASS on real PostGIS (`-race`), plus full
  backend/static/secret suites. Worker quality-native cases need the pinned
  Rust preparers (environmental, unrelated). Threat matrix AUTH-01/AUTH-03/
  BND-04/BND-05/BND-07 now CLOSED_LOCAL. No provider proof/sale/deployment.
  Next: PAY-20 only after maintainer authorization.

- Stripe/Asaas payment PAY-18 2026-10-25: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-17 `24a0c0dc`. Tests only, no
  production change: webhook refusal/intake redaction (no PAN/CVV/secret/QR/
  URL/token echo or retention, `no-store` everywhere), rotated operator-proof
  refusal on request/replay/reconcile, restore/stale-replay convergence with
  no financial effect, outage-pending recovery with exactly one provider
  write, and frozen bounded intake ceilings with indexed hot-query plans.
  [PAY-18 evidence](../backend/evidence/payments/PAY-18.md): privacy unit
  suites, race PostGIS acceptance, measured 200-callback intake (total
  174.76ms, mean 873.80µs, p95 1.12ms) and full backend/static/secret suites
  PASS. First full unit run hit one unrelated pre-existing keyprover flake
  (passes alone and on re-run). Threat matrix AUTH-05/WHK-04/WHK-07/REC-02/
  REC-04/REC-05/BND-01/BND-02 now CLOSED_LOCAL. No worker/route/provider
  proof/sale. Next: PAY-19 only after maintainer authorization.

- Stripe/Asaas payment PAY-17 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-16 `72725283`. Migration 000130
  permits capability-free orders/grants for the enterprise family only; premium
  invariants (non-empty capabilities in catalog, binding and settlement) are
  untouched. Added `DecideEnterpriseBinding` (single payer = billed entity =
  service target, quantity one, no app capabilities, explicit approval flag) and
  a runtime gate on card/Pix-manual/Pix-auto starts. Billing history now exposes
  the provider invoice/receipt reference, owner-scoped. [PAY-17 evidence](../backend/evidence/payments/PAY-17.md):
  domain binding matrix, cross-channel refusal without order/provider side
  effects, approved self-purchase end-to-end (checkout + Pix Automático mandate),
  product-confusion binding refusals, no-capability grant with denied premium
  access, and history scoping/proof suites PASS. No worker/route/provider
  proof/sale; no fiscal engine. Next: PAY-18 only after maintainer authorization.

- Stripe/Asaas payment PAY-16 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-15 `5a1a96e6`. Migration 000129 adds
  refund/dispute/hold/audit tables plus settlement charge reference. Refunds run
  only on the original route under entitlement locks, with fresh operator proof,
  reason/audit and amount-dependent dual approval; disputes revoke only matched
  unexpired grants; unmatched money becomes an explicit reviewable hold.
  [PAY-16 evidence](../backend/evidence/payments/PAY-16.md): adapter contract and
  PostGIS concurrency/lost-reply/insufficient-balance/duplicate/reversed/no-payment-
  hold suites PASS. No worker/route/provider proof/sale. Next: PAY-17 only after
  maintainer authorization.

- Stripe/Asaas payment PAY-15 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-14 `a9b2903f`. Migration 000128 adds
  durable billing-exit intents and per-source cancellation records. Owner
  cancel/status/history/export use fresh proof for sensitive operations; account
  deletion records the exit before mutation, retains minimal financial rows,
  never refunds and keeps uncertain provider cancellation visible. The API
  composition records the intent; provider execution/reconciliation remains
  PAY-19. [PAY-15 evidence](../backend/evidence/payments/PAY-15.md): account/
  PostGIS exit, outage, stale-replay, proof and retention suites PASS. No
  worker/route/provider proof/sale. Next: PAY-16 only after maintainer authorization.

- Stripe/Asaas payment PAY-14 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-13 `5ba72b33`. Migration 000127 adds
  durable switch intents and leases. Stripe card renewal uses confirmed
  `cancel_at_period_end`; Pix Automático uses the durable mandate cancellation
  path. New recurring starts are gated by family, paid-through boundary and a
  matching READY target quote; unresolved old cancellation blocks activation.
  Next-term same-family plan/price changes are supported as source replacement;
  proration/immediate changes are refused. [PAY-14 evidence](../backend/evidence/payments/PAY-14.md):
  two-provider/concurrent/unknown-cancel/stale-quote/excess PostGIS and full
  race/Play suites PASS. No worker/route/provider proof/sale. Next: PAY-15 only
  after maintainer authorization.

- Stripe/Asaas payment PAY-13 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-12 `e60dcf60`. Migration 000126 adds
  provider-owned Pix Automático instruction/eligibility tracking, retry policy and
  durable cancel intent. Asaas remains the only cycle owner: each verified
  subscription payment funds one local cycle order/grant, instructions never
  grant, and cancellation/expiry/ineligibility stop future authority without
  erasing paid-through. [PAY-13 evidence](../backend/evidence/payments/PAY-13.md):
  concurrent PostGIS cycle/cancel/eligibility and full race/Play suites PASS. No
  worker/route/provider proof/sale. Next: PAY-14 only after maintainer authorization.

- Stripe/Asaas payment PAY-12 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-11 `65ffd486`. Migration 000125 adds
  Pix Automático mandates with explicit consent audit, bounded authorization,
  provider-owned SUBSCRIPTION mode, refusal/expiry/cancel/reconcile and lost-reply
  recovery. Initial paid period is independent from mandate status; MERCHANT
  eligibility remains an external gate. [PAY-12 evidence](../backend/evidence/payments/PAY-12.md):
  Asaas adapter/PostGIS Pix-Auto and full race/Play suites PASS. No worker/route/
  provider proof/sale. Next: PAY-13 cycles only after maintainer authorization.

- Stripe/Asaas payment PAY-11 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-10 `2529e167`. Migration 000124 adds
  provider subscription contracts separate from invoice grants; Stripe lifecycle
  maps subscription updated/deleted, invoice failed/action-required/paid and
  checkout completion, with last-event stale fencing and reconciliation. Portal
  is owner-bound and requires approved configuration. SCA/past-due/cancel keep only
  paid-through; invoice creation grants nothing. [PAY-11 evidence](../backend/evidence/payments/PAY-11.md):
  adapter/PostGIS lifecycle/portal and full Play regression/race suites PASS. No
  runtime/worker/portal route/sale. Next: PAY-12 Pix Automático only after
  maintainer authorization.

- Stripe/Asaas payment PAY-10 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-09 `8251aa19`. Canonical Stripe invoice
  and Asaas received-payment verifiers feed an idempotent settlement processor;
  settlement/order/grant/cycle/outbox commit atomically and inbox processing is
  durable. Revocation/replay, delayed/duplicate events, provider outage recovery,
  wrong amount/currency/pending/manual invoice and concurrent receipt are tested.
  Subscription capability/status now honors active web grants without Play
  credentials; Play/free access regressions pass. [PAY-10 evidence](../backend/evidence/payments/PAY-10.md):
  race PostGIS/payment/subscription suites, full backend, vet/staticcheck/
  govulncheck/sqlc/security gates PASS. No runtime/worker/sale. Next: PAY-11
  Stripe recurring lifecycle only after maintainer authorization.

- Stripe/Asaas payment PAY-09 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-08 `bb200890`. Asaas webhook handler
  uses a separate 32–255 char rotating token in constant time, optional TLS,
  1 MiB bound, merchant binding and durable hashed inbox before HTTP 200. Unknown
  events, duplicates/conflicts, token rotation/API-key separation, backlog retry
  and DB outage are tested. No provider call/grant/runtime. [PAY-09 evidence](../backend/evidence/payments/PAY-09.md):
  handler/race/full backend/vet/staticcheck/govulncheck/sqlc/security gates PASS.
  No runtime wiring/sale. Next: PAY-10 settlement/access only after maintainer
  authorization.

- Stripe/Asaas payment PAY-08 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-07 `0af7918e`. Stripe webhook handler
  verifies exact raw body via SDK, 300s tolerance, pinned API version/livemode/
  no-Connect/merchant, bounds body/signature and writes hashed minimal metadata
  to the durable inbox before HTTP 200. Duplicate alteration 409; DB failure 503;
  unknown/rotated-signature cases tested; no provider call or grant. [PAY-08
  evidence](../backend/evidence/payments/PAY-08.md): handler/race/full backend/
  vet/staticcheck/govulncheck/sqlc/security gates PASS. No runtime wiring/sale.
  Next: PAY-09 Asaas webhook inbox only after maintainer authorization.

- Stripe/Asaas payment PAY-07 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-06 `6e52f9bd`. Conventional Asaas Pix
  service/adapter added: owner customer mapping, server price/BRL, due date,
  external-reference correlation, bounded owner QR/status, dispatch-before-call,
  ambiguous recovery and incomplete-search fencing. No auto-debit/settlement/
  grant. [PAY-07 evidence](../backend/evidence/payments/PAY-07.md): app/adapter/
  real-Store race tests, full backend suite, vet/staticcheck/govulncheck/sqlc and
  security/secret gates PASS. No HTTP/runtime/webhook or sale activation. Next:
  PAY-08 Stripe webhook inbox only after maintainer authorization.

- Stripe/Asaas payment PAY-06 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-05 `d519a49b`. Stripe Go v87 hosted
  Checkout use case added: server catalog, deterministic order, owner customer
  mapping, card-only `allowed_payment_method_types`, config return origin, stable
  idempotency, dispatch-before-call and ambiguous recovery via persisted
  `client_reference_id`. Client amount/customer/order/return URL/metadata are
  refused. [PAY-06 evidence](../backend/evidence/payments/PAY-06.md): app/adapter/
  real-Store race integration tests, full backend suite, vet/staticcheck,
  govulncheck, sqlc/security/secret gates PASS. No HTTP/runtime/webhook/grant or
  sale activation. Next: PAY-07 Asaas Pix only after maintainer authorization.

- Stripe/Asaas payment PAY-05 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-04 `b931a1df`. `PaymentsConfig`
  binds Stripe/Asaas test/live modes, merchant/catalog/keys/webhook material and
  return origin; sale flags are independent and default OFF while maintenance
  defaults ON. Redacted `Preflight()`/`logValue()` expose readiness only; missing/
  mixed values disable unsafe channels without crashing, and rotation redaction
  is tested. Staging template uses test placeholders with all sales false.
  [PAY-05 evidence](../backend/evidence/payments/PAY-05.md): config race, full
  backend unit suite, vet/staticcheck, security/infra/secret gates PASS. No
  provider/runtime/sale. Next: PAY-06 Stripe checkout only after maintainer
  authorization.

- Stripe/Asaas payment PAY-04 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-03 `22feac69`. Migration
  `000123_payment_intents` (MAP owns 122) and `db/queries/payments` persist
  customers/orders/operations/attempts/settlements/grants/cycles/inbox; outbox
  reuses transactional `job_queue`. Store fences claims by unique key, payload
  hash, lease and generation; webhook-before-reply, unknown outcomes and stale
  workers are tested; concurrent claims/settlements produce one grant. [PAY-04
  evidence](../backend/evidence/payments/PAY-04.md): 10 PostGIS integration tests,
  race, migration upgrade/replay, forced rollback, vet/staticcheck/sqlc/diff/secret
  PASS. No provider/HTTP/runtime/sale. Next: PAY-05 config/activation only after
  maintainer authorization.

- Stripe/Asaas payment PAY-03 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-02 `efe5f170`. New pure
  `backend/internal/modules/payments/domain` implements order/attempt/settlement/
  contract/mandate/refund/dispute/grant transitions, exact BRL periods, one order
  one grant, early/late renewal, zero grace, bounded refunds/disputes, excess and
  one recurring source. Redirect/authorization/manual/unknown evidence cannot
  grant. Play/free exits unchanged. [PAY-03 evidence](../backend/evidence/payments/PAY-03.md):
  race tests, existing subscription suites, vet, staticcheck, 26 golden cases,
  73.8% domain coverage PASS. No DB/HTTP/provider/runtime/sale. Next: PAY-04
  persistence/concurrency only after maintainer authorization.

- Stripe/Asaas payment PAY-02 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-01 `7bfa2fd4`.
  [Provider admission v1](../planning/STRIPE_ASAAS_PROVIDER_ADMISSION.md) pins
  Stripe Go `v87.0.0`/API `2026-09-30.endive` (MIT, Go 1.24+, hosted Checkout +
  SDK raw-body webhooks) and an Asaas v3 `net/http` client (no vendor Go SDK;
  `access_token` API header, `asaas-access-token` webhook header). Synthetic
  [Stripe](../../contracts/testdata/payments/stripe-contracts-v1.json) and
  [Asaas](../../contracts/testdata/payments/asaas-contracts-v1.json) fixtures cover
  endpoints, events, status maps, idempotency limits, Pix QR and provider-owned
  Pix Automático (`SUBSCRIPTION`, retry `NOT_ALLOWED`). No go.mod/go.sum,
  provider/runtime, credential or sale change; SDK installation remains PAY-06
  and account/sandbox proof PAY-21. Next: PAY-03 pure billing/access rules only
  after maintainer authorization.

- Stripe/Asaas payment PAY-01 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/payments-stripe-asaas`, parent PAY-00 `5f66c0d1`. [Offer/lifecycle policy v1](../product/STRIPE_ASAAS_OFFER_POLICY.md)
  freezes `premium.monthly` (one account, P1M anchor, zero grace, one future
  period, exact amount), refund/dispute/excess/switching rules and the
  provider-owned Pix Automático scheduler. Production prices remain UNRESOLVED
  (`1234` centavos is synthetic only); Enterprise is non-sellable and all three
  channels stay OFF. [26-case golden fixture](../../contracts/testdata/payments/offer-policy-v1.json)
  and [PAY-01 evidence](../backend/evidence/payments/PAY-01.md): period/access/
  refund/dispute/excess/switch/sale recomputation, diff, links and secret scan
  PASS. Additive shared edit to `PREMIUM_PRODUCT_CONTRACT.md`; no provider/
  runtime/migration or sale activation. Next: PAY-02 provider/SDK admission only
  after maintainer authorization.

- Stripe/Asaas payment PAY-00 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  exclusive `codex/payments-stripe-asaas`, base fetched `origin/dev` `721582b8`.
  Payment planning from `ee8d316b` replayed onto the reconciled head (five docs
  unchanged; scoped ROADMAP/README/PREMIUM/PROGRESS additions re-applied); original
  planning worktree preserved. [Baseline/ownership evidence](../backend/evidence/payments/PAY-00.md)
  and v1 [threat/test matrix](../backend/evidence/payments/PAY_THREAT_MATRIX.md)
  record source `721582b8` versus deployed test source `53719336`/schema 120, MAP
  (`606f3b08`), ESP (`46fee9e0`) and integrated PRM owners. PAY-00 edits are
  docs/evidence only: no provider/catalog/credential/runtime/migration change, no
  charge/refund/mandate, no PR/CI/merge/deploy/wiki and no occupied worktree moved.
  Checks: worktree/ref ancestry, diff check, scoped links/registry/DAG and secret
  scan. Next: PAY-01 offers/lifecycle policy only after maintainer authorization.

- Enterprise portal second-review corrections 2026-10-25: LOCAL_DONE / INTEGRATION_PENDING on `codex/enterprise-static-portal`; validated code head `7fd5c414` (this evidence commit follows). [Second-review evidence](../backend/evidence/enterprise-static/ESP-CORRECTIONS-02.md).
  The earlier follow-up entry below was too broad: the four additional RED scenarios were real and are now closed with maintained regressions, not only external decisions.
  (1) Official publication is serialized per survey window (advisory lock even without a pointer row + revision row lock): concurrent first publications/corrections publish once, stale finishes retire `superseded`, duplicate finishes converge, Stage is frozen on every terminal status, divergent restage is refused, run/revision/pointer/counts are checked, persisted windows are validated on resume, and cancellation/transaction rollback leave no partial state. Migration 000131 retained and status compatibility asserted.
  (2) Operational pause (`operator-pause`) is distinct from invalid-content revocation (`revoked-source`): the same eligible source/bytes recover through Promote/Refresh in full and official-only modes, including restart and failure preservation, while revoked identity stays refused by any hash.
  (3) The current release is mandatory in the retained store beyond `keep`, budgets fail before the pointer moves, nested/safe asset paths and unique-byte accounting are enforced, and promote/rollback/refresh/withdraw prepare assets automatically.
  (4) `retained` is an atomic symlink to an immutable generation with completeness marker: failed switches keep the previous store, missing links recover the newest complete generation, legacy directories migrate recoverably, withdrawal purges retired generations, and concurrent HTTP readers survive transitions.
  Commits: `6e34f223`, `aafb492b`, `c958c866`, `3235d9ad`, `471c1f2d`, `7fd5c414`. Verification: real disposable PostGIS `-race -tags=integration` official + enterpriseexport suites PASS; unit/race portalgen+jobs PASS; build/vet/staticcheck/sqlc/gofmt/diff CHECK PASS; local Caddy full and official-only rehearsals PASS (A→B→A, retained asset after rollback, corrupt rollback, pause/recover, withdraw fallback/404).
  Scope remains LOCAL_ONLY: no VPS/staging data/third-party/push/PR/merge/wiki/publication, and no precision/rights/public-scope approval. Branch is based on `bf32027d`; `dev` has three later Android-only commits with no ESP overlap and was not blindly merged. Remaining: integration/remote checks, authorized staging drill, precision policy, rights/clarification/public scope and G09 certification.
- Enterprise portal follow-up corrections 2026-10-25: LOCAL_ONLY / NO_PUBLIC_EXECUTION on `codex/enterprise-static-portal` (49 ahead / 43 behind origin/dev at fetch; snapshot only, nothing incorporated).
  All three follow-up RED reproduced via overlay, then fixed with maintained regressions (no assertions weakened): (1) automatic withdrawal classifies reason from source state — only `Status=withdrawn` revokes (`ErrOfficialRevoked` by any hash), outage pauses; full + official-only + restart + recovery covered.
  (2) atomic import begin (single tx + advisory lock): identical retries converge, orphans adopted with supersession from persisted state, cancel persists nothing, stale finish retires as `superseded` (new append-only migration 000131, coordinated past dev's 000122-130) instead of repointing; race/concurrency green on real PostGIS.
  (3) official insights reconcile every state (wrong_branch/succession_review grouped as unlinked with explanation; inconsistent census refused).
  (4) real cross-release asset retention: hardlinked `retained/` store with keep/byte budgets, withdraw purge, `/assets/*` edge mapping; Caddy rehearsals green (retained-200 after rollback, no mixing, purged-404 after withdraw); prior 404-rehearsal distinguished from retention acceptance.
  (5) backup/restore proves canonical content digests (rows, raw text, runs, revisions, pointers) from per-execution private dumps.
  (6) S-ESP-03 aligned to indeterminate (menu without selection): table/section now `RIGHTS_PENDING`, S-ESP-04 preserved, gates needed no change (enforcement never admitted).
  Publication scope still EMPTY: no origin/rights/approval invented, no VPS/third-party/PR/merge. Commits: reason classification, atomic begin, insights census, retention, backup proof, rights alignment. Remaining externals: precision policy, clarification answers, public scope, authorized staging A→B→A drill.
  Historical note: the "all fixed" scope of this entry is corrected by the second-review entry above; the four uncovered publication/freeze/pause/asset scenarios were real and are now closed.
- Enterprise portal Prompt-2 delivery prep 2026-10-25: LOCAL_ONLY / NO_PUBLIC_EXECUTION on `codex/enterprise-static-portal` (42 ahead / 36 behind origin/dev at fetch; counts are a snapshot, nothing incorporated).
  Publication scope fields are EMPTY, so no origin/rights/approval was invented and no public step ran; STAGING-01 is not claimed as the full portal.
  [ESP-07 addendum](../backend/evidence/enterprise-static/ESP-07.md): registered edition re-fetched byte-identical and re-imported (19,609 staged / 880 over-precision / needs-review); all 880 are IEEE-754 storage noise collapsing to 46 exact 2-decimal values (0 genuine decimals) — no parser bug, no rounding implemented; exact float-noise admission policy proposed with golden vectors, owner decision pending; real-data replay/outage/backup-restore/export-noop proven (`TestManualRealWeeklyRecovery`); retry-path defect fixed (identical retries crashed on run identity → now converge/resume, `TestFailedBytesConvergeWithoutNewRevision`, `TestRunningAttemptResumesSameRevision`, `TestConcurrentIdenticalBeginConverges`, race green).
  [Rights re-examination](../product/ENTERPRISE_SOURCE_ADMISSION.md): current primaries re-fetched (registry PDF unchanged + OCR without license; prices metadata URL 404s with no dataset license; enforcement DOCX unchanged menu without selection; fines CC BY re-verified; PMQC DOCX unchanged without license) — no status change, clarification draft recorded unsent.
  Functional: admitted official insights added to curiosidades (counts/formulas, no guilt verdicts); association-change rebuild proven; historical static navigation stays pending (scope not approved); PMQC/FQ-06 not blockers nor done.
  Pipeline: full+official-only Caddy rehearsals green (A→B→A, withdraw→404, resurrection refused, asset-retention probe); target capacity campaign OWED with predeclared limits/stops, never against ANP.
  Integration: origin/dev tip is active payments work (migrations 000123–130 our branch lacks) with payments/MAP branches ahead in other worktrees — NO blind merge, NO PR without delivery authorization; zero file overlap in ESP areas; primary checkout and other worktrees untouched.
  Commits: retry-convergence fix, rights addendum, insights/rehearsal. Limits: real revision switch OWED behind precision policy; public origin/rights/approval/window/CDN all missing. Next: owner decisions (precision policy, clarification answers, public scope), then final candidate + authorized staging drill.
- Enterprise portal review corrections 2026-10-25: IN_PROGRESS / LOCAL_ONLY on `codex/enterprise-static-portal`.
  Three reproduced defects, no VPS/publication/merge: (1) withdrawn releases could be re-promoted — fixed with pre-copy refusal preserving the mark plus revoked-source identity (`ErrReleaseWithdrawn`/`ErrOfficialRevoked`); the STAGING-01 drill exercised the defect, corrected by addendum without deleting history; staging A → B → A drill OWED.
  (2) preflight/smoke are mode-aware (official-only home `/fiscalizacao/`, manifest cross-check) with required dedicated|shared topology and generator→preflight integration tests; approval stays external.
  (3) official-only verification enforces an exact file allowlist (export CSV/JSONL and unexpected roots refused even when manifested).
  (4) maintained `cmd/portalexport` replaces the uncommitted helper (read-only REPEATABLE READ over ExportPin/BuildOfficialExport; real-PostGIS no-op/second-version/refusal green); `Refresh`/config/worker propagate `OfficialOnly` (official-only withdraw goes 404, never full fallback); serve/current pointer contract in the runbook.
  (5) obligation ledger corrected: ESP-07 real prices BLOCKED_OVER_PRECISION, ESP-12 PARTIAL, ESP-11 revalidation OWED, ESP-16 public capacity OWED, ESP-18 public BLOCKED; staging noindex != private (anonymous GET documented). Next: full local validation, atomic commits.
- Enterprise portal ESP-STAGING-01 2026-10-25: STAGING_DONE / NOT_PUBLIC on `codex/enterprise-static-portal` after ESP-18 `f1591b82`.
  [Evidence](../backend/evidence/enterprise-static/ESP-STAGING-01.md). User-authorized scope: real FQ-07 2016–2025 via staging API plus official-only `/fiscalizacao` on `https://teste.abastevo.com.br`; no synthetics.
  Implemented `portalgen`/`portalcheck`/`portalrelease --official-only` (admitted slice only, delinked station hrefs, mode-mismatch refusal); `enterpriseexport` suites green.
  Staging: pre-ingest backup verified; 7/7 sanction discovery+import jobs done, seven VERIFIED revisions, current 2025 lease (3,375 rows); candidate `460d8b9f…` (3,394 pages) promoted; additive noindex edge locations; smoke PASS (manifest hash, no price links, API/health intact); withdraw→404→repromote→200 rollback drill PASS.
  Linked records show association state without station pages; price/registry/PMQC rights still pending; ESP-18 public still blocked; no merge/pilot/G09 claim. Next: second official-only release replay or rights-gated full portal decision.
- Enterprise portal ESP-18 2026-10-25: BLOCKED_EXTERNAL_AUTHORIZATION /
  NOT_PUBLISHED on `codex/enterprise-static-portal` after ESP-17 `103dcf6b`.
  [Evidence](../backend/evidence/enterprise-static/ESP-18.md).
  [Runbook](../operator/PORTAL_PUBLICATION.md). Implemented the local public
  preflight, read-only public smoke and publication/rollback runbook;
  `make check-publication-gate` PASS. No public release was executed: no named
  public origin/host/root/CDN/TLS/ownership/approval/window; candidate B
  (`914d8b9f…`) is internally valid at the `https://portal.example` placeholder
  but its sitemap/robots use that origin and it contains price/station/download
  pages whose S-ESP-01/02/05 public rights are `RIGHTS_PENDING` (only official
  S-ESP-04 is admitted; B's official census is 3,375 unmatched). Preflight
  correctly returns FAILED_LOCAL_GATE; public smoke self-tests PASS but was not
  run. No VPS/DNS/CDN/shared edge/API/DB/ANP mutation, no app/API/G09
  certification, no merge/publication. Next: owner approval/rights and an
  official-only candidate rebuilt at the approved origin.
- Enterprise portal ESP-17 2026-10-25: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/enterprise-static-portal` after ESP-16 `a6c3758e`.
  [Evidence](../backend/evidence/enterprise-static/ESP-17.md). Found and fixed
  missing shared CSS/search script on `metodologia`/`404` (raw `pageBase` →
  `g.base`), with regression tests. Added isolated HTTPS preview (Caddy internal
  CA, localhost:18443, HTTP 308, HSTS/cache/noindex, no API), preview probe
  (metadata/landmarks/labels/links, JS-disabled state/station navigation,
  keyboard skip/focus order, 200% mobile reflow, screenshots) and release-root
  backup/restore rehearsal. New A/B artifacts `7c37f485…`/`914d8b9f…`; preview
  A→B→A, 0 probe failures/warnings, backup SHA `a99a3ee9…`. No public
  TLS/DNS/CDN, shared preview edge, real device/screen reader, cross-release
  asset retention or production claim; real prices remain OWED. No
  VPS/publication/merge. Next: ESP-18.
- Enterprise portal ESP-16 2026-10-25: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/enterprise-static-portal` after ESP-15 `71e4f92b`.
  [Evidence](../backend/evidence/enterprise-static/ESP-16.md). Added a frozen
  local capacity/browser harness: pinned Caddy container, separate pinned
  Python generator container, gzip open-loop driver, Caddy CPU/mem/net/block
  sampling, headless-Chrome FCP/LCP/CLS/same-origin/API probe and fail-closed
  report; `make check-portal-capacity` + `make portal-capacity`. Baseline
  real-official 2025 full artifact (`75bc8a1a…`): 3,000/3,000 requests,
  100.03 req/s, p95 1.47 ms, 0 errors/drops, 6.46% CPU, 26.64 MiB Caddy,
  browser FCP/LCP 16–184 ms, CLS 0. Concurrent switch A→B and rollback under
  load: 100.03 req/s, p95 1.25 ms, 0 errors, final current restored; browser
  FCP/LCP 20–108 ms. No CDN/VPS-direct campaign; real prices, old-asset
  cross-release retention and production capacity remain OWED. No
  VPS/publication/merge. Next: ESP-17.
- Enterprise portal ESP-15 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/enterprise-static-portal` after ESP-14 `43574f7a`.
  [Evidence](../backend/evidence/enterprise-static/ESP-15.md). Added default-off
  automatic refresh: fine/official publication → export build (`ObservedAt`) →
  durable `enterprise-portal-refresh` → generate/verify/promote with current
  fence; 15-minute recovery schedule, dedicated dispatcher, no-op/restart/
  orphan recovery, failed-build preservation, private receipt and withdrawal of
  a release whose FQ revision is no longer published. Real 2025 official slice
  (3,375 all-unmatched records) over the synthetic price fixture: 3,401 pages/
  3,411 files/15,881,532 bytes, content `ccd42669…`; correction created a second
  release, withdrawal removed the current official release. Unit/race,
  PostGIS integration, real refresh and fine after-publish hook tests PASS;
  build/vet/staticcheck/sqlc/gofmt/diff PASS. No production activation; real
  prices, multi-edition FQ, live fine revalidation and prod hosting remain OWED.
  No VPS/publication/merge. Next: ESP-16.
- Enterprise portal ESP-14 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/enterprise-static-portal` after ESP-13 `06e871e9`.
  [Evidence](../backend/evidence/enterprise-static/ESP-14.md). Added an atomic
  private portal publisher (`portalrelease`: flock/fence, complete copy +
  second verify, content-addressed releases, atomic `current` symlink,
  rollback/withdraw/stale-state reconcile), a pinned loopback Caddy rehearsal
  with security/immutable/noindex headers, gzip and a real 404, and the
  isolated `rehearse-portal.sh` script. Real ESP-13 A/B artifacts: promote,
  switch, rollback, corrupt-current recovery, withdraw-to-fallback and
  withdraw-to-404 all PASS; headers/compression/API-independence checked.
  Unit/race and integration enterpriseexport PASS; build/vet/staticcheck/sqlc/
  gofmt/diff PASS. Production host/CDN/TLS/DNS/shared edge, multi-host locking,
  real prices and auto-refresh remain OWED. No VPS/publication/merge. Next:
  ESP-15.
- Enterprise portal ESP-13 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  `codex/enterprise-static-portal` after ESP-11 `af615284`.
  [Evidence](../backend/evidence/enterprise-static/ESP-13.md). Portal artifacts
  now use `enterprise-portal-v2`: official-pin-bound manifest digest,
  content-addressed CSS/JS, indexable sitemap + robots, and `portalcheck`
  verification of manifest/file digests, public-field/JSON/search/CSV
  allowlists, CPF/private-key/raw-HTML rejection, link/sitemap completeness,
  gzip/page/file/byte budgets and two-build reproducibility. Real 2025 official
  slice (3,375 all-unmatched records) over the synthetic station/price fixture:
  noindex 3,409 pages/3,419 files/15,231,739 bytes; indexable + real search JS
  3,409 pages/3,421 files/15,932,636 bytes, sitemap 3,408 URLs/466,272 bytes;
  reproducible compare PASS; max HTML gzip 10,023 B, shard 376 B. Unit/race,
  integration, real official portal, vet/staticcheck/sqlc/gofmt/diff PASS.
  Real prices remain blocked by ESP-07/rights; multi-edition FQ and live fine
  revalidation remain OWED. No VPS/publication/merge. Next: ESP-14.
- Enterprise data/static portal ESP-11 2026-10-24: LOCAL_DONE (event-level) /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-11.md).
  Merged FQ-PORTAL-READY; real FQ-07 seven-file acceptance 35,911=35,612+293
  CPF+6 metadata, 35,612 unmatched/zero links; 2025 export 3,375 rows.
  `/fiscalizacao/` event-level index/pagination/detail and linked station
  sections; only `linked` shows a station, unmatched/ambiguous never. Real 2025
  portal candidate: 3,409 pages/3,419 files/15.2 MB; synthetic linked/ambiguous
  case covered. Builder provider/state, FQ pin reconcile and no-published gate
  tested. FQ-06, live fine-link revalidation/rebound, multi-edition snapshot and
  real FQ+price same-DB deployment remain OWED; real prices still ESP-07-blocked.
  Next: ESP-13 artifacts.

- Fuel-quality portal readiness 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  local `codex/fq-portal-ready` worktree from `origin/dev 721582b8`; commits
  `424d71b7`, `9795a36e` plus this evidence commit. [Evidence](../backend/evidence/fuel-quality/FQ-PORTAL-READY.md).
  Seven verified FQ-07 fine resources accepted through the real Rust→worker→
  PostGIS→FQ-08 read path: 35,911 input = 35,612 accepted + 293 CPF + 6 metadata;
  35,612 explicit unmatched company decisions, zero links/identity writes in the
  empty Directory. Added a read-only frozen official-record export port
  (`ExportPin`/`ExportSanctions`, revision-sealed keyset cursor) and
  `BuildOfficialExport` JSONL/manifest with payload SHA-256; real exports total
  35,612 rows. Tests: quality integration/race, seven-file `qualitysource`
  PASS, build/vet/staticcheck/sqlc/diff. No FQ-06, portal page, map, VPS,
  production activation or `main` merge claimed. Fine-association live
  revalidation/rebound remains OWED; event-level ESP-11 can consume the frozen
  snapshot without station claims.

- Enterprise data/static portal ESP-12 2026-10-24: LOCAL_DONE / PARTIAL;
  official-record insights were blocked at that checkpoint; ESP-11 event-level
  now unblocks them, but they are not included in this batch.
  [Evidence](../backend/evidence/enterprise-static/ESP-12.md).
  `/curiosidades/` has six approved deterministic questions with formulas,
  sample-change explanation and exclusion census; `/dados/` exports latest
  station/price/summary CSVs plus a hashed manifest, with safe spreadsheet text,
  attribution and no private fields. Fixture 18 pages/28 files; download census
  7/8/8; Go/race, Portalgen 8 tests + 5 subtests, real PostGIS build→portal and
  static checks PASS. No official records, API, source activation or publication.
  Real prices remain blocked by ESP-07/rights. Next: ESP-13 artifacts; ESP-11
  remains the gating phase.

- Enterprise data/static portal ESP-10 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-10.md).
  Accent-insensitive name and numeric/alphanumeric CNPJ search over one immutable
  regional shard per UF (`sem-uf` when absent), 80-char/50-result bounds, city
  filter, abort/fallback for missing or version-mismatched shards and no national
  locator. `search.ts` is emitted by the landing build and embedded by
  `portalgen --search-js`; HTML directory works without JS. Landing suite 60/60;
  Go/portalgen tests, real PostGIS build→portal, typecheck/build/check and
  static checks PASS. Synthetic 6,000-station shard: SP 346,084 B / 23,639 gzip
  vs national 1,020,084 / 70,029. Real-source search remains blocked by ESP-07.
  Next: ESP-11 official-record pages.

- Enterprise data/static portal ESP-09 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-09.md).
  `portalgen` verifies one export candidate (manifest/JSONL/analytics hash) and
  writes deterministic station, price, state/municipality, methodology, gated
  fiscalization, 404, CSS, robots and portal-manifest pages. Exact prices,
  periods, history gaps, status, sample/insufficient states, no JS/API, noindex
  preview, page/byte budgets and local links are tested; real PostGIS
  build→generate PASS. Real price publication remains blocked by ESP-07
  over-precision/rights. Next: ESP-10 static search.

- Enterprise data/static portal ESP-08 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-08.md).
  [Routes/editorial contract](../product/ENTERPRISE_PORTAL_ROUTES_AND_EDITORIAL.md)
  freezes canonical `/postos/{uuid}/`, `/consultar/`, `/precos/`,
  `/metodologia/`, gated `/fiscalizacao/`, field allowlists, empty/stale/error
  states, sample/period wording, attribution and no-JS accessibility. Labelled
  synthetic prototype under `landing/prototype/` with active/suspended, missing
  week, insufficient sample, derived summary and gated fiscalization. Landing
  prototype tests 9/9, full suite 54/54, typecheck/build/check PASS; not in
  `dist/` or sitemap. Next: ESP-09 generated catalog/price/history pages.

- Enterprise data/static portal ESP-07 2026-10-24: LOCAL_DONE /
  SYNTHETIC_SLICE_ACCEPTED; real source `BLOCKED_OVER_PRECISION`.
  [Evidence](../backend/evidence/enterprise-static/ESP-07.md). Synthetic
  Directory/Official/export/analytics/build chain passes unit/race/PostGIS.
  Real weekly `revendas_lpc_2026_10_11-2026_10_17.xlsx` (1,875,300 bytes,
  SHA-256 `e148758e…aaa6d6`) parsed 20,489 rows; import created 7,708 stations
  and staged 19,609 prices, but 880 over-precision rows (4.29%) correctly kept
  the revision `needs-review` (limit 1%). Fixed lost CNPJ leading zeros, dot
  decimal and GLP/DIESEL station labels; no rounding policy invented. No real
  edition published/exported. Next: ESP-08 routes/templates with labelled
  synthetic data; real VPS price publication remains blocked.

- Enterprise data/static portal ESP-06 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-06.md).
  Durable after-publication build intent, private file-lock/state workspace,
  latest-window coalescing, atomic candidate/state/analytics, Directory
  fingerprint and Official pin reconciliation, period/fingerprint dedupe and
  default-off worker schedule. Real PostGIS tests cover correction coalescing,
  no-op, busy lock, cancellation, lost-notification reconcile and hook
  failure/publication retention. Build has a dedicated dispatcher; no migration,
  public route or activation. Next: ESP-07 local data acceptance.

- Enterprise data/static portal ESP-05 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-05.md).
  One regional pricing analyst buyer, one sample-comparison decision and one
  Regional Fuel Price Brief frozen; five-station municipal sample rule,
  Abastevo-derived label, unmeasured costs, `RIGHTS_PENDING` redistribution and
  `DEMAND_UNVALIDATED`. No implementation, customer, price, contract or
  deployment. Next: ESP-06 bounded export orchestration.

- Enterprise data/static portal ESP-04 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-04.md).
  Pure `enterpriseexport/analytics` metrics: per-revision census/coverage,
  national/state/municipality summaries with sample hash, national/state
  cheapest rankings with explicit ties/cutoff/population, station history with
  gaps and summary change only for matching samples. Golden tests cover ties,
  missing units/periods/identity, sample changes, exclusion reconciliation and
  input-order independence. No DB/network/clock dependency, migration, route or
  artifact publication. Next: ESP-05 enterprise offering boundary.

- Enterprise data/static portal ESP-03 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-03.md).
  Directory/Official now expose bounded export ports; `enterpriseexport` writes
  deterministic stations/revisions/prices JSONL plus a pinned manifest and
  content hash into a private candidate dir. Active-identifier-only CNPJ,
  sourced status, reviewed coordinates, current-revision-only prices and
  superseded/staging exclusion are covered by real PostGIS tests. A
  repeatable-read snapshot proves a concurrent correction cannot mix into one
  build; no DB write, migration, dependency, route or publication. Next: ESP-04
  deterministic analytics.

- Enterprise data/static portal ESP-02 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Evidence](../backend/evidence/enterprise-static/ESP-02.md).
  Official discovery now accepts the observed current/legacy weekly filename
  shapes, dedupes by bounded content SHA-256 (unreliable ETag), keeps the daily
  two-week scope with an operator correction sweep capped at five weeks, and
  supersedes only the same survey window. `StationCurrentPrices` uses the latest
  station window plus its current pointer, so an older correction cannot become
  current. Unit/race + real-PostGIS Official/read/jobs suites PASS; sqlc stable;
  no migration, dependency, schedule or source activation. Next: ESP-03 export.

- Enterprise data/static portal ESP-01 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING. [Source admission register](../product/ENTERPRISE_SOURCE_ADMISSION.md)
  and [evidence](../backend/evidence/enterprise-static/ESP-01.md). Six ANP
  profiles frozen with fields/keys/clocks/coverage/retention/quotas; rights split
  into `RIGHTS_PENDING` (registry, prices, PMQC), `ADMITTED` CC BY (enforcement,
  fines, still runtime/association blocked) and `NOT_ADMITTED` (registry API).
  Observed weekly filename variant breaks current discovery; historical series is
  semester ZIP; FQ-06 live stream and safe fine links remain OWED. No source,
  schema, job, route or deployment enabled. Next: ESP-02 bounded discovery.

- Enterprise data/static portal ESP-00 2026-10-24: LOCAL_DONE /
  INTEGRATION_PENDING on `codex/enterprise-static-portal` (isolated worktree),
  replayed onto maintained dev `721582b8`; planning commit `b1a2bde6` preserved.
  [Evidence](../backend/evidence/enterprise-static/ESP-00.md). Actual baseline,
  V1/V2 scope, MAP component split, FQ-06/07/08 runtime/safe-link OWED and
  unaccepted MAP-08 work boundary recorded. Docs only; no source, schema, job,
  API or hosting change. Next: ESP-01 source/rights/public-field admission.
- Map/EV execution 2026-10-24: MAP-00–07 LOCAL_DONE / INTEGRATION_PENDING;
  MAP-08 IN_PROGRESS/BLOCKED; engine spike PASS but adapter pending. Commits
  `34750fe1..5cf02a73` on maintained dev after `01c1f068`; no PR/merge/wiki.
  [MAP-00](evidence/MAP-00.md), [MAP-01](evidence/MAP-01.md),
  [MAP-02](evidence/MAP-02.md), [MAP-03](evidence/MAP-03.md),
  [MAP-04](evidence/MAP-04.md), [MAP-05](evidence/MAP-05.md),
  [MAP-06](evidence/MAP-06.md), [MAP-07](evidence/MAP-07.md),
  [MAP-08](evidence/MAP-08.md), [engine spike](MAP-14_SPIKE.md).
  Contracts/goldens for POI/price/native manifest; OSM PBF nodes/ways/relations;
  exact ANP station facts; admitted OCM/PBE reuse; reviewed charging/station
  links and separate access points; electric handlers/ledger charge fixed.
  Tests: real PostGIS linking/migration/concurrency/rollback; Rust station-prep
  full suites/clippy; Go charging/ledger/API process; Kotlin contract/ANP fixture.
  CoMaps `4587b8ad` built (AAR x86_64; APK 144 MB) and proved render, offline
  search, POI selection and a 2,007.85 m offline car route on emulator API 26.
  A generated Curitiba `.mwm` renders but fails `--check_mwm` and an earlier
  routed build crashed; no map package was published. No Abastevo adapter/UI,
  arm64/16KB, data gate MAP-09–13, device manual acceptance, staging or release
  claim. Next: fix MAP-08 generation/check/route, then MAP-09–13 and the
  Abastevo adapter/Compose map before MAP-15–33.
- Map minimum local acceptance 2026-10-25: **MINIMUM_MAP_GATE LOCAL_PASS /
  STAGING_PENDING / PRODUCTION_PENDING**, LOCAL_DONE / INTEGRATION_PENDING on
  `codex/map-ev-minimum-gate`. [Evidence](../backend/evidence/maps/MAP-20_MINIMUM_LOCAL_20261025.md),
  [receipt](../backend/evidence/maps/MAP-20-minimum-local-20261025/receipt.json).
  Real ANP registry (45,706 stations), real 2026-10-11..17 survey facts and the
  ANP Curitiba API were combined with admitted OSM geometry under exact
  address/number + ≤50 m review: 62 reviewed points and 33 exact priced
  stations. A disposable PostGIS API fed the real app: 62 native fuel pins on
  the admitted MWM, quick sheet `R$ 6,79 L` Standard for 2026-10-11..17,
  no-price explicit state and existing profile route. Offline process restart
  used the separate Room public-page/price cache with explicit cached banner and
  price label. Native labels no longer expose UUID/cluster IDs; search is no
  longer clobbered by camera sync. Engine 32, data 612 (2 known external skips)
  and app 537 tests PASS; lint/release size PASS. Unsigned release 78,121,062
  bytes / 74.50 MiB SHA-256 `5937649c…deabdd`, local proof config empty. Staging
  data/distribution origin/keys, opening-hours contract, MAP-21–33 and
  production/G09 remain PENDING. Gate states: MINIMUM_MAP_GATE LOCAL_PASS /
  STAGING_PENDING, OFFLINE_GATE PARTIAL (public station/price overlay only),
  PERFORMANCE_GATE NOT_STARTED, DELIVERY_GATE NOT_STARTED /
  INTEGRATION_PENDING. MAP-21 operational events and MAP-22–33 remain
  unimplemented in this batch. No push/PR/merge/deploy.

- Map review remediation 2026-10-25: bounded repairs LOCAL_DONE /
  INTEGRATION_PENDING on `codex/map-ev-implementation`; inherited Antigravity
  commits `483cff59` and `2d51eef6` and unfinished native work preserved.
  [Current evidence](../backend/evidence/maps/MAP-15_20_REMEDIATION_20261025.md);
  [previous progress preserved](history/MAP_REMEDIATION_PROGRESS_BEFORE_20261025.md).
  Fixed rollback/cancellation/reader consistency, real Gradle distribution
  values, Main-only native reload/query handling, pan/zoom restoration,
  conditional prices and reachable source controls, general city/week reuse.
  Real phone signed-download → exact A/B activation → regional-map registration
  → offline search → Activity recreation PASS (1/1); route/screen PASS (5/5).
  JVM/domain/application/data/engine/app and scoped release security checks PASS.
  Final emulator UI 5/5 at 200% font / 320 dp; final app JVM 535/535 + lint
  PASS. Unsigned release 78,121,062 bytes / 74.502 MiB, all native ELF/ZIP
  alignments 16 KB. Source checkpoint `51cc4457`; final receipt pins hashes.
  Phone disconnected; temporary emulator and proof server stopped.
  MAP-15/16/17/19 retain local implementation checkpoints; MAP-18 artwork and
  MAP-20 / MINIMUM_MAP_GATE remain PARTIAL. Admitted live station geometry/
  prices, product distribution config, native combined artwork, opening-hours
  contract and public offline-overlay acceptance remain named obligations.
  No push/PR/merge/wiki/deploy or production/pilot certification. MAP-21–33 untouched.

- Map/EV checkpoint 2026-10-25 (measurements 2026-10-24): MAP-00–13 LOCAL_DONE / INTEGRATION_PENDING; MAP-14
  **LOCAL_DONE / ENGINE_GATE_PASS / INTEGRATION_PENDING** on
  `codex/map-ev-implementation`, from `8e707b62` plus the checkpoint commit
  containing this record. [Current evidence](../backend/evidence/maps/MAP-14_ENGINE_GATE.md),
  [acceptance receipt](../backend/evidence/maps/map14-final/acceptance.json),
  [resume](MAP_EV_RESUME.md). MAP-15/16 local records remain; their retained
  integration gaps must be closed as engineering tasks, not owner decisions.
  Both ARM ABIs have byte-identical independent-checkout AAR/classes/core
  comparisons, exact resource admission (193 admitted / 2 excluded fonts), full
  retained notices and expanded runtime-code audit (36 hosts classified, 0
  unknown, 0 secret-pattern hits). Actual ARM64/16 KB/OpenGLES3 and
  ARM32/API30/Vulkan render/search/route passed; four native rotations per ABI,
  old Activity collection and three ARM32 process restarts passed. The ARM32
  OpenGL emulator attempts crashed the emulator and are not accepted profiles.
  APK 77,794,496 bytes / 74.19 MiB, both advertised ARM ABIs complete, all 64-bit
  LOAD/ZIP alignments 16 KB, existing 80 MiB ceiling unchanged. Persistent
  candidate: `/data/dev/artifacts/abastevo-map-engine/d2ec37881ede4e22`.
  Fixed SDK lifecycle forwarding/buffer teardown, readiness-before-surface,
  retired callbacks, generation-correlated KML category recovery and search
  snapshots. Eight Python, 19 engine JVM and 4 Compose instrumentation tests
  PASS; affected app JVM run 490 PASS, normal debug, engine probe/release,
  lint and scoped integrity checks PASS. No redundant backend/SQL aggregate.
  Next: close MAP-15 native activation/API26 crypto/recovery integration items,
  then MAP-17 real four-tab/native host wiring; MAP-18/19/20 own final markers,
  disambiguation, prices and minimum-map acceptance. No fabricated map UI,
  source activation, phone installation, push/PR/merge, deploy or release claim.
  [Earlier detailed progress](history/MAP_ENGINE_PROGRESS_BEFORE_ACCEPTANCE_20261024.md)
  preserves the superseded checkpoints and all outstanding product obligations.

- Enterprise data/static portal planning 2026-10-24: PLANNING_LOCAL_DONE /
  IMPLEMENTATION_PLANNED / LOCAL_ONLY. Planning commit `b1a2bde6` was preserved
  from its old local-dev parent and replayed onto maintained dev `721582b8` in
  ESP worktree branch `codex/enterprise-static-portal`; the map worktree remains
  untouched. [ESP-00–18 plan](ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md),
  [rules/use cases](../product/ENTERPRISE_DATA_STATIC_PORTAL_CONTRACT.md) and
  [continuation prompt](ENTERPRISE_DATA_STATIC_PORTAL_PROMPT.md) define existing
  ANP/FQ reuse, analytics, static search/pages, versioned publication, correction
  recovery and measured VPS/CDN capacity. Next: ESP-00 actual baseline/scope
  reconciliation and component split with the map. No source activation,
  D implementation, VPS/load/deployment or release acceptance.

- Fuel/electric map planning 2026-10-24: PLANNING_LOCAL_DONE /
  IMPLEMENTATION_PLANNED, isolated from occupied checkouts at `b90a0cd1`.
  [MAP-00–33](MAP_EV_EXECUTION_PLAN.md), [phase registry](MAP_EV_PHASES.json),
  [rights/source register](MAP_EV_SOURCE_REGISTER.md), [agent prompt](MAP_EV_AGENT_PROMPT.md).
  User-requested data-first pipeline, commercial/open-only dependencies, CoMaps
  native proof gate, four-tab minimum map and later offline/EV journeys are scoped.
  [Prepared map icons](../assets/map-icons/README.md) retain raw/prompt/SVG/PNG;
  one accidental electric SVG void was explicitly removed and re-exported with
  bounded-pixel repair proof. This is planning/assets only, no product code,
  source admission, phone/VPS change, push/PR/merge/wiki or release certification.
  Validation: registry/link/structure/diff/asset checks; no runtime test claim.
  Next: integrate the scoped planning artifacts when construction is requested,
  then reconcile actual baseline at MAP-00. Native feasibility and external gates
  remain unproven.
- Android Premium area 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  maintained `dev`; connected-device update installed, dev push follows this record.
  [Evidence](../mobile/premium-area-organization.md). Dedicated Premium and
  Manage subscription pages/routes, Profile category below the signed-in card,
  compact Manage-account shortcut, old account Premium section/query removed,
  owner-fenced VM moved, resume-scoped status refresh, 60s account-scoped
  display cache, structural Premium/Management loading skeletons, fixed
  light/dark gold palette, local crown, 25 strings × 8 locales and three-tab
  Profile context preserved. Profile loading now uses a full-page skeleton with
  no mount animation; Auth login crossfade untouched. App-wide content-card
  shape standardized on the active medium radius (12dp), with one-offs limited
  to dialogs, dropdowns, sheets, badges and controls. 490 app tests 0
  failures/errors/skips; lint 0 errors (257 existing warnings); debug APK
  28,272,706 bytes, SHA-256 `f2869b78…`, installed via `adb install -r` on
  2311DRK48G without data clear. Purchase/restore remain unavailable by design;
  no backend/billing change. Rendered visual/font/TalkBack review remains
  VISUAL_VALIDATION_PENDING for the owner; no production or release claim.

- Android container outlines 2026-10-24: LOCAL_DONE / INTEGRATION_PENDING on
  maintained `dev`, no PR/push/merge. [Rule and validation](../mobile/UI_CONTAINER_GUIDELINES.md).
  Shared `AnpContainerDefaults` resolves a 1dp neutral outline from the active
  Material scheme (`outlineVariant`→`outline`, ≥2:1 against the effective fill),
  preserving shape, fill, semantics and state borders under light/dark/dynamic
  colors. Applied to Home vehicle/fuel/benefit/skeleton cards, vehicles,
  stations list/summary/details/public page, account/auth/key backup, settings/
  privacy/consent, profile/contributions, community/charging, ledger/monthly,
  games, help, suggestions, capture and occurrence. `OutlinedCard`, selectable
  option rows, branded hero and status/badge surfaces keep their specialized
  treatments. Commits `26b8534a`, `0e6e95ce`, `f80fab46`. Full app unit suite
  470 tests 0 failures/errors/skips; lint 0 errors (258 existing warnings);
  debug APK and `git diff --check` PASS; no credentials/PII added. Rendered
  light/dark/dynamic, 200% font, narrow-device and TalkBack checks remain
  VISUAL_VALIDATION_PENDING for user device review; no blanket accessibility
  claim.

- Test-environment update 2026-10-24: API/worker `53719336` deployed to the
  explicitly selected `teste.abastevo.com.br`; schema95→120 rehearsed on a
  restored copy, quiesced backups retained, original data fingerprints unchanged
  across105 tables. API/worker/storage ready with zero restarts; public health200,
  signed consent16 checks and account/session30 checks PASS. Physical debug APK
  replaced via ADB with matching signature/hash and retained app data; UI tests
  remain user-owned. [Runtime evidence and gaps](../backend/evidence/TEST_ENV_UPDATE_2026-10-24.md):
  EV community/rank/history routes unmounted, charge reads/delete selector incomplete,
  charging sources unpublished; full electric runtime acceptance remains PARTIAL.
  Local test setup/restore repairs: Go race2414 and integrated3352 outcomes PASS
  (2/6 opt-in skips), real source→HTTP/PostGIS, static/security/restore/load PASS.
  Test-harness corrections LOCAL_DONE / INTEGRATION_PENDING; no new remote delivery
  or G09 certification. Next: bounded runtime-gap correction and user manual checks.

- Delivery integrated 2026-10-24: PR #147 merged to protected main as
  `c82823cb` (head `3fa0bd08`, base `c3de5c03`); PR #149's five audited commits
  are ancestors of main (`ebd777ef` etc.) and the PR is MERGED via the dev
  reconciliation merge `9c93892c`. Current-head required checks
  (Quick verification, fast, test, integration) all passed before the guarded
  finish; local quick PASS. Wiki published once from `c82823cb` as `02e5391`
  (dry run no conflicts, zero mutation before publish). `dev` retained and
  synchronized to the merge; this entry is a docs-only post-merge note.
  External obligations (Play/staging/operator/device/production) remain OWED;
  no RELEASE_CERTIFIED or production-ready claim.

- Final union after PR149 merge 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING,
  merge `9c93892c`. Re-ran the union instead of reusing pre-merge counts:
  domain675/application454/data549/app465 = 2,143 JVM (0 failures/errors, 2
  pre-existing skips), lint/APK, Go race/vet/staticcheck(1.27.1)/sqlc, real
  PostGIS integration (trust transient DB-startup failure passed isolated),
  Rust177/fmt/clippy PASS; occurrence refresh + PR149 revision fencing both
  preserved, gofmt drift fixed `f522a884`. Next: current-head checks and
  guarded PR147 dev → main.

- EV/Android batch map 2026-10-23 (authorized, sequential): MEX-16 (Premium
  status), EE-24 (EV sync/privacy), MEX-32/EE-26 (shared refresh), EE-22
  (community EV), EE-25/MEX-31 carried gaps (scenario execution/vehicle
  context), EE-27 (end-to-end), MEX-33/34, EE-28/MEX-35 (measurements),
  EE-29/MEX-36/37 (regression/privacy/union), EE-30/MEX-38 (manifest/handoff),
  EE-31/MEX-39 (combined overview). MEX-16, MEX-32+EE-26, EE-22 and the
  acceptance units have real gaps; already-accepted EE-00–21/23/25 and
  MEX-14/15 code is reused, not reimplemented. Delivery: one atomic commit
  and normal dev push per unit, no CI wait during construction; final
  inventory/reconciliation of all open PRs and guarded dev → main integration
  happen after the union. Native/device, staging, provider, sales and
  deployment remain separately OWED.

- EE-31/MEX-39 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent
  `80bd7eb2`. [Combined overview](../backend/evidence/ev-support/EE-31.md):
  Portuguese task/commit/test/measurement map for the 17-commit batch; 2,130
  JVM (2 pre-existing skips), Go/Rust/PostGIS/OpenAPI, real source chains and
  operational manifest. Explicit pending: EE-25 energy/scenario execution,
  MEX-35 combined device campaign, Android vehicle backup, exact Go-page
  Android decode, staging/Play/native/operator/production. No release claim.
  Next: final GitHub inventory and protected dev → main delivery.

- MEX-38 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `83b77df4`.
  [Operational handoff](../backend/evidence/mobile-expansion/MEX-38.md): reuses
  the EE-30 source-ready manifest and local restore/erase/replay rehearsals;
  exact external gates (Play sandbox, staging two-device/private objects,
  operator access, physical Android, production deploy/load/restore, pilot)
  remain OWED. No VPS/deploy/sales/source activation, no G09 claim.

- EE-30 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `113d6df7`.
  [Source-ready manifest](../backend/evidence/ev-support/EE-30.md): pins code
  head `887d5a71`, append-only `000120`, Room 25, OpenAPI/profile/vector hashes,
  private OCM/PBE/enforcement artifact hashes/census, feature defaults and
  license boundaries. SOURCE_READY_LOCAL only; staging/integration/release and
  all external proofs stay OWED. No deploy/activation.

- MEX-37 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `4c8f86eb`.
  [Final scoped union](../backend/evidence/mobile-expansion/MEX-37.md): code
  head `887d5a71` full matrix 2,130 JVM (0 failures/errors, 2 pre-existing
  skips), lint/APK, Go race/vet/static/sqlc/PostGIS/OpenAPI, Rust 177/fmt/
  clippy, compat script and 8-locale parity PASS; later commits are docs only.
  Physical TalkBack/font/offline/device Room and staging remain OWED. Next
  EE-30/MEX-38 manifest.

- MEX-36 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `54e8a021`.
  [Privacy/all-copy recovery](../backend/evidence/mobile-expansion/MEX-36.md):
  real PostGIS erasure reuse/outage/retry, stale/double restore, delete-during-
  activity restore, foreign-owner/free export paths and CSV formula refusal
  PASS; secret/PII scan clean; EE-22 wire carries no GPS/route/home/photo.
  Real S3/staging object receipts remain OWED.

- EE-29 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `887d5a71`.
  [Retained/upgrade/privacy](../backend/evidence/ev-support/EE-29.md): full
  matrix 2,130 JVM (0 failures/errors, 2 pre-existing skips), lint/APK, Go
  race/vet/static/sqlc/gofmt and real PostGIS integration, Rust 177/fmt/clippy
  PASS. PostgreSQL upgrade derives every ≥000109 migration; SQLite 24→25 6/6;
  CSV formula refusal; source data stays outside MIT. Physical Room/device and
  staging receipts OWED.

- MEX-35 2026-10-23: PARTIAL / INTEGRATION_PENDING, parent `dd9d9c7e`.
  [Discovery measurement map](../backend/evidence/mobile-expansion/MEX-35.md):
  cited prior MEX-06/OPT-A/MEX-24/28/29 measurements plus EE-28 real prep/public
  chain; no new combined mobile workload, device p50/p95, payload/memory or
  safety-job contention campaign. No new optimization justified; combined
  campaign OWED, no invented numbers. Next EE-29/MEX-36/37.

- EE-28 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `8ed7b1de`.
  [Bounded measurements](../backend/evidence/ev-support/EE-28.md): real
  `prepare_charging` 0.78 s cold / 0.66 s warm, 15.1 MiB max RSS, 3.33 MB
  entities + 1.60 MB decisions, 3,387/3,387 accepted, byte-identical repeat;
  real Rust→Go→PostGIS→13-page HTTP chain 1.28 s. Android device first-content/
  scroll/cache and concurrent backend p95 remain OWED; prior MEX-06/OPT-A
  measurements are cited, not re-run. No third-party load. Next EE-29/MEX-36.

- MEX-33 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `c9c7d13a`.
  [Private E2E/free boundary](../backend/evidence/mobile-expansion/MEX-33.md):
  real PostGIS ledger/cloud journeys (two-device conflict/delete/replay,
  consent revoke, late correction invalidation, report/erase, backup lapse/
  isolation, privacy delete/restore, free export/erasure/foreign refusal) and
  JVM/real-SQLite selectors PASS. Append-only migration expectation repaired
  to derive ≥000109. Staging/two-device/object receipts and physical Room
  remain OWED.

- MEX-34 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `3b36e117`.
  [Official source-to-mobile](../backend/evidence/mobile-expansion/MEX-34.md):
  pinned 16,188,087-byte enforcement workbook + independent census + trusted
  nonroot preparer through the real worker/source lease/PostGIS
  replay-correction-full traversal PASS in 60.4 s; wrong workbook refused
  first; Android occurrence/public typed selectors PASS. Exact whole-source
  Android decode/device remains OWED; flags OFF, no activation.

- EE-27 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `f46a7f2c`.
  [End-to-end acceptance](../backend/evidence/ev-support/EE-27.md): actual
  admitted OCM Rust artifact → strict Go bridge → PostGIS publication → full
  public HTTP cursor traversal (1,298 places/2,089 groups/0 vehicles, coverage
  equal, no PII) PASS; Rust→Go parity on both real batches PASS; signed
  community PostGIS command/moderation and private ledger/cloudsync real
  PostGIS suites PASS. Android decoding of the exact Go page and physical Room
  import remain OWED; no mocks-only claim. Next MEX-33/34.

- MEX-31/EE-25 gap 2026-10-23: PARTIAL / INTEGRATION_PENDING, parent
  `82ea53c3`. [Gap record](../backend/evidence/ev-support/EE-25-MEX-31-GAP.md):
  master EV place intents now open compatible charging with a real BEV/PHEV
  vehicle (single/choice/none-add), and energy/scenario intents show explicit
  execution-pending notices instead of dead routes. Reported-energy ranking and
  complete scenario cost remain OWED on missing place-level aggregate/provider
  tariff inputs; interpretation is not counted as a result. Master tests/lint/
  APK and eight locales PASS. Next EE-27.

- EE-22 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `b5fc0277`.
  [Community EV journeys](../backend/evidence/ev-support/EE-22.md): completed
  EE-12/13 HTTP reads (owner revisions), helpfulness and report endpoints with
  self-action refusal, OpenAPI; Android typed client, durable account/origin
  pending store, use case, ViewModel and Compose journey (publish, list, edit,
  withdraw, vote, report), offline/conflict/signed-out states and 47 strings ×
  8 locales. Real-PostGIS command/moderation tests, client/use-case/VM tests,
  lint/APK, vet/sqlc/OpenAPI PASS. No GPS/home/route/photo in wire. Device/
  staging E2E and operator moderation actions remain OWED. Next EE-25/MEX-31
  execution gaps.

- EE-26 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `05c22e23`.
  [Refresh/stale policy](../backend/evidence/ev-support/EE-26.md): charging
  snapshot age is UNKNOWN/FRESH/STALE at a 24 h boundary, shown in eight
  locales as dated source (never live availability); duplicate triggers stay
  single-flight and failed refreshes keep rows; occurrence/community reuse
  the accepted event/manual policy. Domain/app/data targeted tests and
  compilation PASS; byte/SQL measurement and physical lifecycle evidence
  remain OWED. Next EE-22.

- MEX-32 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `188c2cdb`.
  [Shared refresh](../backend/evidence/mobile-expansion/MEX-32.md): community
  policy reused; advisory `refresh_after_seconds` decoupled from a fixed 15
  (bounded 0..3600, never a policy); charging and occurrence manual refreshes
  are single-flight and keep loaded windows; eight locales verified free of
  fixed-interval claims. Targeted app/data tests and compilation PASS. Device
  lifecycle and byte/SQL measurements remain OWED. Next EE-26.

- EE-24 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `af32cea5`.
  [EV sync/privacy allowlist](../backend/evidence/ev-support/EE-24.md): MHEV
  is a first-class hosted kind (tank-only, same as HEV); vehicle payloads use
  the frozen allowlist and preserve unknown newer/energy fields through both
  `PutVehicle` and sync `Apply` for legacy partial updates. Existing MEX-14/15
  ledger transport, account/origin and erasure guards reused. One RED
  regression (sync Apply erased stored energy/unknown fields) closed; cloudsync
  unit/HTTP suites, targeted real-PostGIS MHEV/preservation/concurrency tests,
  vet/gofmt/diff/secrets PASS. Android vehicle cloud transport is absent from
  this codebase and remains an explicit product gap, not claimed. Next
  MEX-32/EE-26.

- MEX-16 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING, parent `a5c4ed37`.
  [Premium status](../backend/evidence/mobile-expansion/MEX-16.md): signed-in
  owner sees server-verified state/capabilities/paid-through/retained-data,
  manage link, manual refresh and honest offline/denied/error states; restore
  is explicitly unavailable because no Play Billing token source ships. No
  local entitlement derivation, purchase or provider activation. 12 targeted
  tests (5 data, 2 application, 5 app) plus debug APK/lint PASS; 18 strings ×
  8 locales; diff/secret clean. Sandbox/provider quotes remain
  EXTERNAL_PROOF_OWED. Next EE-24 EV sync/privacy.

- MEX-15 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING on maintained `dev`, parent `52c994b3`. [Conflict, backup controls and restore](../backend/evidence/mobile-expansion/MEX-15.md): explicit keep/take/cancel, device/dataset gates, honest last-ack status, owner-bound expiring restore preview/confirm, transactional local apply, erasure/legacy-delete guards and 27 strings × 8 locales. Full JVM 2093 tests, 0 failures/errors (2 pre-existing live skips), six real SQLite cases, lint/debug APK/diff/secrets PASS. Tested source fingerprint in evidence; local task tracking, issue/PR unchanged and remote checks NOT_RUN. One local atomic commit only; no push/PR/merge. Native process/Room and signed two-device/staging/private-object receipts remain OWED. Next: MEX-16 bounded source phase; final integration separate.

- MEX-14 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent6d195688. [Private sync transport + constrained worker](../backend/evidence/mobile-expansion/MEX-14.md): explicit opt-in (default OFF), typed ledger POST with live proof, atomic claim/ack/conflict/backoff on reused outbox, one-command Hilt worker + unique CONNECTED/backoff scheduling, pausing keeps rows. Full suites (2038 tests, 0 failures; 2 pre-existing live skips), app/data lint, diff, secrets green; device reopen deferred. Two-device/reconnect runs stay staging-gated.

- MEX-31 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parentd1e1b4d6. [Master discovery journeys](../backend/evidence/mobile-expansion/MEX-31.md): on-device interpreter port (golden parity), typed ranking client (no-store, cancellable, bounded), IBGE-exact municipal resolution, master screen (suggestions, interpretation, NEEDS_CHOICE, ranking results, official deep-link), 11 strings x 8 locales. Full suites (2029 tests, 0 failures), lint, diff, secrets green; device reopen deferred.

- EE-25 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parentd1e1b4b6. [Electric master questions](../backend/evidence/ev-support/EE-25.md): typed EV intents (compatible places, reported energy price, complete scenario cost) with pinned source/bounded pages, NEEDS_CHOICE for ambiguous/missing, refusals without guessing; deterministic vectors green, no liquid/official regressions. Backend vet/static/diff green; no OpenAPI/SQL change.

- EE-23 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parentb26c5547. [Energy comparisons](../backend/evidence/ev-support/EE-23.md): Go/Kotlin golden parity corpus (11 vectors, both engines green), pure energy engine with weighted compare + km/L-equivalent refusal, monthly measured/reference/comparison/combined, formula-safe CSV export, energy UI block (7 strings × 8 locales). Full suites (2009 tests, 0 failures) + backend metrics, lint, diff, secrets green; device reopen deferred.

- MEX-13 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parenta5e98507. [Monthly indicators](../backend/evidence/mobile-expansion/MEX-13.md): month/vehicle totals, measured vs estimated, cost/km + combined with window explanation, stale-safe rebuilds, comparison, formula-safe CSV export, 19 strings × 8 locales. Full suites (2005 tests, 0 failures), lint, diff, secrets green; device reopen deferred. No revenue/guessed denominators.

- EE-21 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent7d2c35dd. [Compatible discovery](../backend/evidence/ev-support/EE-21.md): pure compatibility verdicts, snapshot-backed repository (OCM-only sync, atomic generations, wrong-base/corrupt refusal, offline reads), per-vehicle Explore screen (filters, badges, source age, no live claims), 21 strings × 8 locales. Full suites (1996 tests, 0 failures), lint, diff, secrets green; fuel discovery untouched; device reopen deferred.

- EE-20 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parenta655095a. [Private charging entry](../backend/evidence/ev-support/EE-20.md): home/work/public/other local context (migration v23, never synced/posted), charge form (kWh/rate/SOC/tariff pins, simulation vs actual, pt-BR parsing, electric gate, typed errors), late correction with revision/context, LOCAL/PENDING/CONFLICT states, double-tap guard, restart reload, account isolation, rollback-safe saves. Full suites (1986 tests, 0 failures), lint, diff, secrets green; device reopen deferred. No dispatch.

- MEX-12 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parent7f4b300. [Refuel/expense journeys](../backend/evidence/mobile-expansion/MEX-12.md): entries UI per vehicle (list/add/edit/delete, fills, categories, pt-BR parsing, typed per-field errors, LOCAL/PENDING/CONFLICT, gap/reset intents, estimated km/L, double-tap guard, restart reload), 50 strings × 8 locales. Full suites (1979 tests, 0 failures), lint, diff, secrets green; device reopen deferred. No transport (MEX-14).

- MEX-11 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parentf7d8110d. [Durable ledger drafts/outbox](../backend/evidence/mobile-expansion/MEX-11.md): validated refuel/expense/charge drafts with exact canonical bodies, deterministic op IDs (double-save converges, lost-ack retries same ID), partition-keyed draft table v22, reused premium outbox + restart release, 3 use cases (no transport). Full suites (1967 tests, 0 failures), lint, diff, secrets green; device reopen deferred. No dispatch/sync.

- MEX-08 2026-10-26: LOCAL_DONE / INTEGRATION_PENDING, parenta3b4c597. [Local free/Premium vehicle admission](../backend/evidence/mobile-expansion/MEX-08.md): pure 23-vector admission rule + lease resolution (versioned owner/origin, 24h/paid-through bounds, rollback/revoke/unknown refusal), atomic Room admit (replay free, recount in-tx, no count-then-save), lease table v21, gated save + honest Premium entry UI (3 strings × 8 locales). Full domain/data/application/app suites (1952 tests, 0 failures), app+data lint, diff, secrets green; device reopen deferred. No live proof/purchase/sync.

- EE-19 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentc48c3abd. [Energy-capable vehicle model](../backend/evidence/ev-support/EE-19.md): Propulsion/Battery/connector on Vehicle with EE-10 parity (BEV no tank/fuel, PHEV both, HEV/MHEV tank-only, unknown refused, no backfill), nullable tank/fuel with BEV excluded from fill estimates/alerts, same 3-slot gate, Room v20 rebuild preserving legacy rows as ICE, conditional UI with localized kWh, 10 strings × 8 locales. Full domain/data/application/app suites, app+data lint, diff, secrets green; device reopen deferred. No sync/discovery/reports.

- EE-18 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent679a3c9f. [Android typed public snapshot](../backend/evidence/ev-support/EE-18.md): strict DTOs with unknown-enum retention, codec + cancellable no-store HTTP client (EE-09), pure pack planner (admitted profiles, hash/base/origin/schema/tombstone/coverage/budget gates, resume cursor), atomic Room generations v19 + additive migration (fuel/history/private untouched, Room schema cross-checked). Domain/data suites, data lint, diff, secrets green; device reopen deferred per standing directive. No UI/wiring/activation.

- EE-17 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent4ca4b3a3. [Licensed operator read adapter](../backend/evidence/ev-support/EE-17.md): executable ACCESS_PENDING port (pinned read-only config, credential/expiry resolution, typed fault taxonomy, UNKNOWN never available, LICENSED_OPERATOR tariff stamping, routeless default fails closed); 11 config refusals, pending/expiry/route/replay/stale/scope/revoke/cancel/secret-hygiene PASS; charging race, vet, staticcheck, gofmt, diff, secrets green. No credential, feed, wiring or activation.

- EE-16 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent6993bdef. [Electric/hybrid reports](../backend/evidence/ev-support/EE-16.md): ledger→report charge aggregation (stated energy/cost/fees, nulls out, fees separate), pure sum-first energy/combined metrics, PHEV shared-distance basis (`phev-shared-distance`, money exact, counted once), projection v3, CSV lines; charge-anchored gaps (adapter+SQL), erasure revokes charge bit, consent reports charge. Golden parity, unequal weights, cross-month/timezone, late correction, SOC/gap/segment/missing, erasure PASS with PostGIS; ledger/api/apicontract race, vet, staticcheck, sqlc, gofmt, diff, secrets green. No activation.

- EE-15 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent7b2c6a95. [Private electric session backend](../backend/evidence/ev-support/EE-15.md): Charge type with optional energy/total/SOC/odometer/fees/tariff-revision pins in owned ledger (new charge dataset + consent bit via append-only migrations); PHEV/fuel coexistence, nulls preserved, revision idempotency, concurrent conflicts, account isolation, no public leak. CRUD integration PASS with PostGIS; ledger module, vet, staticcheck green. No activation.

- EE-14 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent48a8f555. [Tariff and observation reads](../backend/evidence/ev-support/EE-14.md): exact tariff ranking by scenario total (rational arithmetic, stable ties, unknowns last, member-target isolation) plus community history pages with tallies; OpenAPI paths; ranking tests PASS; charging module, vet, staticcheck, vacuum zero errors green. No stored tariffs or activation.

- EE-13 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent6abb3479. [EV community projection and moderation](../backend/evidence/ev-support/EE-13.md): helpfulness votes/reports with idempotent convergence, projection with tallies excluding hidden/deleted/expired (no writes for expiry), conflicting votes coexist; integration PASS with PostGIS; domain, vet, staticcheck, sqlc vet green. No verified-price claims or lifecycle automation.

- EE-12 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentc8490dae. [Signed community EV observation commands](../backend/evidence/ev-support/EE-12.md): account-scoped observations (connector/price/condition/day/note) with quotas, revision-guarded correction/withdrawal, idempotent replay by ID, session-authenticated POST/PUT/DELETE; forged/cross-owner/malformed refuse. HTTP→DB integration PASS with PostGIS; domain, vet, staticcheck, sqlc vet green. No projections/moderation lifecycle (EE-13).

- EE-10 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentf08b6740. [Hosted vehicle energy-capability migration](../backend/evidence/ev-support/EE-10.md): propulsion/battery pointers on Vehicle with BEV/PHEV/HEV/ICE rules, no backfill, merge-preserving partial updates with revision-guarded races; owner BEV, legacy merge, concurrent conflict and account isolation PASS with PostGIS; domain, vet, staticcheck green. No migration (JSONB), HTTP or activation.

- EE-09 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentda7a9551. [Public charging/reference APIs](../backend/evidence/ev-support/EE-09.md): bounded pages/detail/coverage under explicit profiles with keyset/cursors/ETags, kill switch (default true, false answers 404), OpenAPI paths/schemas; traversal/coverage/malformed/OpenAPI tests PASS with PostGIS; charging module, vet, staticcheck, sqlc vet/diff, vacuum zero errors green. No ingestion/activation.

- EE-08 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent2a0f39e7. [Verified ingestion jobs](../backend/evidence/ev-support/EE-08.md): bounded Runner (workspace, trusted binaries, cleanup, dead budgets, idempotent replay by manifest hash) plus allowlisted Discover (https/TLS12/no-redirect/1 MB/30 s, no SSRF) and pure merge/split review flags; queue handlers without auto-actions; config gate default-off; Dockerfile + CI with both preparers as nonroot. Seven job tests plus integration PASS; vet/staticcheck green. No dispatcher/schedule activation.

- EE-07 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent7c7414d8. [Append-only charging/reference storage](../backend/evidence/ev-support/EE-07.md): migration 000115 with source-owned immutable tables (batches, places, groups, vehicles) plus owned charging sqlc package; StoreBatch stages verified batches in kind order with per-batch census and rollback; Directory links stay null. Real PostGIS upgrade/replay preserves data (3,387 + 181 entities); race, vet, staticcheck, sqlc vet/diff green. No publication/reads/linking.

- EE-06 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent41a0a29e. [Strict Go prepared bridge](../backend/evidence/ev-support/EE-06.md): versioned manifest/integrity/census/rights/ID admission with recomputed content identity and source keys, bounded streams, fail-closed on corruption/cancel/duplicate/license/oversize; prerequisite frozen-envelope fixes to both CLIs (array provenance, uppercase outcomes, sorted streams, full schema validation). Unit plus real Rust→Go parity on both batches PASS with race; charging module, vet, staticcheck, 177 Rust tests, fmt/clippy green. No storage/publication/runtime.

- EE-05 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent1001dbc5. User-authorized PBE admission plus dealer BYD supplement (provenance documented). [Rust vehicle-reference preparation](../backend/evidence/ev-support/EE-05.md): PBE nine-layout reader (gap-split fields, right-anchored MJ/range, comma decimals, frozen minimal payload) plus supplement path; 181 distinct EV references with 5 cross-listed duplicates and full loss accounting; MJ/km cross-validated with supplement; extraction via reviewed pdftotext adapter, no Rust dependency. Real-file acceptance matches oracle; 177 Rust tests, fmt/clippy green. No bridge/storage/activation.

- EE-04 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentc3de5c03. User-authorized OCM admission (open licenses 1/40/57). [Rust charging-location preparation](../backend/evidence/ev-support/EE-04.md): whole-Brazil snapshot (1,298 POIs, pinned commit, blob-verified, 1.8 MB) with license/PII analysis; profile amended to admitted; dependency-free Rust reader (places + connection groups, exact lexemes, frozen enums, assertion digests) plus prepare_charging CLI (4-file batch, deterministic, 0600/0700). Real-file acceptance matches independent oracle (3,387 units, zero PII, unique keys, byte-identical repeats); 174 Rust tests, fmt/clippy green, no new dependency. No vehicle/bridge/storage/activation.

- Recent-main audit 2026-10-23: LOCAL_DONE / INTEGRATION_PENDING. [Independent review](../backend/evidence/reviews/RECENT_MAIN_PRS_AUDIT.md) and [acceptance](../backend/evidence/reviews/RECENT_MAIN_PRS_ACCEPTANCE.json). PR140–146 are actually merged through main `c3de5c03`; older per-phase pending notes below remain historical. Corrected guarded enforcement publication/live company reads, nullable geography, Android revocation/stale/detail/revision/filter handling and seven exact upgrade assertions; delivery guard now rejects enabled failed/incomplete CI even outside the required list. GitHub confirms PR142/145 merged with failed integration signals. Final local union116 Go packages/3287 outcomes, Kotlin644/390/477/410 (two existing data skips), Rust170, whole244247-row/two2443-page source traversal, security/static/build/lint PASS; domain90% gate repaired via behavioral default-event identity/clock tests (90.44%, no weakened gate). Changes isolated on `codex/audit-recent-main-prs`; preserve the other agent's occupied dev and primary changes. Next: reconcile this tested correction into maintained dev when its front is ready, then current-head protected delivery; no merge/deploy/activation/manual or production certification claimed.

- MEX-27 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent69d584bb. User-authorized Android scope. [Station filters and timeline](../backend/evidence/mobile-expansion/MEX-27.md): modal filter sheet (UF/segment/kind with clear/apply), company/source-global date-descending timeline sections, preserved filters/pages, every selection hits the wire, no clean-filter inference; six ViewModel tests plus navigation regression PASS; app lint green with eight locales. No backend/Room/activation.

- MEX-26 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent30228626. User-authorized Android scope. [National occurrence explorer](../backend/evidence/mobile-expansion/MEX-26.md): Compose explorer (filter chips, lazy list, coverage, stale/unavailable/empty) plus source detail with no-guilt copy, ViewModel with cursor/generation/saved-state handling, occurrence-explorer/detail routes without a new tab, OCCURRENCE_UI_ENABLED flag, eight-locale strings. Four ViewModel tests plus navigation regression PASS; app/data lint green. No backend/Room/activation.

- MEX-25 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parenta5b08ef5. User-authorized Android scope (G09/G24). [Public occurrence Android types](../backend/evidence/mobile-expansion/MEX-25.md): strict DTOs/scopes/vectors in domain with unknown-enum retention; cancellable typed HTTP client over accepted 23/24 APIs reusing the shared public cache (ETag/304/404/no-store, validation-before-cache, private refused, stale disclosed); five domain plus five client tests PASS; lint and local-cache/Room regression green. No UI/backend/activation.

- MEX-30 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentf56aeb51. [Deterministic question interpretation](../backend/evidence/mobile-expansion/MEX-30.md): stdlib-only PT-BR interpreter in a new exploration module (no DB/HTTP/LLM); price intents with exact fuel/unit/geography/metric/week/condition plus official routing; ambiguous metrics/fuels/sources/periods ask with labelled alternatives, unknown/adversarial refused literally; six frozen suggestion chips all resolving; outputs validate against the frozen exploration schema. Twelve tests PASS; vet/staticcheck green. No execution/reads/activation.

- MEX-24 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentbd19f0c7. [Station occurrence filter query](../backend/evidence/mobile-expansion/MEX-24.md): distinct bound stations with record counts by exact state/segment/day facets plus coverage; ambiguous/unmatched/retired links excluded live, no-price stations included, cursors bind revision/filters, index proven with 20k unrelated rows; prerequisite fix stages admitted-subset associations atomically with publication. Station/coverage/plan tests plus pre-existing suites PASS with race; quality module, vet, staticcheck, sqlc vet/diff, vacuum zero errors green. No projection table justified, no activation.

- MEX-23 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentde13d57a. [National source-record read API](../backend/evidence/mobile-expansion/MEX-23.md): bounded global pages with exact facets (UF, municipality, segment, subject kind, document-day range) plus coverage endpoint (observations vs cases, day bounds, basis); unknown facets page empty, malformed 400, unmatched/unknown included, date_basis declared, cursors bind revision/filters/limit; facet index proven with 20k unrelated rows; keyset traversal complete; HTTP/OpenAPI validated with no PII. Facet/coverage/integration tests plus pre-existing enforcement suites PASS with race; quality module, vet, staticcheck, sqlc vet/diff, vacuum zero errors green. No worker/publisher/activation.

- MEX-22 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parenta325890d. [Source-record identity and grouping](../backend/evidence/mobile-expansion/MEX-22.md): pure domain correlation for the read stage; document-scoped cases (only documented link), distinct results retained unmerged, explicit observation vs case counts, declared date basis (document day or UNKNOWN), stable cross-revision case keys; repeated snapshots raise no count, corrections keep the case, missing dates/documents handled without loss. Five unit tests PASS; full-source proof (244,247 observations into 141,290 cases, all retained, 33 s, local) matches the document census; domain/application, vet, staticcheck green. No storage/HTTP/activation.

- MEX-21 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent08495125. [Revalidation and conservative association](../backend/evidence/mobile-expansion/MEX-21.md): enforcement company linkage under the current-company policy reusing exact-CNPJ resolution; admitted records only (source-global records refused, not dropped); revision-scoped association tables (migration 000112, FK'd to enforcement records, sanctions table untouched) with sqlc ports; pure deterministic revalidator flagging unbind on retirement/succession/suspension and review-only rebound, restart-safe, no Directory writes, no fuzzy binding. Five unit tests plus PostGIS staging/withdrawal/Directory-untouched integration PASS; quality module, vet, staticcheck, sqlc vet/diff green. No publication/reads/HTTP, no activation.

- MEX-20f 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent91d86a05. [Whole-source acceptance](../backend/evidence/mobile-expansion/MEX-20f.md), [aggregate](../backend/evidence/mobile-expansion/MEX-20f-acceptance.json): pinned244247 rows through image Rust→Go→worker→PostGIS, native replay and private full-file correction; every public field on two2443-page frozen traversals matches. Fresh independent/native census and whole-artifact privacy PASS; race203.7s and final valid-XML fixture54.4s, targeted runtime/static/secrets PASS. Four authorized units MEX-20c/d/e/f complete; source flags OFF and admission/production/Android obligations remain OWED. dev cumulative PR144 includes20b–f; next delivery current-head remote checks/guarded integration/wiki, no MEX-21 implementation in this batch.

- MEX-20e 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parente10af8bc. [Bounded native worker](../backend/evidence/mobile-expansion/MEX-20e.md): exact HTTPS resource/listing, existing source lease/private fetch, cleanup-before-publication,200MiB writer/bridge ceiling,120s runner, independent default-off flags and reserved queue kinds. Actual Rust/image-preparer→worker→PostGIS correction/replay/ack-loss/cleanup/dead-budget proofs PASS; affected race matrix, Rust170/fmt/clippy, static/vet and nonroot static image smoke PASS. Next MEX-20f full pinned source; no activation.

- MEX-20d 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent2206b31e. [Atomic native publication/frozen reads](../backend/evidence/mobile-expansion/MEX-20d.md): shared publisher with optional context identity, bounded128-row staging, four-writer convergence, replay/correction/late rollback, global keyset/detail and actual HTTP/OpenAPI/privacy proofs. Extraction unknown, document dates separate; no activation. Affected race/PostGIS, vet/static/sqlc/OpenAPI/diff/secrets PASS. Next MEX-20e.

- MEX-20c 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent2dffb82e. [Compatible native storage](../backend/evidence/mobile-expansion/MEX-20c.md): append-only migration000111, revision-scoped source context/minimized facts, content constraints and locked bounded staging; legacy hashes/replay retained. Actual PostGIS upgrade/rollback/concurrent writers/withdrawal barrier and admission/count regression PASS. User authorized next four ready units MEX-20c/d/e/f; next MEX-20d, no intermediate delivery waits or activation. PR143 already merged through20a; do not reuse it.

- MEX-20b 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent6a04d1ac. [Verified typed enforcement artifacts](../backend/evidence/mobile-expansion/MEX-20b.md): Go typed boundary in quality/prepared reusing the bridge pattern, no new package; strict versioned manifest (schema plus pinned anp-enforcement-xlsx-2019-v1 profile), bounded stream bridge (200 MB output budget), per-record exact validation with recomputed record_hash/tuple_hash, locator census, repeat/twin accounting, full census recomputation and canonical enforcement-source IDs; no station binding, persistence or publication. Native contract fixed in-commit (source_profile plus tuple_hash, additive). Five synthetic Go tests plus Rust→Go round-trip (synthetic XLSX, CI) and full-source oracle (244,247 records, census match, 42 s, local) PASS; quality module, vet, staticcheck, 168 Rust tests, fmt/clippy green; prepare_enforcement added to CI with its test path. No storage/publication/worker/acceptance, no source admission.

- Batch delivery rule 2026-10-22 (user-directed): one cumulative dev → main
  PR (#143) carries every microphase of the authorized batch. Each microphase
  is one atomic dev commit pushed normally; the PR stays open and green, with
  no intermediate merge and no bookkeeping-only commit. The guarded merge, the
  single wiki mirror and the final state happen once, from the really merged
  SHA, after the last authorized microphase of the batch (MEX-20a) is finished.
  A moving main is merged into dev, never rebased, and revalidated.

- MEX-20a 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent0ff7010f. [Actual enforcement workbook reader](../backend/evidence/mobile-expansion/MEX-20a.md): dependency-free native XLSX reader in tools/station-prep (raw DEFLATE, bounded XML pull parser refusing DTD/entities, bounded ZIP with CRC32 and expansion budgets, workbook profile, shared strings, streaming rows); pinned official enforcement profile (2019+, eleven exact headers, workbook date system as the only clock); minimized source records keeping distinct results that share a document, admitting only a valid CNPJ and never emitting personal identifiers, agent names, neighbourhoods or addresses; private prepare_enforcement CLI (0700/0600 artifacts, counts-only stdout, deterministic repeats). Real-file acceptance against the independent Python census agrees on every aggregate: 244,247 rows, 141,290 documents, 180,661 subject/date/document/procedure tuples, 495 result labels, 116 maximum result characters, 32,822 results beyond 64 characters, 37 segments with equal counts, subject shapes 242,948 company/1,294 personal/5 other, 18 members, 127,781,352 expanded bytes; 0 personal identifiers in the artifacts; 168 Rust tests, fmt/clippy green, no new dependency; 4.8-5.1 s and 67.8 MB peak resident set, byte-identical repeat artifacts. No Go acceptance (MEX-20b), no storage/publication/worker/whole-source acceptance, no source admission or activation.

- MEX-29 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parenteb168dbe. [Public price ranking](../backend/evidence/mobile-expansion/MEX-29.md): `GET /v1/prices/ranking` typed ANP weekly OBSERVATION_PRICE/AREA_MEAN for COUNTRY/STATE/MUNICIPALITY; source-revision-bound cursor/ETag, visible interpretation/source/coverage, exact means/variation; bounded in-process cache (entries/items/TTL, revision in key) with `ANPFUEL_PRICE_RANKING_CACHE_ENTRIES=0` uncached and `ANPFUEL_PRICE_RANKING_ENABLED=false` disable recovery; outage503 no-store, stale ETag/cursor after revision switch refused. 10 unit tests (cold/warm SQL/bytes, revision, outage, cursor binding/expiry, cancellation, cache bounds) + real HTTP/PostGIS OpenAPI/concurrency/revision test PASS; 11-package race matrix, vet/static/sqlc/diff/gofmt/secrets and OpenAPI zero errors (414 existing warnings) PASS. No authorized dependency-ready unit remains: MEX-20a retained, MEX-30 blocked by MEX-23, EE-04/05 source-gated.

- MEX-28 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentd64eb98b. [Typed price ranking](../backend/evidence/mobile-expansion/MEX-28.md): server-side ANP weekly OBSERVATION_PRICE/AREA_MEAN for COUNTRY/STATE/MUNICIPALITY, exact rational means and max-min variation, stable station/location ties, latest observation per station and full-population coverage (raw observations, stations with price, source stations, unlocated). Additive covering index `official_station_prices_ranking_idx`; current-pointer correction visible without aggregate writes, non-published pointer unavailable. Community source/other conditions and HTTP/cursor/ETag refused/deferred to MEX-29. Application6 tests +10 refusals and PostGIS2 tests (hand-derived means, correction/no-write, missing product coverage, index plan with20k rows/1000 stations after VACUUM) PASS; full official+apicontract race9 packages, vet/static/sqlc/diff/gofmt/secrets PASS. Next MEX-29 public endpoint.

- MEX-19 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentf0b8b1a9. [Scoped sanctions HTTP](../backend/evidence/mobile-expansion/MEX-19.md): optional station_id page/detail is exact current-company-only with no global fallback; fresh revision/station/Directory proof before304; cursor/ETag seal revision, station, filters, limit and private fingerprint; independent default-off `ANPFUEL_QUALITY_COMPANY_READS_ENABLED`; global/source routes unchanged. Five pre-fix filter assertions RED; targeted HTTP/PostGIS, final12-package race matrix, root loopback factory, config/API/OpenAPI (zero errors, existing392 warnings/103 informs), vet/static/sqlc, gofmt/diff/secrets PASS. Final regressions: stale detail ETag after Directory change404 (never304), foreign-stream revision404, empty NO_CURRENT_COMPANY_MATCH scope explicit with no fallback. Four-unit local batch MEX-10/17/18/19 complete; PR140/Go/wiki already integrated separately. Next: push dev, protected batch PR, current-head checks and guarded merge; no MEX-20/EE-04–10.

- MEX-18 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parentfa7632f4. [Scoped reader acceptance](../backend/evidence/mobile-expansion/MEX-18.md): four RED cancellation/withdrawal defects fixed; revision validity held through reads. Eight observed PostGIS Directory writer page/detail commit/cancel cases, six historical profiles, empty/legacy/outage/census negatives and exact indexed plans with20000 unrelated rows PASS. Full quality/Directory race9 packages, vet/static/sqlc/diff/secrets PASS. No source/HTTP activation; next MEX-19.

- MEX-17 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent6e679820. [Source/census freeze](../backend/evidence/mobile-expansion/MEX-17.md): fresh official listings and nine whole selected resources; native+independent fine oracles7/7, current enforcement244247 rows and PMQC66225 assays independently counted. Enforcement new hash held for MEX-20; unknown segments, nonunique case keys, seven blank PMQC results and missing/repeated month links explicit. Five oracle tests/repeat artifacts/diff/secrets PASS; no source activation. Next MEX-18 then19.

- MEX-10 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING, parent597f5e95. [Ledger/report reacceptance](../backend/evidence/mobile-expansion/MEX-10.md): seven real RED assertions closed, exact category sums, erase/stale refusal, gap/reset/missing-odo evidence and revision fingerprint independent of clocks. Owner-lock correction/commit/cancellation observed on PostGIS; complete bounded comparison replaces broken public-page wiring. Final ledger/metrics/cloudsync/private signed journey/API race matrix, vet/static/sqlc/contracts/diff/secrets PASS. No migration/dependency/activation; next MEX-17, then18/19.

- Expansion/main reconciliation 2026-10-22: LOCAL_DONE / INTEGRATION_PENDING. [Evidence](../backend/evidence/mobile-expansion/MAIN_RECONCILIATION_20261022.md): audited expansion retained on maintained dev, canonical Room17 preserved; additive prefix index moved to18. Physical 16→17→18 and17→18 retention/reopen/index tests2/2 PASS; combined JVM638/390/465(two existing skips)/400, APKs/lint, Go affected race/contracts/root compilation, SQLite oracle/diff/secrets PASS. Next authorized MEX-10 then17/18/19; blocked source gates unchanged.

- PR140 integrated 2026-10-22 as `2480d2cc`, all four current-head remote checks passed; guarded finish ran local Quick once, dev retained/synchronized. Original commits/review fixes/Go1.27.3 correction remain ancestors. Wiki published once from merged SHA as `ac6bf04`; heartbeat PAUSED. Signed staging contribution/full manual/production obligations remain OWED.

- Go1.27.3 security reconciliation 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent550c5e12. [Audit](../backend/GO1273_SECURITY_REVALIDATION.md): latest stable official Go/CI/Docker/smoke pins aligned, manifest digest verified, govulncheck clean and stale-runtime refusal tested (security11/11). Full backend unit race/vet/static/sqlc/OpenAPI PASS; tagged real PostGIS/S3 matrix112 packages PASS plus worker PASS after correcting mode775 native-fixture prerequisites to755. Four explicit opt-in proofs remain separate. Four nonroot images built and actual migrate/API live/ready PASS; no deployment. App inputs unchanged. Next publish existing PR140 head and require current-head remote checks/guarded finish/wiki; heartbeat stays paused during the requested audit.

- OPT feed independent delivery review 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, reconciled submitted `8bb652fa` with dev `a2f8925c` at `438833a1`; protected dev → main PR #140 reused. [Corrections and acceptance](../mobile/opt-home-community-feed-review-20261021.md): ten reproduced defects corrected (delay/lifecycle/retention/retry/pagination/cancellation/corrupt snapshot and reconnect cursor state); concurrency and permanent 300 s call-count scenarios PASS. Full JVM 1,777 cases (two pre-existing opt-in skips), build/lint zero errors, physical Room upgrade/reopen 1 + Compose 7 + prior Room/migration 12 PASS; HTTP contract, Directory fault race/PostGIS ×3, public staging empty reads, diff/secret review PASS. Next: publish validated head to dev, current-head remote checks, guarded finish and one wiki mirror. Contribution staging E2E/full manual/production measurements remain OWED.

- OPT home/community loading Phase 1 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING on `codex/opt-home-community-feed` from dev `fb85d4d1`. [Inventory](../mobile/opt-home-community-feed-phase1.md). Named efficiency/consolidation plans absent; contract anchored on actual source. Foreground 15 s loops in Community + Home-community, `no-store`, advisory `refresh_after_seconds:15`, no ETag/304, no Room community snapshot. Baseline JVM community+home and Go feed/summary green; no latency claim.
- OPT home/community loading Phase 2 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING. [Evidence](../mobile/opt-home-community-feed-phase2.md). `feed_auto_refresh` renamed to `feed_standard_prices` in 8 locales, interval text removed, refresh button/a11y kept. Assemble, community JVM tests, lintDebug (0 errors), diff-check and secret review green.
- OPT home/community loading Phase 3 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING. [Evidence](../mobile/opt-home-community-feed-phase3.md). Verified `no-store`, no ETag/304, advisory hint only; no backend change. Added Room v17 `community_feed_cache` (scope-keyed first-page/summary snapshots, original dates, expiry filtered at read, 64-entry/7-day bounds, cursor pages excluded). 18 new tests green; domain/app/data suites, assembleDebug, lintDebug (0 errors), diff-check and secret review green. Device reopen pending.
- OPT home/community loading Phase 4 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING. [Evidence](../mobile/opt-home-community-feed-phase4.md). Local-first paint (snapshot, else retained content, never blanked); fixed 15 s polling removed from both surfaces; central `CommunityRefreshPolicy` (60 s stale window candidate, doubling backoff, 5-retry budget, 30 s local sweep, 5 s loop floor) evaluated on open/return, scope change, reconnect and manual refresh; no off-screen/offline polling; Home failure now retains validated prices; pagination preservation unchanged. Fixed a real zero-delay busy-loop found during testing. 14 new tests green; domain 549 + app 95 + data 23 green; assembleDebug, lintDebug (0 errors), diff-check and secret review green. Device rendering/reopen pending.
- OPT home/community loading Phase 5 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING. [Evidence](../mobile/opt-home-community-feed-phase5.md). Acceptance matrix (open with/without cache, offline reopen, slow/failure retention, valid-empty, expiry, fast scope switch, simultaneous triggers, exit/return, reconnect, manual, pagination, contribution-via-return, isolation) green on JVM. Final tree: domain 549, data 448, app community/home/stations 95, assembleDebug, lintDebug (0 errors), backend http contract, diff-check and secret review green; backend untouched. Measured 5-min foreground feed: 21→6 calls fresh (−71%), 21→0 offline, 21→5 outage-then-silence; first paint with snapshot needs 0 network calls. No latency/battery/capacity claims. Device rendering/reopen, staging E2E and production measurement remain pending. Scope implemented, validated and committed locally; other fronts preserved.
- EE-11 independent audit + existing domain boundaries 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parentd9b31fe4. [Review](../backend/evidence/ev-support/EE-11-REVIEW.md) repaired real Rust intermediate overflow/calendar/nanosecond defects and Go/Kotlin UUID/Unicode/unused-quantity validation;53 shared vectors PASS across Go/Kotlin/Rust, no dependency/schema/provider activation. [Domain audit](../backend/evidence/mobile-expansion/DOMAIN_AUDIT_20261021.md) closes the existing90% coverage gate at90.0607% through meaningful money/privacy/source/OCR/fallback tests; threshold/exclusions unchanged. Full JVM632/390/450(two pre-existing skips)/375, APKs/lint, Go race/contracts/vet/static/Go1.27.3 govuln zero findings, Rust test/fmt/clippy and diff/secrets PASS. Separate dev security commit3c80c1ab updates Go1.27.3 pins for PR140; source history reconciliation remains pending. Newly authorized four-unit batch: MEX-10/17/18/19 subject to actual prerequisites; MEX-08/EE-04–10 remain blocked on their owning gates.

- EE-11 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent2afc77bf. [Tariff engine](../backend/evidence/ev-support/EE-11.md):
  pure exact tariff scenario calculation in Go/Kotlin + Rust parity over 42
  shared synthetic vectors: only INCLUDED/complete/no-min-max/final-half-up
  yields KNOWN centavos; absent energy, unknown fees, eligibility mismatch,
  partial validity windows and overflow are explicit refusals/UNKNOWN. Go
  race/vet/static PASS; domain 615 tests zero failures (charging 100%
  coverage; repo-wide 0.90 JaCoCo gate remains pre-existing failing for
  unrelated packages); Rust lib+integration, fmt/clippy PASS; APK compile/
  assemble, diff/secret PASS. No persistence/API/provider activation.
  EE-04–10 blocked on admitted sources; next ready MEX-10 (needs EE-11+MEX-09).

- MEX-09 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parentb71f2732. [Exact financial/wire parity](../backend/evidence/mobile-expansion/MEX-09.md): repaired both full anchors, excluded-anchor fuel/unit checks, negative matched-window money and overflow in existing Go/Kotlin engines; checked sums and exact rational intermediates. Actual RED→GREEN, 36 shared execution vectors covering all15 frozen examples plus22 raw-value vectors, Go race/API/contracts/vet/static/build PASS. JVM domain608/application390/data450 (two existing skips)/app375, APKs and lint zero errors PASS; diff/secret checks PASS. No new API/dependency/schema or publication; retained raw/estimate/UNKNOWN distinctions. Two-unit request MEX-07/09 complete locally. MEX-08 needs EE-19; next ready MEX-10 or independent EE-04 needs new authorization. Global Go vulnerability integration gate remains separate.

- MEX-07 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent9f05bdc1. [Hosted vehicle admission](../backend/evidence/mobile-expansion/MEX-07.md): live vehicles.multiple and central atomic 1-free/3-ceiling rule reused in cloud creation/import/restore; final proof recheck, retained legacy IDs and honest previews. Three RED defects closed; actual PostGIS race/rollback/retry and full cloud/subscription/API/contract matrix PASS, vet/build/static/OpenAPI/diff/secret checks PASS. MEX-06 location13 independently re-executed PASS. No schema/dependency/activation change. Global Go vulnerability CI remains blocked separately. MEX-08 needs EE-19; next ready MEX-09.

- MEX-06 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent3c6a9878. [Evidence](../backend/evidence/mobile-expansion/MEX-06.md):
  fast picker UI: generation fencing on catalog loads (obsolete responses
  cannot land or error), memoized composition filters over a precomputed
  normalized index, measured zero-debounce policy (indexed keystroke p95
  0.35ms vs 5.13ms before, host JVM). app 375 tests incl. 5 new ViewModel
  fencing/retention cases — zero failures; assemble/lint/diff/secrets
  PASS. Native keystroke timing stays OWED. Next MEX-07.

- MEX-05 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent431b935a. [Evidence](../backend/evidence/mobile-expansion/MEX-05.md):
  bound typo search, retired per-keystroke findAll: one indexed
  normalized-prefix range per stem (migration 16→17 additive, schema 17),
  normalized FTS tokens, ranking on Default dispatcher with cancellation.
  Documented exact/accent/typo probes keep visible top results; mid-token
  substring retrieval retired (rule unchanged). Real-SQLite plan/count
  oracle + asset parity benchmark PASS. data 449 (2 pre-existing skips),
  application 390, domain 547, app 367 — zero failures; assemble/lint/
  diff-check/secrets PASS. Next MEX-06 fast state/city UI.

- MEX-04 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parentd23aebc1. [Evidence](../backend/evidence/mobile-expansion/MEX-04.md):
  batch place lookup/availability: discovery reads without mandatory imported
  week (unavailable flags, never error), one grouped availability query + one
  indexed state read per involved state, per-result N+1 eliminated with
  identical accent/unknown-state semantics. Real-SQLite oracle PASS;
  data 440 (2 pre-existing skips), application 390, domain 547, app 367 —
  zero failures; assemble/lint/diff-check/secrets PASS. Selection write
  still requires a week (week-bound event). Next MEX-05 typo search.

- EE-03 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent49b8be2d. [Evidence](../backend/evidence/ev-support/EE-03.md):
  versioned preparation/census envelope frozen (charging-prepared-v1,
  vehicle-energy-reference-v1): pinned OCM-candidate/PBE-held/synthetic
  profiles, 4 planned schemas + 5 payloads, byte/identity goldens, 14-unit
  hand-counted synthetic census (6 accepted/5 duplicates/3 quarantined).
  Python oracle 17/17, Rust lib 12 + vectors 1 + prepared 2, Go apicontract
  race, fmt/clippy/gofmt/vet/staticcheck and docs/secrets PASS. Admission
  stays false everywhere (OCM candidate, PBE rights/layout pending);
  OCM Quantity is reported group count, never physical EVSE proof.
  Next EE-04 OCM preparation or independent MEX-04.

- EE-02 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent22adc7b8. [Evidence](../backend/evidence/ev-support/EE-02.md):
  typed vehicle/propulsion/source/place-EVSE-connector/tariff/private-session/port
  contract,15 planned components (zero paths),46 shape examples and24 exact shared
  CSV examples PASS in Go/Kotlin/Rust. Full apicontract race/vet/static, JVM test/
  compile, Rust test/fmt/clippy and document/secrets PASS. No runtime engine,
  migration or app/source/sales activation. Four requested units complete:
  MEX-03,EE-00/01/02; EE-01 is qualification only, PBE rights/source admission
  pending. [Handoff](MOBILE_EXPANSION_CONTINUATION.md): EE-03 or independent MEX-04.

- EE-01 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING as qualification only,
  parent83897983. [Evidence/register](../backend/evidence/ev-support/EE-01.md):
  three real commit-pinned Brazilian OCM samples/reference and actual9-page PBE
  bytes/hash/profile verified; seven privacy/failure tests PASS. Only minimized
  metadata committed. Provider1 reports CC-BY4.0; source admission remains OFF,
  PBE RIGHTS_PENDING and full corpus/privacy/provider gates owed. Next EE-02,
  independently; EE-03/source activation cannot infer approval from this checkpoint.

- EE-00 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent42e4efe7. [Inventory](../backend/evidence/ev-support/EE-00.md)
  pins12 source files/eight ancestor checkpoints, actual liquid-only vehicle/Room16/
  hosted model and PRM/FQ/community/Rust owners. No accepted EV source/parser/session
  implementation found; no runtime mutation. Hash/ancestry/docs checks PASS.
  Next authorized EE-01 qualification and EE-02 contracts; rights/runtime stay separate.

- MEX-03 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent5cb8e660 on
  codex/mobile-expansion-plan. [Evidence](../backend/evidence/mobile-expansion/MEX-03.md):
  pinned5,571-city SQLite/actual JVM fallback and dense synthetic model baselines,
  structural budgets and actual picker-versus-unused-search distinction. Host
  probes/JVM compile/test PASS; native UI/network/PostGIS measurements OWED.
  Next authorized units EE-00/01/02; no runtime optimization or publication.

- Mobile expansion MEX-02 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent
  d61ccc4c on codex/mobile-expansion-plan. [Evidence](../backend/evidence/mobile-expansion/MEX-02.md):
  typed exploration/official-source/company-site/filter/cache/Android-copy contract,
  20 planned schemas (zero live paths), 54 shape and three counting examples.
  Full apicontract race (including MEX-01/live goldens), vet/static/docs/secrets PASS.
  No runtime parser/query/UI or source activation claim. Requested two microtasks
  complete; [continuation](MOBILE_EXPANSION_CONTINUATION.md) starts at MEX-03,
  or independently EE-00. No publication/device/deploy; other work preserved.

- Mobile expansion MEX-01 2026-10-21: LOCAL_DONE / INTEGRATION_PENDING, parent
  0e7af9d5 on codex/mobile-expansion-plan. [Evidence](../backend/evidence/mobile-expansion/MEX-01.md):
  registration v2/machine amendment, retained legacy data, planned vehicles.multiple,
  unchanged ceiling3 and professional capability matrix; 23 contract examples,
  full apicontract/subscription domain race, vet/build/static/document checks PASS.
  Runtime quotas/capability grants remain MEX-07/08 pending. Next MEX-02 only;
  no publication, native run or commercial/source activation.

- Mobile expansion MEX-00 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING on isolated
  codex/mobile-expansion-plan. [Foundation audit](../backend/evidence/mobile-expansion/MEX-00.md)
  reconciles planning 35accb9e with committed dev a2f8925c, preserving Home and
  foreign work. Ownership, 30 PRM evidence base pins, app-first/FQ/OPT gaps,
  source defaults and tools inventoried; documentation checks PASS. Separate OPT
  copy/cache commits exist but are not blindly accepted (no-store/SQLite gates).
  Remote freshness unverified (DNS); no new runtime behavior or publication.
  Stopped after the requested first microtask. [Handoff](MOBILE_EXPANSION_CONTINUATION.md):
  next MEX-01, or independently EE-00; both still unexecuted.

- Full EV extension EE-PLAN 2026-10-20: PLANNING_LOCAL_DONE / INTEGRATION_PENDING
  on isolated codex/mobile-expansion-plan, parent 47fded8c. [Research](EV_SOURCE_RESEARCH.md)
  records 25 source/data families (23 external, two first-party), actual access/
  rights/coverage limits and no presumed national live feed. [EE plan](EV_FULL_SUPPORT_EXECUTION_PLAN.md),
  [rules](../product/EV_SUPPORT_CONTRACT.md), [financial method](../product/EV_FINANCIAL_METHOD.md),
  [combined prompt](EV_SUPPORT_EXECUTION_PROMPT.md), [validation](EV_SUPPORT_PLAN_VALIDATION.md).
  Adds 32 EE units and an acyclic MEX splice (77 combined execution units,
  conditional live-provider access separate), preserving existing IDs/data/free
  boundaries. Includes BEV/PHEV/HEV, Rust source prep, community tariffs, local
  Room processing and private electric ledger/report/sync. No implementation,
  provider/account/contact/purchase, source activation, native run or publication;
  D/iOS/charging-control/payment remain outside scope. Next: submit combined prompt
  and reconcile current accepted MEX/EE prerequisites before schema/UI work.

- Mobile expansion MEX-PLAN 2026-10-20: PLANNING_LOCAL_DONE / INTEGRATION_PENDING,
  isolated codex/mobile-expansion-plan from dev fb85d4d1; occupied Home/week-picker/
  manual-sync and primary work preserved. [Plan](MOBILE_EXPANSION_EXECUTION_PLAN.md),
  [contract](../product/MOBILE_EXPANSION_CONTRACT.md),
  [agent prompt](MOBILE_EXPANSION_EXECUTION_PROMPT.md),
  [validation](MOBILE_EXPANSION_PLAN_VALIDATION.md). Forty parent tasks with six
  enforcement children (45 execution units) reuse PRM/FQ/OPT owners. Confirmed:
  one free vehicle, additional vehicles Premium, retained legacy data; existing
  technical ceiling and professional capability matrix preserved. Includes fast
  local places, financial/cloud Android, national price exploration and separate
  source-global/current-company/site ANP occurrence filters. Implementation and
  remote/runtime/provider/native acceptance remain pending; no publication or D
  override. Next implementation candidate MEX-00 after human submits the prompt.

- Fuel quality FQ-07R6E1 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING, parent c9ffbbf6. [Lease-bound current-company read ports](../backend/evidence/fuel-quality/FQ07R6E1_SCOPED_COMPANY_READS.md): frozen lease/current-company scope sealed in fingerprint without CNPJs; page/detail derive bindings and hold Directory guard through read Tx, retire/successor/suspend between scope and rows fails closed with no leaked rows; source-global history stays reachable. Real PostGIS retire/successor/suspend plus global accessibility PASS; 14 read tests and company adapter matrix PASS. No HTTP/OpenAPI/activation/deployment/D claim. Next FQ-07R6E2.

- P35/quality final delivery 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING in the independent `mobile-pagination-integration` checkout, private maintained `dev`; PR138 reused. Captured mobile `d0918d03`, committed quality `9737b22f`, main `08a54dc4`, preserving both histories and the occupied quality/evidence drafts. [Reconciliation and CI repairs](mobile-pagination-main-delivery-20261020.md): Docker filter classifier RED→GREEN; deterministic alert schedule fixtures including Sunday/window refusal; historic index-upgrade boundary retained; real-PostGIS snapshot upload barrier reproduced duplicate admission, fixed with an owner session lease and pool-one/cancel/retry acceptance. Combined109 Go acceptance covers113 packages (112 full-matrix passes plus corrected Caddy/race rerun); four optional cases skipped, no known failure remains. Classifier/SQLite checks PASS. Final current-head CI/guarded finish pending; no merge/release claim.

- P35 pagination/sync ADB follow-up 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING, parent `46bdf737`, branch `codex/mobile-pagination-review`. [Native evidence](../mobile/pagination-sync-device-20261020.md):38 actual native cases PASS/no skips (Room/SQL/worker27, UI/touch8, staging/network3); fixed invalid/outdated test fixtures and isolated the weekly action without changing its default. Persistent125-row offline traversal and5,000-row virtualization PASS; actual-touch debug FrameMetrics828 frames, p50=9.3ms/p95=16.8ms (diagnostic, not release certification). Installed APK hash verified; no package-data clear/uninstall, network1/1 restored. Newer dev reconciliation/protected delivery and whole-app/release acceptance remain pending.

- P35 pagination/sync independent review 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING on `codex/mobile-pagination-review`, tested parents `cb10cc5b` + `28a3de0a`. [Corrections and acceptance](../mobile/pagination-sync-review-20261019.md): preserved quality Room13→14→15, appended16, repaired query/polling/cursor races, origin-scoped bounded cache, resumable opt-in sync with atomic preference fencing, and other collection continuations. JVM1,726 executed/two existing opt-in skips, actual SQLite12 cases, four race/PostGIS packages, lint/build/static and app/data test-APK compilation PASS. Native execution/frame-time/staging OWED; newer dev `08bef884` reconciliation and protected delivery pending. Occupied checkouts preserved; no publication/deployment/release claim.

- Fuel quality FQ-07R6D 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING, parent5c60d5e6. [Directory worker composition](../backend/evidence/fuel-quality/FQ07R6D_COMPANY_WORKER_COMPOSITION.md): fresh512-CNPJ evidence/branch batches and typed live guard in the actual fine worker, fail-closed complete plans, cleanup-before-publish and no identity writes. Existing index serves512-prefix EXPLAIN on20,001 retained rows; stale Directory upgrade suffix corrected to109. Actual worker binding/correction/retirement/ack-loss tests PASS; complete seven-file worker/replay/traversal PASS74.313s with35,612 explicit unmatched decisions in empty Directory and zero identity writes.12-package race/PostGIS/contracts/static/build/sqlc PASS. No activation/VPS apply/public station attribution. Next FQ-07R6E scoped live/frozen reads, revalidation, FQ-06/Android/FQ-16/17/protected delivery. D held.

- Fuel quality FQ-07R6C 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING, parent9737b22f. [Atomic company publication](../backend/evidence/fuel-quality/FQ07R6C_COMPANY_ASSOCIATION_PUBLICATION.md): complete typed decision census, consistent exact company verdict, optional sorted identity preserves all old keys and excludes operational day; actual derived bindings require typed Directory transaction guard before bound publication/replay. Same transaction stages source/context/decisions and checks full census. Seven profiles/empty/replay/correction/frozen history, mismatched station/retirement, eight-publisher convergence, later-row/census-corruption rollback PASS;11-package race/PostGIS/contracts + actual Directory guard concurrency/static/build PASS. No runtime producer/public station association/deploy. Next FQ-07R6D Directory worker composition, then scoped live reads/revalidation, FQ-06/Android/FQ-16/17/protected delivery. D held.
- Fuel quality FQ-07R6B 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING, parent91ad38c0. [Scoped company ledger](../backend/evidence/fuel-quality/FQ07R6B_COMPANY_ASSOCIATION_STORAGE.md): append109 separates current-company-v1 context/decisions from dated PMQC links, strict scope/unknown-business-window/calendar/kind/reason/station/same-source-event FKs and locked staging. Real108→109/replay preserves15 frozen table hashes, seven profiles/empty/census,15 negative cases, eight-writer convergence, two observed withdrawal barriers and whole-stage rollback PASS. Ten-package race/PostGIS/migrate/HTTP contract matrix + build/static/sqlc PASS. No VPS apply/runtime/public association. Next FQ-07R6C atomic scoped publication/live guards, then worker/read/revalidation, FQ-06/Android/FQ-16/17/protected delivery. D held.

- Fuel quality FQ-07R6A 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING, parent3a0d6be4. [Current-company policy](../backend/evidence/fuel-quality/FQ07R6A_COMPANY_ASSOCIATION_POLICY.md): explicit current-company-v1/CURRENT_COMPANY_ONLY, unknown historical business window, no incident/due/extraction date input; exact current active full identity only, ambiguous/retired/branch/missing remain unassociated. Bounded512-CNPJ application planning preserves separate observations, validates all source/evidence and returns no partial plan on failure/cancellation. Six new test functions plus PMQC/domain-architecture/kernel regression/race/static/build PASS. No SQL/runtime/public association yet. Next FQ-07R6B scoped compatible ledger/publication, then live fences/reads/revalidation, FQ-06/Android/FQ-16/17/protected delivery. D held.

- Fuel quality FQ-07R5E 2026-10-20: LOCAL_DONE / INTEGRATION_PENDING, parent08bef884. [Seven-resource worker](../backend/evidence/fuel-quality/FQ07R5E_HISTORICAL_FINE_WORKER.md): exact grouped/annual/misnamed-CSV profiles, trusted history runner, typed file-job dispatch/common cleanup-before-publication, bounded cursor, dead-budget preservation and static nonroot image/CI. Real worker correction/replay/ack-loss tests PASS; whole seven-source worker/full traversal PASS35,911=35,612accepted+293CPF+6metadata, zero links; live HTTPS/listing2021+2025 and uid65532/network-none image PASS. Eleven-package race/PostGIS/static/build PASS. No activation/deploy. Next safe Directory links, FQ-06 actual XLSX, linked Android/FQ-16/17/protected delivery. D held.

- Fuel quality FQ-07R5D 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parent73312985. [Atomic historical publication/public contracts](../backend/evidence/fuel-quality/FQ07R5D_HISTORICAL_FINE_PUBLICATION.md): exact typed admission, frozen optional context key, one transaction for context/events/notes, replay/correction/current/live guards; accurate source DAY/MONTH, references, null paid, notes/metadata counters on anonymous HTTP/OpenAPI/ETag/keysets. Eight publishers per six profiles, failed row/note rollback, empty/late history and actual HTTP/privacy/cache tests PASS. Whole seven-resource Rust→Go→PostGIS/replay/full traversal PASS35,911records=35,612accepted+293CPF+6metadata (no duplicates); all source fields preserved, zero inferred links. Final ten-package race matrix/static/build/sqlc PASS. Next FQ-07R5E bounded history runner/discovery/image, then safe links/FQ-06/Android/FQ-16/17/protected delivery. No activation/deployment; D held.

- Fuel quality FQ-07R5C 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parenta59c61d1. [Compatible historical fine storage](../backend/evidence/fuel-quality/FQ07R5C_HISTORICAL_FINE_STORAGE.md): additive108 preserves legacy/2025 facts/pointers and adds exact profile/clock/metadata context, conditional event FK, real references/transit/unlabelled fields and allowlisted source notes; exact currency/nullable-paid/calendar and original-locator guards. Real107→108/replay, six profiles, negative money/date/NULL/notes, eight-writer convergence and three observed withdrawal lock barriers PASS. Fixed real dual-unique-index native retry race without weakening test; ID-collision publication rollback PASS. Ten-package affected race/PostGIS matrix + migration/contracts/static/build/sqlc stability PASS. No VPS apply/runtime historical publisher. Next FQ-07R5D atomic publication/read contracts, then history worker/safe links, FQ-06/Android/FQ-16/17/protected delivery. D held.

- Fuel quality FQ-07R5B 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parent06526993. [Verified historical fine bridge](../backend/evidence/fuel-quality/FQ07R5B_HISTORICAL_FINE_BRIDGE.md): exact six-profile manifest/clock/source and four-stream integrity, canonical minimized identities, duplicate twins/full original census and safe metadata/footer ordering; no partial batch on any failure. Application semantics preserve unknown paid/department and actual references. Actual Rust→Go→application whole-source race PASS32,516records (32,237accepted/273CPF/6metadata);30tamper, pre-I/O budgets, late cancellation/read/close and resource leak RED→GREEN PASS; native canonical separator parity fixed; full Rust132/static/build PASS. No SQL/runtime activation. Next FQ-07R5C compatible ledger with real PostGIS, then publication/history worker/safe links, FQ-06/Android/FQ-16/17/protected delivery. D held.

- Fuel quality FQ-07R5A 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parent74051a36. [Historical native fine preparer](../backend/evidence/fuel-quality/FQ07R5A_HISTORICAL_FINE_PREPARER.md): six verified source profiles, actual reference labels/transit/unknown-paid/unlabelled facts and group DAY versus annual MONTH;2021 misnamed .xlsx is UTF-8 BOM CSV. Independent Python whole-file/hash/census oracle PASS for32,516records=32,237accepted+0duplicates+273CPF+6metadata, including1,303unknown paid preserved NULL. Fixed unjustified empty-department rejection (159records) and truncated quoted-EOF framing via RED→GREEN. Full Rust131/clippy/fmt/privacy/CLI checks PASS. Separate typed contract not yet Go/SQL/worker registered. Next FQ-07R5B verified history bridge, then compatible ledger/history worker/safe links, live FQ-06/Android/FQ-16/17/protected delivery. No activation/deployment; D held.

- Fuel quality FQ-07R4B 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parenta1cce81e. [Native fine worker/discovery/image](../backend/evidence/fuel-quality/FQ07R4B_NATIVE_FINE_WORKER.md): reserved default-off serial claims and independent default-off schedule, guarded HTTPS/private artifact inspection, one actual annual CSV href per persisted serialized cursor step, correction enqueue and dead-attempt preservation. Actual SQL concurrency/failure/disabled claims and worker→Rust→PostGIS correction/history PASS; five-package affected race matrix/static/build PASS. Static worker image ran as uid65532 with networknone; manual real2025 secure fetch reconciled3,395=3,375+0+20 with cleanup. Five annual CSV hrefs listed; other historical profiles/complete history and station links remain owed. Next FQ-07R5 historical source reconciliation, then live FQ-06/linked Android/FQ-16/17/protected delivery. No activation/deployment; D held.

- Fuel quality FQ-07R4A 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parentf4bed458. [Native fine file/process/job boundary](../backend/evidence/fuel-quality/FQ07R4A_NATIVE_FINE_FILE_IMPORT.md): exact official URL/year, trusted executable/private verified snapshot, bounded process/artifacts, whole-attempt source lease and confirmed raw-source cleanup before typed publication. Actual Rust→PostGIS replay/correction/cleanup/watermark-loss recovery and scoped native/jobs/prepared race matrix PASS; timeout/snapshot mutation and negative payload/PII redaction cases PASS, static/build/YAML checks PASS. CI/reproducer now build/export the required fine CLI. No producer/runtime/image/source activation or station linkage yet. Next FQ-07R4B reserve/compose native worker/fetch/discovery/static image; FQ-06 XLSX, safe links, Android/FQ-17/protected delivery remain owed. D held.

- Fuel quality FQ-07R3B 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parentcb10cc5b. [Atomic native fine publication/read clocks](../backend/evidence/fuel-quality/FQ07R3B_NATIVE_FINE_PUBLICATION.md): strict typed/count/content-identity admission; additive107 freezes source month even for empty revisions; native publication reuses correction/replay/live guards and preserves legacy keys. Public page/detail disclose due/month/raw-paid facts without company IDs; source age cannot be refreshed by local import. Eight-publisher convergence, failed second-stage rollback, historical/empty clocks, actual populated HTTP/OpenAPI/cache/cursor and ten-package critical race matrix PASS; real2025 Rust→Go→application contract PASS (3,395/3,375/0/20), build/static/sqlc/secret checks PASS. Next FQ-07R4 native fetch/discovery/worker and safe Directory association; FQ-06 XLSX, linked Android/FQ-17/protected delivery remain owed. D held; no activation/deployment.

- Fuel quality FQ-07R3A 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parent820af736. [Native fine storage compatibility](../backend/evidence/fuel-quality/FQ07R3A_NATIVE_FINE_STORAGE.md): append-only106 preserves legacy snapshots/pointers and adds source-specific due/month/raw-money/paid/DUF/department fields, exact SQL money checks and locked staging-only native operation. Real105→106/replay,14 invalid cases, eight writers and observed revision-lock withdrawal barrier PASS. Fixed PostgreSQL infinity/year10000 acceptance before final eight-package quality race/PostGIS matrix; migrate/apicontract/static/build/sqlc stability PASS. No runtime native publication or deployment. Next FQ-07R3B typed publisher/explicit clock read contracts, then live streams/linked journeys/FQ-17/protected delivery; D held.

- Fuel quality FQ-07R2 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parent0a5876c8. [Verified typed fine bridge](../backend/evidence/fuel-quality/FQ07R2_VERIFIED_FINE_BRIDGE.md): native manifest/three-stream integrity, exact canonical observation/money/month precision and complete non-overlapping original row census; typed output cannot enter the legacy synthetic publisher. Scoped race/static/build and real Rust→Go2025 source PASS (3,395 input,3,375 accepted,20 CPF quarantine);19 negative tamper cases plus cancellation/ports/close failures PASS. No SQL/publication/source activation. Next FQ-07R3 compatible append-only ledger/read contracts with real-PostGIS acceptance, then live streams/station journeys/FQ-17; D held.

- Fuel quality FQ-07R1 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parent06190644. [Official fine preparer](../backend/evidence/fuel-quality/FQ07R1_OFFICIAL_FINE_PREPARER.md): verified CP850/nine-column SICOM source, exact due dates/money, extraction-month precision and minimized observation identity; private hashed CLI artifacts, no names/CPF. Eight Rust cases plus independent real2025 oracle PASS (3,395=3,375 accepted+0 duplicates+20 CPF quarantine); format/clippy/secret checks PASS. Independent oracle fixed discarded-name control rejection. Real enforcement XLSX inspected; synthetic keys/64-char labels demonstrably insufficient. No Go live bridge/ledger/image/UI/deployment acceptance yet. Next typed Go native artifact bridge and compatible ledger, then live streams/remaining FQ-14/FQ-17; D held.

- Fuel quality FQ-16B2 2026-10-19: LOCAL_DONE / INTEGRATION_PENDING, parent22730930. [Durable dispatch and privacy lifecycle](../backend/evidence/fuel-quality/FQ16B2_DURABLE_DISPATCH.md): owner/family-bound KeyStore queue, fresh signed dispatch, WorkManager, pending-intent restoration and Profile recovery; Room14→15 key receipts and independent restart-safe erasure markers repair Room/key failures. Six actual-SQLite and68 affected JVM cases PASS; feature-off variant PASS, default lint/app/test APK compilation PASS. Eight native quality data/UI cases compiled only, execution OWED. Fixed manual public-read flag bypass; no source/deployment/merge/D claim. Next actual FQ-06/07 source contracts/live streams, remaining FQ-14/FQ-17 and final protected delivery.

- Fuel quality FQ-16B1 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING, parent c973ae93. [Encrypted intent journal](../backend/evidence/fuel-quality/FQ16B1_ENCRYPTED_JOURNAL.md): additive Room13→14, AES/GCM owner-bound command, frozen replay, bounded admission/attempts, claim/clock/expiry fences and durable owner erasure gate. Five actual-SQLite cases (including twelve connections), six JVM cases, lint and app/data-test APK compilation PASS. Three native data cases compiled only, execution OWED. Not yet wired to runtime: next FQ-16B2 key custody, worker/UI restoration and crypto-erasure. Then live streams/remaining FQ-14/FQ-17/protected delivery; D held.

- Fuel quality FQ-16A 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING, parent d50b6b10. [Public offline cache](../backend/evidence/fuel-quality/FQ16A_PUBLIC_OFFLINE_CACHE.md): bounded validated origin/query cache, atomic restart recovery, 304 ETags, stale-only outage fallback, clock/TTL recheck, corruption/eviction and 404/no-store purge. Twenty affected JVM cases, lint and both APK compilations PASS, zero quality lint findings. Cache can be disabled independently; no private persistence/source/device/deployment. Next encrypted owner outbox/erasure FQ-16, then live enforcement/fines and full FQ-14/FQ-17/protected delivery; D held.

- Fuel quality FQ-15B 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING, parent f3a9e89d. [Android private document-free reports](../backend/evidence/fuel-quality/FQ15B_ANDROID_OWNER_REPORTS.md): free login, own bounded history/status, explicit submit/edit/withdraw/appeal confirmation, exact displayed sample linkage-review, localized reasons and deliberate ANP guidance. Seven new plus four existing ViewModel cases, lint and both APK compilations PASS; four native quality UI cases compiled, execution OWED. In-memory uncertain command replay/owner switch/late response protection complete; durable offline intent remains FQ-16. Next FQ-16, then live streams/remaining FQ-14/FQ-17 and protected delivery; D held.

- Fuel quality FQ-15A 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING, parent6c470b15. [Signed document-free owner client](../backend/evidence/fuel-quality/FQ15A_SIGNED_OWNER_CLIENT.md): typed command validation, fresh-session/exact-body proof, stable command replay, origin/account/device pinning and bounded no-store owner reads. Domain RED→GREEN, six client cases, existing proof transport and immediate report race/PostGIS/auth/IDOR/quota/revocation matrix PASS. No private local persistence. Next FQ-15B Android form/recovery, then FQ-16 and remaining source/acceptance work; D held.

- Fuel quality FQ-14B 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING, parent d3702420 on maintained dev. [Android public PMQC](../backend/evidence/fuel-quality/FQ14B_ANDROID_PMQC.md): canonical station/fuel guest journey, dated raw/sample dimensions, complete bounded forward pages, verified history/sample/source details, late-response isolation and independent UI flag. Twelve affected JVM cases, actual HTTP/PostGIS/OpenAPI, lint and app/test APK compilation PASS; native UI execution OWED. Full FQ-14 still owes linked enforcement/sanction journeys; next document-free FQ-15, then FQ-16/17 and remaining live streams. No deployment/merge; D held.

- Fuel quality FQ-14A 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING on maintained dev, parent8c579e4d. [Public sample dimensions](../backend/evidence/fuel-quality/FQ14A_PUBLIC_DIMENSIONS.md). Additive bounded selected-lease/station/fuel dimensions in summary, explicit unavailable projection, numeric score null, ETags include derived results. Real PostGIS basis/fuel/station isolation, actual populated OpenAPI and no-source/disabled/cache regressions PASS. Methodology/policy-aware recompute dedupe permits new calculation without resetting completed jobs; RED→GREEN queue acceptance PASS. Affected race/build/static/SQL checks PASS. Next Android FQ-14, then FQ-15/16/17 and unresolved FQ-06/07 live streams; protected delivery pending, D held.

- Fuel quality independent review 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING, parent901c432c on maintained dev. [Review and corrections](../backend/evidence/fuel-quality/FQ_REVIEW_20261018.md). Five continuation commits independently tested; fixed missing transaction pointer lock, order-dependent Aspecto replicas (methodology v2026-10b), automatic dead-job budget resets, ambiguous input-digest boundaries and station/fuel report counts. Corrected the test oracle full-CNPJ/clock identity and pinned both real fixture hashes/counts: January5,776 rather than5,549; September4,164 unchanged. Twelve affected race/PostGIS packages and actual-data/static/SQL checks PASS. Agent had reached FQ-13 only; FQ-14–17, FQ-06/07 live coverage and final protected delivery remain pending; D held.

- Fuel quality FQ-13 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Deterministic projections and fenced recomputation](../backend/evidence/fuel-quality/FQ13_PROJECTIONS.md). Migration105 stores versioned per-station/fuel dimension rows bound to the frozen basis revision and canonical input digest, `UNSCORED`/NULL score, monotonic generation; `ProjectionStore.RecomputeRevision` judges the current published revision atomically through the current-pointer fence with stale-basis refusal and idempotent digests; publication enqueues one deduped `quality-recompute` job and the reserved handler treats stale bases as completed no-ops. Real-PostGIS matrix: deterministic case dimensions, correction/unknown-basis/stale-refusal, four-way concurrent convergence without forks, report-erasure isolation, and the real publication→enqueue→apply handoff PASS with the full quality race suites, build/staticcheck/sqlc stability, whitespace and secret scan. No API/Android surface yet (FQ-14), no numeric score. Next FQ-14–17 and the final protected dev→main batch; D held.

- Fuel quality FQ-12 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Frozen sample-judgment methodology](../backend/evidence/fuel-quality/FQ12_METHODOLOGY.md). Inspected the October-2026 official PMQC guidance; implemented the versioned assay→sample aggregation with the Aspecto exceptions (ethanol residue/non-volatile; diesel defect/grades, no invented 968/2024 effective day), replica-conflict and unknown-status conservatism, and dimensions-only nullable `UNSCORED` output with no invented weights/thresholds. An independent stdlib-Python oracle ran first; the Go domain reproduced it exactly on both real datasets (September2026 4,164 samples: gasoline1,457/129/0, ethanol1,049/29/1, S101,018/39/3, S500424/15/0; January2025 5,549 samples with the same reconciliation). 21 contract cases, both real-file reconciliations, gofmt and scoped secret checks PASS. No runtime/projection/API change (FQ-13). Next FQ-13–17 and the final protected dev→main batch; D held.

- Fuel quality FQ-05W8 2026-10-18: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Static worker image and explicit schedules](../backend/evidence/fuel-quality/FQ05W8_IMAGE_SCHEDULING.md). Pinned Rust builder (musl static, mode755, chown 65532) ships only `/prepare_quality` into the distroless worker; narrowed `.dockerignore`; real container run prepared a synthetic CSV as nonroot, the migrate image applied chain104, and the worker booted with native handlers and stopped cleanly. Two explicit default-off schedule switches (discovery/revalidation) install bounded daily jobs without activating unrelated ANP/registry imports. FQ-06/07 runtime reconciliation recorded: no prepare CLI, Go source stream, verified live inventory or complete history yet; adapter fixtures are not runtime acceptance. Config/build/static/secret checks PASS; no dependency addition. No deployment/import activation/schedule enablement/merge. FQ-05W prerequisites complete except FQ-06/07 live streams (owed); next FQ-12; then FQ-13–17 and the final protected dev→main batch; D held.

- Fuel quality FQ-05W7 2026-10-17: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Bounded live discovery](../backend/evidence/fuel-quality/FQ05W7_LIVE_DISCOVERY.md). Verified href-only page parser (never invents missing periods), migration104 discovery cursor, `quality-discovery` handler inspecting exactly one listed archive per run with created-only enqueue budget and cursor hold/wrap, shared enqueue with revalidation, and the injected `Discovery` handler fixed to charge only created jobs. Real-PostGIS member/starving/correction-hash/dead-job/recovery/wrap/wrap-convergence cases and the six-edition budget-progress case PASS, plus the full affected race/PostGIS matrix, vet/build/staticcheck/sqlc stability, whitespace and secret scan. No schedule registration/import activation/live fetch/deployment/merge. FQ-05W remains IN_PROGRESS for the static Rust worker image and explicit bounded source scheduling plus FQ-06/07 runtime reconciliation; then FQ-12–17 and the final protected dev→main batch; D held.

- Fuel quality FQ-05W6 2026-10-17: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Durable Directory revalidation](../backend/evidence/fuel-quality/FQ05W6_DIRECTORY_REVALIDATION.md). Platform enqueue now reports created/existing/status (eight concurrent callers create exactly one job), Directory owns a bounded batch evidence read, migration103 adds an edition-level revalidation cursor, and a reserved `quality-revalidate` sweep compares published active associations with live Directory evidence, enqueues one frozen-provenance refresh per divergent edition, converges repeated sweeps and replays a converged dead refresh once with an audited reason. Real Rust→Directory→PostGIS retirement→sweep→dead-replay→correction→frozen-history acceptance and deterministic cursor-progress cases PASS, plus the full affected race/PostGIS matrix, vet/build/staticcheck/sqlc stability, whitespace and secret scan. Unbound→rebound reactivation is explicitly not auto-republished (frozen publication-key contract). No schedule/import activation/deployment/merge. FQ-05W remains IN_PROGRESS for bounded live discovery/non-starving enqueue, static worker image and explicit scheduling; then FQ-12–17 and the final protected dev→main batch; D held.

- Fuel quality FQ-05W5 2026-10-17: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Native worker composition](../backend/evidence/fuel-quality/FQ05W5_WORKER_COMPOSITION.md). Existing HTTPS/Rust/Directory/live guard/publication composed with a bounded separate-connection source lease and default-off isolated import claims. Real Rust→PostGIS replay/retirement/history, SQL lock/cancel/failure/pool isolation and disabled queue attempts PASS; five affected race packages and build/static/SQL checks PASS. No import activation/deployment/merge. Maintainer requested stopping after this microstage; [remaining-work handoff](FUEL_QUALITY_REMAINING_HANDOFF.md) covers post-publication Directory revalidation, bounded discovery/non-starving enqueue/static image, FQ-12–17 and final protected delivery. FQ-05W overall remains IN_PROGRESS; D held.

- Fuel quality FQ-05W4 2026-10-17: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Verified file-backed job boundary](../backend/evidence/fuel-quality/FQ05W4_FILE_IMPORT.md). Strict source/member payload before I/O, private bounded file hash/byte checks before/after prepare, mandatory whole-attempt serial and live publication-guard ports, cleanup/error containment and shared existing publication semantics. Race/PostGIS job suite and negative/cancel/stale/replay/cleanup retry cases PASS; affected build/static checks PASS. Actual native fetch/lease/Directory/discovery/claim slots/image composition remains next; FQ-12–17/final protected PR/main pending, D held.

- Fuel quality FQ-05W3 2026-10-17: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Directory source identity/fence](../backend/evidence/fuel-quality/FQ05W3_DIRECTORY_FENCE.md). Owned bounded exact/history/branch reads, conservative current-identity policy, per-publication/replay transaction guard and ordered512-pair locks. Retirement/status barriers,514-binding boundary, failures/no partial writes and indexed101→102 upgrade PASS. Eight quality race/PostGIS packages, affected Directory regressions/build/static checks PASS. Native fetch/discovery/worker/image composition remains next, then FQ-12–17/final protected PR/main; D held.

- Fuel quality FQ-02C 2026-10-16: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Historical source clocks](../backend/evidence/fuel-quality/FQ02C_SOURCE_CLOCKS.md). Versioned native date-time support preserves original unzoned clock and distinct full-clock sample/fingerprint scope; migration101 keeps old rows NULL, immutable reads expose optional source clock. Real100→101/replay/failed-correction, eight-package quality race/PostGIS, actual HTTP/OpenAPI and Rust113/static checks PASS. January96,239→44,544 accepted/51,695 duplicates/0 quarantine; September counts/logical hash unchanged. Next: FQ-05W Directory/fetch/worker/image prerequisites, then FQ-12–17/final protected PR/main; D held.

- Fuel quality FQ-05W2 2026-10-16: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Bounded archive/process](../backend/evidence/fuel-quality/FQ05W2_ARCHIVE_PROCESS.md). Verified file-backed parent/member ZIP handling, preallocation budgets, native-only trusted Rust process, redaction/timeout/cleanup and real source/static binary checks PASS. Annual inventory has twelve monthly editions; January has 5,263 quarantined ISO date-time rows, so FQ-02C compatibility is next before historical activation. Native Directory/fetch/worker/image composition remains owed; FQ-12–17/final PR/main pending; D held.

- Fuel quality FQ-05W1 2026-10-16: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Monthly provenance and monotonic history](../backend/evidence/fuel-quality/FQ05W1_MEMBER_PROVENANCE.md). Migration100 freezes exact archive members without changing memberless replay keys; discovery/dedupe and public revision metadata preserve monthly identity. PMQC period snapshots, late historical current/watermark protection and eight-writer real SQL lock barrier PASS, including 99→100 replay. Critical quality race/PostGIS and affected contract/build/static checks PASS. Native archive/Directory/worker composition remains IN_PROGRESS in FQ-05W; FQ-12–17 and final protected PR/main delivery pending, D held.

- Fuel quality FQ-05C 2026-10-16: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Verified Rust–Go bridge](../backend/evidence/fuel-quality/FQ05C_PREPARED_BRIDGE.md). Strict bounded file-backed integrity/loss accounting before read-only resolution, opaque full sample scope, calendar correction, real Rust→Go→PostGIS/read replay convergence and CI preparer wiring. Seven-package critical race matrix PASS. Recovered official September hashes match; 66,225→34,158/32,067/0 and 4,164 samples verified; bridge 302 ms, peak RSS 45,800 KiB locally. No native worker/deployment activation. Next: FQ-05W trusted archive/Directory/worker composition, then FQ-12–17; D held, final protected PR/main delivery pending.

- Fuel quality FQ-08H 2026-10-15: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Complete revision access](../backend/evidence/fuel-quality/FQ08H_HISTORICAL_READS.md). Bounded public live revision catalog, source-bound historical selection on existing routes, frozen provenance detail, status/current/correction ancestry, legacy/withdrawn containment and ETags. Race/PostGIS/actual-wire contracts and 5,001-revision indexed load proof PASS; no new schema or deployment. Historical facts now reachable by explicit revision. Next: native Go importer composition, then FQ-12 onward; final protected PR/main delivery pending, D held.

- Fuel quality FQ-11 document-free slice 2026-10-15: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Review/appeal/private signal evidence](../backend/evidence/fuel-quality/FQ11_DOCUMENT_FREE_REVIEW.md). Revision-bound restricted moderation, legacy-unbound refusal, atomic audited decisions, signed owner appeal/hidden withdrawal, per-account transactional existing quota, bounded private queue/group/inspection and erasure. Immediate race/PostGIS/auth/replay/concurrency/upgrade/rollback and affected contracts/static checks PASS. Purchase-supported independence remains D-held; no public community alert/score promotion. Next: complete historical read and native importer composition prerequisites, then FQ-12 onward. No push/main merge/deployment; PR137 remains open for final batch.

- FQ-04c independent verification 2026-10-15: ee528693 confirmed clean; complete quality real-PostGIS/race suite PASS. Strengthened changed/removed period/resource snapshots, exact upgrade chain, all legacy NULL fields, superseded raw-resource replay and import watermark preservation; six targeted cases plus affected static/SQL checks PASS. [Evidence](../backend/evidence/fuel-quality/FQ04C_REVISION_PROVENANCE.md). Historical API access remains owed; next: document-free FQ-11. No push/merge/deployment from this verification.

- Fuel quality FQ-04c 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Immutable revision provenance](../backend/evidence/fuel-quality/FQ04C_REVISION_PROVENANCE.md). Per-revision frozen manifest/resource/period snapshots with publication-key convergence (locked editions, superseded retries refused without watermark drift); legacy revisions explicitly LEGACY_UNVERIFIED with unavailable leases, never fabricated. Real-PostGIS `-race` green: correction survival, 8-way convergence, period snapshots, upgrade replay with legacy state, failed-correction preservation. Next: document-free FQ-11 (FQ-10 D-held); PR137 stays open until final batch delivery.

- Fuel quality native compatibility FQ-02B 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on maintained dev. [Verified native PMQC](../backend/evidence/fuel-quality/FQ02B_NATIVE_PMQC.md). Strict 19-column source mapping, nine real products, Sim/Não, native sample/product grouping, verbatim/empty results, hashed file-backed prepare_quality CLI and append-only migration97. Actual September66,225 rows reconcile as34,158 accepted/32,067 metadata-only-coordinate repetitions/0 quarantined, across4,164 samples. Rust111 tests/fmt/clippy, real PostGIS96→97 upgrade/replay/rollback, Go parity and scoped checks PASS. No live import, score or coverage claim. Next: immutable per-revision provenance and complete-history read hardening before document-free FQ-11/FQ-12 onward; D remains held.

- Fuel quality FQ-09 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on maintained dev after FQ/A–D history reconciliation. [Signed private reports](../backend/evidence/fuel-quality/FQ09_SIGNED_REPORTS.md). Free account plus exact-body signed binding; live transaction guards, optimistic edits/withdrawal, content-bound command replay, current station/fuel/sample lease checks, atomic versioned review jobs, private owner reads and bounded account erasure. Ten affected race/PostGIS packages and HTTP/worker/contract/static checks PASS. FQ-10 D-held. Newly verified native PMQC layout/Aspecto rules require FQ-02 compatibility hardening before calibration/live import; no score/pilot/release claim. FQ-08 hardening also PASS: frozen handler lease, schema-valid unknown/empty responses, lease-aware ETags and native-key cursor decoding; baseline race/cache failures reproduced. Next: native-input adaptation, then document-free FQ-11 and calibrated remaining FQ scope. PR137 remains open to dev until final batch delivery; current required CI not used as construction gate.

- A–D consolidation OPT-PLAN 2026-10-14: PLANNING_LOCAL_DONE / INTEGRATION_PENDING in isolated app-consolidation-plan from 91ff1f93; occupied FQ/primary edits preserved. [Master](APP_CONSOLIDATION_PLAN.md), [A](HOME_OFFLINE_EFFICIENCY_PLAN.md), [B](COMMUNITY_RELIABILITY_PLAN.md), [C](ECONOMY_RETENTION_PLAN.md), [future D](FUTURE_EVIDENCE_BUSINESS_PLAN.md), [target rules](../product/APP_CONSOLIDATION_TARGET_CONTRACT.md), [continuation](APP_CONSOLIDATION_EXECUTION_PROMPT.md). A/B/C implementation PLANNED; D DEFERRED_FUTURE / NOT_AUTHORIZED until extreme consolidation plus thousands of real clients, unless explicit earlier named maintainer request. FQ-10/document parts of FQ-15 and new P30–P33 portal/private-proof timing aligned; FQ-00–08 historical source evidence intact. Docs validation PASS: 50 local links, 33 task contracts, 18 rules/10 use cases, D precedence, whitespace and scoped secret/PII review. No runtime/provider/device/VPS/PR/wiki or release claim. Next: integrate the local planning patch when selected, then explicit A implementation/current-head reconciliation; D remains excluded.

- Fuel quality FQ-08 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Official read API](../backend/evidence/fuel-quality/FQ08_READ_API.md). Frozen-lease Reader (summary, keyset assay/action/sanction pages, sample/event detail), anonymous chi routes with lease envelopes, 180-day staleness rule, sealed revision-bound cursors, 503/404 code discipline, kill switch `ANPFUEL_QUALITY_READS_ENABLED`, additive OpenAPI + main wiring. Real-PostGIS `-race` green (read 7/7, http 6/6); EXPLAIN index-only, no denormalization; apicontract green. VPS recovery confirmed: stale hostPath command overrides executed community binaries instead of the FQ-08 image. API/worker now execute /api and /worker with immutable image digests; pod 3/3, readiness200, public station read200 and FQ route404 proof. Private storage/env/volumes and other namespaces preserved. Current construction resumed on maintained dev after reconciliation of FQ and A–D planning histories. Next: FQ-09 signed structured reports; D document/portal scope remains deferred.

- Fuel quality FQ-07 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Sanction snapshots](../backend/evidence/fuel-quality/FQ07_SANCTION_SNAPSHOTS.md). New `station-prep sanction` module: frozen 8-col `sanction-v1` contract, verbatim extraction-time status, strict minor-unit amounts with no math, cross-snapshot proceedings, correction pairs, fallback periods + 12-row fixture and 14-case suite. RED→GREEN (fixture header typo refused by strict match, fixed). Full crate 104/0; fmt/clippy/diff-check/secret clean. Next: FQ-08 official read API.

- Fuel quality FQ-06 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Enforcement adapter](../backend/evidence/fuel-quality/FQ06_ENFORCEMENT_ADAPTER.md). New `station-prep enforcement` module: frozen 7-col `enforcement-v1` contract, verbatim scope/status (no guilt semantics, machine-checked), correction pairs retained, period labels for mismatch evidence + 12-row fixture and 12-case suite. RED→GREEN (correction-pair sort ties fixed in both readers). Full crate green (enforcement 12/12, quality 15/15, parser 15/15 compatible); fmt/clippy/diff-check/secret clean. Next: FQ-07 sanction snapshots.

- Fuel quality FQ-05 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Backfill and discovery](../backend/evidence/fuel-quality/FQ05_BACKFILL_DISCOVERY.md). Pure PlanBackfill (import/replay/correct/alias/skip), migration 000095 watermarks, Discovery/Import jobs with injected list/fetch/prepare, shared publisher machinery with fallible IDs. Real-PostGIS `-race` green: bounded chunk, 4-run convergence via platform queue, late import, watermark advance, failure preservation. Enforcement-content discovery waits on FQ-06. Next: FQ-06 enforcement adapter.

- Fuel quality FQ-04b 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Event ledgers](../backend/evidence/fuel-quality/FQ04B_EVENT_LEDGERS.md). Migration 000094 (enforcement/sanction events with verbatim scope/status, nullable minor-unit amount, period claims), extended sqlc `quality` queries, EventsInput + PublishEvents reusing the 04a single-tx machinery (extracted stage closure; 04a behavior unchanged). Real-PostGIS `-race` green: scoped lease, replay, rollback, correction, periods. FQ-04 LOCAL_DONE; next: FQ-05 backfill and discovery.

- Fuel quality FQ-04a 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Assay ledger](../backend/evidence/fuel-quality/FQ04A_ASSAY_LEDGER.md). Migration 000093 (editions/resources/manifests/revisions/assays/links/current pointer, one-published-per-edition index), sqlc `quality` stanza generated, application Publisher port, single-tx adapters publisher with loser convergence. Real-PostGIS `-race`: 7/7 integration (lease, replay, 8-racer, rollback, correction, staging invisibility, recovery); official neighbor suites PASS. FQ-04 IN_PROGRESS; next: FQ-04b enforcement/sanction ledgers.

- Fuel quality FQ-03 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Identity resolution](../backend/evidence/fuel-quality/FQ03_IDENTITY_RESOLUTION.md). New stdlib-only `resolution.go`: read-only DirectoryView port, five-outcome Resolve (bound/unmatched/ambiguous/wrong_branch/succession_review), LinkRevision with independent clocks + window validation. 22/22 Go tests PASS; vet/gofmt/diff-check/secret clean. No station write, checksum stays with kernel/Directory. Next: FQ-04 ledger and atomic publication.

- Fuel quality FQ-02 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Quality preparation](../backend/evidence/fuel-quality/FQ02_QUALITY_PREP.md). New `station-prep quality` module: frozen 8-col `pmqc-assay-v1` contract, verbatim native results/units, kernel-mirror fuel map, closed conformity vocabulary, deterministic sort, exact-duplicate twins, manifest hash + 12-row fixture and 14-case suite. RED→GREEN (header-case test bug, clippy arity refactor). Full crate green (quality 14/14, parser 15/15 compatible); fmt/clippy/diff-check/secret clean. Coordinate/loader artifacts unchanged. Next: FQ-03 identity resolution.

- Fuel quality FQ-01 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Batch contracts](../backend/evidence/fuel-quality/FQ01_CONTRACTS.md). New stdlib-only `quality/domain`: frozen assay/enforcement/sanction streams, sample/assay keys, kernel-parity fuel enum, closed conformity vocabulary, loss-accounting grouping, reconciled manifest, append-only corrections, collision review + 2 golden manifest fixtures. RED→GREEN (accent fold order, sample-key buckets). 13/13 Go tests PASS; vet/gofmt/diff-check/secret clean. No ADR (reuse only), no score/migration/API/Android. Next: FQ-02 Rust preparation.

- Fuel quality FQ-00 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on `codex/fuel-quality-plan` (PR #137 → dev). [Source inventory](../backend/evidence/fuel-quality/FQ00_SOURCE_INVENTORY.md). Bounded verified-TLS checks: 5 official pages 200, metadados.pdf serves DOCX bytes, guidance `.pdf` now 404 (page links `.docx`); 2016–2025 annual present, 2026 monthly 01/03–09 present with 07 duplicated (Feb label → 07 file, 02 absent); 5-col coordinate contract quarantines any assay header (no result/unit/conformity parsing). No ZIP downloaded, no hashes/schemas verified; test-agent `codex/community-first` and primary dirty Store edit untouched. Next: FQ-01 contracts.

- Fuel quality FQ-PLAN 2026-10-14: PLANNING_LOCAL_DONE / INTEGRATION_PENDING; implementation FQ-00–FQ-17 PLANNED. [Professional implementation plan](FUEL_QUALITY_IMPLEMENTATION_PLAN.md), [proposed product B-BR/BUC](../product/FUEL_QUALITY_TARGET_CONTRACT.md), [bounded continuation](FUEL_QUALITY_EXECUTION_PROMPT.md). Isolated from occupied checkout at local dev 757c09ae; primary detached e22824db Evidence Store edit preserved. Reuses existing Rust/Go/Directory and dated ING-AUDIT source evidence; plans PMQC samples/assays, exact CNPJ linkage/quarantine, separate enforcement/sanctions, private structured reports, complete-case access and calibrated nullable per-fuel scores. Official pages checked; raw schemas/score parameters still require FQ-00/12. Docs validation PASS: 43 local links/19 anchors, 18 task contracts, 16 B-BR/9 BUC, whitespace and scoped secret/PII review. No runtime tests, live import, VPS/device, issue/PR/merge/wiki or release claim; remote checks not refreshed. Next: reconcile this planning patch into dev when selected; implementation starts at FQ-00 after current-head prerequisite selection.

- Community final safety/deployment 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING, tested source 96cc0bcc on isolated codex/community-first. [Final safety evidence](../backend/evidence/community-safety/COMMUNITY_FINAL_SAFETY.md). Fixed invisible public prices/anchors, orphan consensus groups, transactional report→operator queue, quota/owner/expiry races, atomic INVALIDATE/BLOCK, missing trust worker and durable-job event deduplication/version progression. Frozen real PostGIS/race matrix, actual signed API→worker→summary→like/dislike/remove/report and Android data/app JVM + 15 final Home cases/lint/debug/native-test/release compilation PASS. User explicitly authorized protected green-CI main merge and Abastevo-only VPS publication; other systems/images and occupied checkouts excluded. Private backup restored into isolated Abastevo rehearsal database; live schema 74/intake 0 unchanged, one legacy orphan report identified for bounded repair. Next: cumulative dev→main PR/current-head CI, guarded merge, checksum-verified migration rehearsal/cutover and actual HTTPS proof. Device/elapsed retention/full G09 certification remain owed; PRM22 deferred.

- Community Home continuation 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING, tested source 8e6f8387 on isolated codex/community-first / [PR135](https://github.com/AlexandreZanata/Abastevo/pull/135). [Evidence](../backend/evidence/community-safety/COMMUNITY_HOME_CONSOLIDATION.md). Removed developer capture from both APKs; ordinary camera/OCR/review/signed contribution has no GPS admission. Home ANP / Community selector shares average/minimum/maximum filters; full-city backend summary is independent of feed pages, source/time/unit separated, foreground refresh every 15s. Community-only station keys/typed cache/UI fixed. Real PostGIS 65-station four-order pagination, 1,500-station one-query/EXPLAIN, rounding/transaction/expiry tests and actual signed contribution→worker→Home summary PASS. Android 1,583 discovered/1,581 executed JVM cases plus final 12 Home cases, 90.19% domain coverage, lint/debug/release/native-test compilation PASS; parallel incremental lint/Hilt race resolved sequentially without disabling lint. Backend fast/vulnerability/mobile static/secret checks PASS. Next: current-head required Quick verification and review on cumulative PR135; integration, native/device/staging/provider proof remain pending. Primary dirty adapter/occupied dev preserved; no merge/deploy/wiki.

- Community-first construction 2026-10-14: LOCAL_DONE / INTEGRATION_PENDING on isolated codex/community-first from integrated dev/main 68483db9; tested source e2a58f25. [Evidence](../backend/evidence/community-safety/PRM_COMMUNITY_FIRST.md). PRM15–PRM20 source and PRM21-T01/T03 local acceptance/handoff complete; PRM21-T02 [rollout/rollback prepared](../backend/evidence/community-safety/PRM_COMMUNITY_FIRST_ROLLOUT.md), actual staging/device proof OWED. Rust, real PostGIS/API-worker/private S3 campaigns, backend fast/vulnerability/mobile static and Android 1,580 discovered / 1,578 executed JVM tests PASS (two explicit opt-ins skipped); domain instruction coverage 90.15%, lint/debug/release/native-test compilation PASS. New normal v2 capture and independent-evidence validation omit GPS/150m, normal nominal search preserves address-only identity, signed helpfulness/report controls replace fake local counters. Next: authorized cumulative GitHub PR to main; required Quick verification and actual integration remain pending. PRM22 DEFERRED, no national coverage claim. Primary unrelated dirty adapter/occupied dev preserved; no merge/deploy/wiki.

- Community-first PRM amendment 2026-10-13: PLANNING_LOCAL_DONE / IMPLEMENTATION_PLANNED. [PRM15–PRM22 plan](PREMIUM_BACKEND_APP_PLAN.md#2026-10-13-community-first-planning-amendment--prm15prm22), [next-agent prompt](PRM_COMMUNITY_FIRST_PROMPT.md), [source inventory/document validation](PRM_COMMUNITY_FIRST_VALIDATION.md). Added 21 initial-policy tasks for optional station coordinates/nominal address, normal name search, no mandatory 150m capture/intake barrier and free community validation/moderation, plus three DEFERRED future proximity tasks. Future enforcement requires validated product, strictly >90% reliably verified national coordinate coverage and new explicit activation authority. Documentation only in an isolated detached checkout from a104ed80; occupied dev e22824db/evidence adapter and Premium implementation worktree preserved. No runtime checks/changes, coverage claim, PR/remote publication/deployment/merge/wiki. Next: explicit implementation request and scoped amendment reconciliation into maintained dev, then PRM15-T01; existing in-flight authorizations remain scoped.

- Main reconciliation 2026-10-13: INTEGRATED. All progress branches merged into protected `main` as `68483db9` via guarded PR132 dev → main (finish, commits preserved, no bypass). PR131 (premium), PR133 (audit) and PR134 (mobile-sync) closed as merged by ancestry; all 13 branch tips proven ancestors of `main`. Final head `cacd0a67`: Quick verification + fast + integration + test all success; local evidence 846 JVM Android green, landing 45/45 twice, backend fast gate green, real-PostGIS RED→GREEN for the discover v2 scope-key lookup. Two timing-sensitive integration cases flaked once on CI and passed on rerun plus 5/5 local -race each (documented, not weakened). Wiki mirrored once from `68483db9` (remote `00ea7e7`, owned pages only, manual pages preserved). `dev` retained and synced at `68483db9`. Primary checkout detached at `e22824db` with the owner's dirty Store edit preserved/excluded; bundle + 22 archive refs kept. Retired locally only `codex/main-reconciliation`; remaining construction branches still occupy others' worktrees and were left untouched. [Record](repository-reconciliation-20261013.md). No deployment/release certification; commercial acceptance (G24) and production certification (G09) remain separate.

- Landing fusion 2026-10-12 (user-authorized, Codex wins): illustrated hero + six-scene + fleet art fused from `codex/landing-hero-art` (landing scope only; app/backend excluded). Overrides kept: header 24px/17.6px, 1152px/12px margins, mobile centered hero, no stats strip, no cheapest/savings/coverage claims; art decorative only.

- LH07 2026-10-12: minimal opening LOCAL_DONE / INTEGRATION_PENDING on isolated `codex/landing-hero-art`, base `e90edab`. [Evidence](../assets/landing/minimal-20261012/BRIEF.md). Removed landscape from HTML/runtime manifest/dist; clean neutral opening with circular CSS support, balanced typography/spacing and authentic Android preview. Standard margins, existing text/buttons and all six artwork bytes preserved. Typecheck/build/check and 45 tests PASS; nine browser widths in both themes without overflow/phone overlap, one theme icon and no hero background image. Primary security work and unrelated prompt edit untouched. Next: integrate five landing checkpoints after concurrent dev work is saved, then protected delivery. No remote/release claim.

- LH06 2026-10-12: fleet action/Android frame LOCAL_DONE / INTEGRATION_PENDING on isolated `codex/landing-hero-art`, base `4e999aa`. [Evidence/asset/prompt](../assets/landing/fleet-android-20261012/BRIEF.md). Regenerated only small-fleet scene (person organizing fuel costs beside vans, rose accent, 28 KB at 640px); five other scene hashes unchanged. Header/hero use standard 72rem/12px bounds; landscape stays full bleed. Refined Android punch-hole/metal frame with authentic minimum-720px capture. Typecheck/build/check, 45 tests and nine browser widths in both themes PASS; margins align, no overflow/phone overlap. Primary security work, reference checkout and unrelated prompt edit untouched. Next: integrate four landing checkpoints after concurrent work is saved, then protected delivery. No remote/release claim.

- LH05 2026-10-12: illustrated landing LOCAL_DONE / INTEGRATION_PENDING on isolated `codex/landing-hero-art`, base `c126127`. [Evidence/assets](../assets/landing/illustrated-20261012/BRIEF.md). Full-viewport flat editorial landscape behind authentic Android home; source badges outside phone, six unique-color scenes and four reusable transparent decorations. 3840×2160 export is enlarged from native 1672×941 (recorded); runtime hero ≤95 KB. OpenCode `41d1372` gutters/branding/hero-fact removal/404 header retained. Typecheck/build/check and 45 tests PASS; nine browser widths in both themes without overflow/overlap, one theme icon. Primary security work and unrelated prompt edit untouched. Next: integrate the three landing checkpoints into maintained dev after concurrent work is saved, then protected batch delivery. No remote/publication/release claim.

- LH04 2026-10-12: reference landing synchronization LOCAL_DONE / INTEGRATION_PENDING on isolated `codex/landing-hero-art`, based on 5360f8b. [Evidence](../assets/landing/sync-20261012/BRIEF.md). Adopted localhost:8099 (landing-refresh df2b992) headline, paragraph, branded two-line GitHub/Play actions and four hero facts; retained authentic Brazil/Android composition, six illustrations and grayscale theme. Native header navigation on all 25 script-enabled pages; honest prelaunch status and verified published-state links. Typecheck/build/check and 44 tests PASS. Browser seven widths 320–1440: zero overflow, controls contained, one theme icon, phone/badges separated and equal desktop art/copy height. Primary security work and unrelated existing prompt edit untouched. Next: integrate both landing commits into maintained dev after active work is saved, then protected batch delivery. No publication/release claim.

- LH01–LH03 2026-10-12: landing artwork/theme LOCAL_DONE / INTEGRATION_PENDING in isolated `.worktrees/landing-hero-art` (`codex/landing-hero-art`, base dev `9fa7ad1`); active security edits in primary dev untouched. [Evidence/assets/prompts](../assets/landing/hero-20261011/BRIEF.md). Authentic Android home over artistic Brazil, equal desktop copy/art height, external source badges, six generated/traced SVG cards; latest check/camera regenerated and normalized to remove alpha defects. Black/grayscale dark theme with white text and mutually exclusive sun/moon SVGs. Typecheck/build/check and 42 tests PASS; six vectors >99% rendered silhouette/ink/white fidelity. Responsive and theme evidence in linked record. Next: integrate atomic commit into dev after active security work is committed, then protected final delivery. No hosting/backend/mobile/release change.

- Games emulator/polish 2026-10-11: SCOPED_EMULATOR_PASS / INTEGRATION_PENDING on isolated `games`; [commands, captures and remaining gate](../mobile/games-polish-emulator.md), [original seven microtasks](../mobile/games-mvp.md). M08–M11 complete within authorized scope: illustrated selection, native scenes/HUD/feedback, responsive controls; preview bounds, landscape start/pause and narrow-label regressions fixed. Final offline API 26: 10 portrait + 2 landscape cases PASS; 9 affected unit cases PASS, lint zero errors, debug/test/release builds and APK-size check PASS (4,345,457 bytes). Full app 258 and unchanged-domain 508 cases previously passed; global domain coverage still blocks aggregate acceptance (87.2467% < 90%; baseline already 86.5864%). Atomic commits backed up to `origin/games` (GitHub canonical rename: AlexandreZanata/Abastevo). Backend/other agents preserved; no physical-phone operation, PR/merge/wiki or release certification. Next: resolve global coverage under its owning scope before integration; physical-device obligations remain owed.

- Premium PRM09-T01 2026-10-13: LOCAL_DONE / INTEGRATION_PENDING on isolated codex/premium-backend / PR131 (draft). Formal entry records G-PR-BACKEND still pending (no readiness claimed). Typed Kotlin ports/DTOs, Go-parity consumption/cost, additive Room 10->11 with owner/origin partitions and durable enqueue. [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md#prm09-t01--typed-ports-exact-values-and-owner-scoped-room-state). Domain/data/application targeted tests + assembleDebug PASS; lintDebug pre-existing 27 MissingTranslation failures on clean tree, untouched. Occupied dev preserved; no merge/deploy/wiki. Next PRM09-T02. Real payments OFF, Play sandbox PROOF_OWED.

- Premium acceptance-five 2026-10-13: IN_PROGRESS on isolated codex/premium-backend / PR131 (draft during construction), batch parent 8553b7a; authorized PRM07-T06 + PRM08-T01..T04, one commit each. Completed sequentially through PRM08-T05, tested working tree atop b3149241; task unit, race and static checks PASS. [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md). Next PRM07-T01. Payments/real Play sandbox deferred, dev/mobile preserved; no merge/deploy.

- OneSignal notification planning 2026-10-13: CONTENT/PLAN LOCAL_DONE; PRM11–PRM14 implementation PLANNED. [Bodies/cost/identity contract](../product/NOTIFICATION_CONTENT_CONTRACT.md), [extended Premium plan](PREMIUM_BACKEND_APP_PLAN.md). Added 17 microtasks without renumbering the original 62; source order integrates OneSignal backend before G-PR-BACKEND and app/provider acceptance before commercialization. No SDK/credential/send/deploy/mobile edit. First-ten backend batch source complete on PR131; real payments deferred.

- Premium first-ten batch 2026-10-13: PRM00-T01..T04 + PRM01-T01..T06 source LOCAL_DONE / INTEGRATION_PENDING on isolated codex/premium-backend, main base 5dc9b2b; ten task commits plus separate authorized notification plan. [PR131](https://github.com/AlexandreZanata/Abastevo/pull/131), [tested source anchor/commands/evidence](../backend/evidence/premium/PRM_FIRST_TEN.md#prm01-t06--private-status-purchase-restore-and-free-exit-api). Full quick-verify PASS (43s; no vulnerabilities); subscription race and real-PostGIS subscription/account/migration/API matrix PASS. Tested PRM01-T06 source candidate on 44653cd; exact source digest retained in evidence. Private status/signed same-owner restore/free expired export integrated in API; provider ports nil, sales OFF. Dev/mobile edits preserved. Review/remote CI/merge pending; no merge/deploy/wiki/Android change. Next plan task PRM01-T07 (recurring reconciliation/runbook/real provider proof), outside this completed batch; payment-module/catalog wiring and sandbox remain user-deferred.

- MOBILE-SYNC-01 station auto-sync 2026-10-13: LOCAL_DONE / INTEGRATION_PENDING on isolated `codex/mobile-station-sync` (dev `e22824db` plus audit `8fefff7d`); original occupied dev and its unrelated backend edit preserved/excluded. [Evidence](../mobile/stations-auto-sync-20261013.md). Foreground sync on entry/resume/validated-reconnect/60s while RESUMED plus unique 15min connected WorkManager job; municipal IBGE scope with scoped Room snapshot (page bound 100); cancellable OkHttp gateway with serialized use case; non-clickable list-summary card (city/fuel/cheapest-first) in 8 locales. 62 scoped JVM tests green, lint 0 errors, debug/test APKs compile; 18 physical API36 cases PASS including shell-level radio loss/return and live staging Sorriso read/outage/reconnect; product APK `05d6abc4…`, radios restored. Next: atomic commit, then scoped dev-targeted integration PR (fresh `origin/dev` + audit merge + cherry-pick, excluding Home `e22824db`); no main merge without Quick verification, no release claim.

- ING-AUDIT-01 scoped GitHub delivery 2026-10-13: INTEGRATION_PENDING / PR_PENDING. Explicit user scope: only this chat's source/activation evidence. Another session advanced remote dev from `bc6d5f29` to `268155ff` with 15 unrelated UI commits during publication preflight; those commits and unfinished local evidence edits are preserved/excluded. Publish the existing clean isolation branch `codex/station-reliability-audit` (`2cc8b7b`, `73ca2d76`, scoped delivery records) with a main-targeted PR containing only this chat, using ADR-020's existing clean delivery branch path. [Tested source and authorized VPS evidence](../backend/STATION_RELIABILITY_AUDIT_20261013.md); behavior/base inputs unchanged from tested main `b7800b2b`. Live main protection verified: required `Quick verification`, strict current-base checks, enforced for admins, no force push. Next: push the isolated branch, open scoped protected PR, record actual URL/check state in delivery metadata. No main merge, unrelated dev integration or further VPS rollout is part of this publication request.

- ING-AUDIT-01 national activation 2026-10-13: ACTIVATED_STAGING / INTEGRATION_PENDING; explicit user follow-up supersedes the earlier approval block. [Execution/preservation evidence](../backend/STATION_RELIABILITY_AUDIT_20261013.md#completed-execution-and-activation). Tested `2cc8b7b` tools ran the authorized Abastevo `registry-sync` container: 45,762 = 45,641 accepted + 0 duplicates + 121 quarantined, 168.695s; verify-only replay 7.111s. All 45,618 prior UUIDs, existing CNPJ bindings/profiles, protected locality/status/locations and ten curated facts preserved; 28 new identities, 11 source-backed refreshes, four absent source CNPJs retained without inferred revocation. Only CronJob tools reference activated, daily 03:00 America/Cuiaba, unchanged 250m/256Mi image/security/state; 26+3 guard samples, zero violations/restarts/OOM, final 4/4 ready. API/worker/storage/edge/other apps untouched. Source integration/PR/wiki, API/worker policy rollout, unknown coordinates, 121 quarantined rows and native/release/soak gates remain separate. Original occupied dev preserved.

- ING-AUDIT-01 Sorriso reliability initial checkpoint 2026-10-13 (activation later superseded by the entry above): LOCAL_DONE / INTEGRATION_PENDING in an isolated main-based checkout; original occupied dev preserved. [Audit, official ANP references and independent RFB/IBGE/OSM sources](../backend/STATION_RELIABILITY_AUDIT_20261013.md). Zero live CNPJ duplicates; UI appended legal/trade-name matches twice, now filtered by full visible CNPJ. Rust0.3.1 header/field hardening, actual API contract/pagination/quotas/versioned replay and copied-location refusal pass JVM/lint/APK and real PostGIS/race. VPS isolated preparation 45,738=45,617+0+121; Sorriso API25/replay same run, 24 candidate points with149.9/150.1m geometry checks, all still unreviewed, canonical UUIDs preserved. Full national persistent publication rejected by automatic approval review pending explicit confirmation; active daily tools/API/worker/storage unchanged. Native install, protected dev delivery/PR/wiki and national activation remain pending; no production certification.

- Bottom tab navigation 2026-10-13: LOCAL_DONE / INTEGRATION_PENDING, isolated from occupied dev at `8a4503c`. [Behavior/tested source evidence](../mobile/bottom-tab-base-navigation.md). Every Explore/Community/Profile bottom-bar action now opens its base without restoring a child stack; single-top retained. Focused JVM navigation 18/18, lint zero errors, debug app/test APKs passed; five actual-controller regression cases compiled, native execution OWED in deferred device batch. User authorized delivery to dev only; no PR/main merge/deployment/wiki claim. Occupied checkout preserved. Next: push scoped commit to dev; final protected batch/native acceptance remain separate.

- Safety/consent batch delivery 2026-10-13: INTEGRATION_PENDING / CI_PENDING. User requested the cumulative dev → main PR now and merge only on green CI. [Delivery scope](../backend/evidence/community-safety/RVA_BATCH_DELIVERY_2026-10-13.md). Clean owner-consent head f7c0d7a includes committed dev corrections; live main/dev 5dc9b2b are ancestors. Preserve original dev's uncommitted intake-gate removal, excluded from the tested batch. Independent delivery checkout will publish dev without changing that occupied checkout. Required Quick verification + all applicable CI/review/head/base guards before finish; no source merge implies release, enabled training/uploads, full offline completion or passed stability/24h proofs.

- RVA08 E Android refresh 2026-10-13: LOCAL_DONE / INTEGRATION_PENDING. [Evidence](../backend/evidence/community-safety/RVA08_OWNER_CONSENT.md#e-requested-adb-apk-refresh--2026-10-13). User-requested physical API36 reinstall succeeded with `adb install -r`; original installation retained, matching certificate, latest 3.1.0-debug/code4 APK already byte-identical. Clean e6a8dfa build/focused test tasks up-to-date; no fresh suite execution claimed. Real Profile → Settings → full-term reader displays the exact 5,224-byte asset/hash and remains open; no consent checkbox/submission, data clear or VPS mutation. Remaining operational/offline gates unchanged.

- RVA08 directory follow-up 2026-10-12: LOCAL_DONE / INTEGRATION_PENDING. Real SQL tracer reproduced 5/43/81 search calls; bounded page-reference batch now uses two, preserves detail/pagination/source trust, passes two-connection concurrency/fault recovery and full read/HTTP race suites. [Evidence](../backend/evidence/community-safety/RVA08_STAGING_ACCEPTANCE_2026-10-12.md#bounded-directory-read-follow-up-opening). API b058a60 deployed, worker 610bdd9/schema74 retained. Actual consent 16/16 and two bounded 600/600 HTTPS windows pass, but an intervening repeat failed (eight timeouts/one drop, telemetry timeout); sustained stability/capacity remain OWED. Aggregate diagnostic shows 16.75% mean CPU steal, 53.97% over 5.020s; causal link to failed repeat unproven. Final annotations reconciled to schema74/API b058a60/worker610bdd9; 3/3 ready, HTTPS terms/health200 and retention inspection correct at 23:15Z. Limits unchanged, no other workload changes.

- RVA07.C/RVA08 staging 2026-10-12: IN_PROGRESS on isolated `codex/owner-consent`. [Evidence](../backend/evidence/community-safety/RVA08_STAGING_ACCEPTANCE_2026-10-12.md). Abastevo-only backup/rehearsal, live schema 54–73 and persistent private media deployed. Consent rollout used API/worker 610bdd9, then API b058a60 superseded it as recorded above; scoped internal 3 MiB relay preserved. First OFF without prior grant and retryable DB-outage errors corrected RED→GREEN; real PostGIS/race/Android plus actual API/worker regression pass. Private backup/restore rehearsal and live append-only 000074 passed; 16 real HTTPS consent cases PASS, both synthetic owners OFF. Worker removed five originals/four abandoned finals; elapsed READY canary cleanup due Oct 13 20:09Z, deadline 22:09Z. Shared-edge upload patch syntax-tested, NOT_APPLIED pending explicit scope extension; intake zero/training OFF. External and VPS HTTPS plus loopback campaigns all stopped with drops/timeouts; stability/root cause/capacity owed. No new ADB device, other workload mutation, main merge or full retention claim; original dirty dev preserved.

- RVA08 D consent device 2026-10-12: LOCAL_DONE / INTEGRATION_PENDING on `codex/owner-consent`, parent b4f6f00; whole RVA08/offline batch IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA08_OWNER_CONSENT.md#d-native-consent-checkpoint). Physical USB API36: eight native cases passed (10.794s), including actual bundled terms, unchecked explicit review, separate owner keys, real disk OFF/reopen, foreign-owner refusal, persisted retry deadline, startup/periodic worker and auth continuity. RED complete-reader action hidden by system navigation; fixed pinned Compose window measurement, real route now fully usable. Privacy summaries corrected in eight locales; collection OFF, no real-user consent or data clear. Debug/native APKs and final lint green. Public staging terms GET returns 404: deployment/digest and real signed staging flow still OWED, together with RVA07.C 24h/provider/backup/budget and remaining synchronization/helpfulness phases. No VPS mutation or main merge; original dev's unrelated dirty Store preserved. Next: reconcile staging within Abastevo-only deployment scope, remaining RVA08 then RVA10–14/CS15–25; final protected dev→main batch and wiki afterward.

- RVA08 A2/C consent 2026-10-12: LOCAL_DONE / INTEGRATION_PENDING in isolated `codex/owner-consent` from b83625d; whole RVA08/offline batch IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA08_OWNER_CONSENT.md). Real Settings privacy/rights cards, complete offline pt-BR terms with shared SHA-256, unchecked explicit confirmation, scoped atomic durable OFF, retained retry/body, signed private/write transport, bounded OFF-only startup/periodic recovery and account/origin-separated Keystore aliases. Full JVM 1,472 discovered / 1,470 executed, zero failures (two explicit opt-ins); lint/debug APK green. Real Kotlin→TLS→Go verifier/PostGIS grant 101/revoke 102/replay/stale/forged/other-owner proof passed; existing privacy SQL/S3 race green. Native review/Keystore/persistence tests compile, execution OWED: no physical ADB device; owned API26 emulator without KVM stopped after five-minute installation limit (no cases executed). Checkpoint a05cba7 backed up on GitHub branch codex/owner-consent; no PR/merge. Old provisional digest override now refuses startup; deployment must match the embedded actual text. Original dev's unidentified reservation-gate removal preserved, excluded from this worktree; tested Store hash remains f495194c…. Next: native consent evidence when connected; remaining RVA08 helpfulness transport/integration, then RVA10–14/CS15–25. RVA07.C actual 24h/provider/backup/device/deployment budget still OWED; training collection OFF. No VPS/release/merge claim.

- RVA08 A1 2026-10-12: consent view ownership LOCAL_DONE / INTEGRATION_PENDING; whole RVA08 IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA08_OWNER_CONSENT.md). Four RED late-account/logout/update/reverse-refresh regressions closed by Main-serialized view generations; old results cannot render/reload/release current guard. Full app JVM 290 tests, zero failures/skips and lint green (32s), including repair of 27 missing helpfulness/privacy translations in seven supported locales. Corrected RVA07 predecessor is 7f3c9e7, full repeated SQL/S3 race 37.961s with tested/committed Store hash equal. Next RVA08 A2 durable owner+origin+key commands/OFF/revisions, then signed transport; global prefs/unsigned gateway remain pending. RVA07 C elapsed/provider/device/backup and deployment budget OWED; collection OFF. No VPS/phone/publication/merge claim.

- RVA07 B3.3c.2 reservation correction 2026-10-12: post-commit review invalidated closure at 89fe92a (reservation body omitted). Corrected successor restores atomic gate; repeated full evidence SQL/S3 race and affected units/vet, with source SHA-256 and committed blob matching required. No publication/merge; use successor checkpoint, retain external obligations. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md).

- RVA07 B3.3c.2 2026-10-12: intake assurance LOCAL_DONE / INTEGRATION_PENDING; phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). Production gated store, default-zero explicit budget (source ceiling 100), three-minute latest successful worker observation, atomic global capacity and 22h-boundary refusal; append-only 000073. SQL RED 40/4 overshoot corrected, GREEN exact four winners, replay/timeout/unknown/stale/rollback/duplicate-key/upgrade/10k-history query plans. Full evidence SQL/S3 race 29.009s, privacy erase/restore 5.243s, actual signed API+worker SQL/S3 flows 20.132s and affected units/vet/sqlc/diff green. Next: C autonomous/provider/backup/temp/device proof preparation, then RVA08 independent source; measured deployment budget/24h proof OWED. No VPS/phone/old-soak/release/publication/merge claim.

- RVA07 B3.3c.1 2026-10-12: safety dispatcher isolation LOCAL_DONE / INTEGRATION_PENDING; phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). Existing worker has four fixed serial kind-scoped safety loops plus general loop excluding them, with cancellation/join and required-handler guard; no new service/container. SQL blocking-general/cleanup-progress and foreign-claim isolation green; actual claim EXPLAIN uses append-only 000072 partial index over 20k unrelated rows, upgrade/replay preserves live lease/dedupe. Full jobs SQL race 6.281s, affected units/vet/sqlc/diff green. Next: B3.3c.2 intake health/inventory/backlog/capacity, then C autonomous/provider proofs; RVA12 queue retention. No measured VPS capacity/phone/soak/release claim.

- RVA07 B3.3b.2 2026-10-12: predeadline/pass bounds LOCAL_DONE / INTEGRATION_PENDING; phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). Cleanup begins at 22h without moving the 24h anchor; source/final leases and commits respect margin, source fence uses live time, queued validators cannot extend retention. Whole pass 45s, separate 20s original/final slices, 32+32 candidates and 5s transfers; append-only 000071 hot index. Real SQL/S3 boundary/live-writer recovery, writer refusal, 10k receipt EXPLAIN and upgrade/replay green; full evidence race 20.163s, privacy erase/restore 4.585s and affected units/vet/sqlc/diff green. Next: B3.3c admission/worker/backlog capacity, C autonomous/provider/backup/device proofs. Two-hour reserve is not a capacity or 24h certification; no VPS/phone/old-soak/release claim.

- RVA07 B3.3b.1 2026-10-12: actual minute dispatch LOCAL_DONE / INTEGRATION_PENDING; phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). Fixed shared scheduler bug collapsing minute intervals into hourly dedupe; evidence now requests every-minute dispatch. Real SQL RED second minute stalled at four jobs, GREEN four schedules × three minutes × 12 concurrent restart views yields 4/8/12 exact jobs; full jobs SQL race 6.710s, affected units/vet/diff green. Earlier minute interval declarations alone were insufficient. Next: B3.3b.2 predeadline/pass fairness/live lease clock, B3.3c admission/capacity, then C proofs. No capacity/VPS/phone/soak/release claim.

- RVA07 B3.3a 2026-10-12: confirmed metadata purge LOCAL_DONE / INTEGRATION_PENDING; phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). At most 100 oldest hash rows, only after both byte receipts, SKIP LOCKED and append-only 000070 partial index; uncertain copies retain inventory. SQL RED loss reproduced then 205-row restart, 12-way concurrency, rollback, 10k live-history EXPLAIN and upgrade/replay green; full evidence SQL/S3 race 22.238s, affected units/vet/sqlc/diff green. Next: B3.3b predeadline dispatch/pass fairness/live clock, B3.3c admission/capacity, then C autonomous/provider/backup proofs. No VPS/phone/old-soak/release claim.

- RVA07 B3.2b 2026-10-12: final cleanup/recovery LOCAL_DONE / INTEGRATION_PENDING; whole phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). 32-final/45s reconciliation, 5s DELETE+HEAD absence, UUID lease/atomic object+inventory receipts, stale cleaner refusal and restart/repair; unconfirmed destinations join overdue audit. Actual SQL/S3 late writer/erasure, lost reply/outage/repair, foreign-owner reuse, 12-way claim, receipt rollback, 65-copy 32/32/1/0, 10k history EXPLAIN index, 000069 upgrade/replay passed. Full evidence SQL race 16.346s and privacy erasure/restore 1.824s green, affected units/vet/sqlc/format/diff; CI fixture owned startup/cleanup 2.657s passed after prior approval usage-limit interruption. Next: B3.3 predeadline dispatch + pass/admission/retention bounds, then C autonomous/provider/backup proofs. Storage-only restore protocol remains RVA14.C; no old-soak/VPS/phone/release claim.

- RVA07 B3.2a 2026-10-11: final inventory/admission/commit fence LOCAL_DONE / INTEGRATION_PENDING; whole phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). Append-only 000068 inventories destination before PUT with 60s UUID authority/five attempts, 20s job, reservation deadline and read-only duplicate reuse. Guarded commits cannot recreate READY after erasure/lease expiry; pending final inventory prevents premature erasure completion. 12-way SQL race, trigger rollback, expiry/identity/budget/deadline, conservative anchor/reuse and upgrade/replay passed; full evidence SQL race (14.604s), affected units/vet/sqlc/format/diff green. Next immediately: B3.2b bounded inventory cleanup + actual S3 recovery, then B3.3/C. Historical object-only sweep is insufficient; no SLO/provider/release claim.

- RVA07 B3.1 2026-10-11: quarantine ingress/erasure fence LOCAL_DONE / INTEGRATION_PENDING; whole phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). Session row lock serializes ingress with expiry, fresh live-transfer check prevents deletion/marker while admitted upload remains active; sweep uses same guard. SQL RED false erasure ack closed; late completion/recovery and concurrent erasure lock/budget proofs pass. Full evidence SQL race, dependent privacy erasure/restore, affected units/vet/sqlc/diff/secret checks green; forward outage assertion strengthened to require error. Next: B3.2 final-output inventory/verification lease, then B3.3 predeadline dispatch and C proofs. No provider/SLO/release claim or existing-soak mutation.

- RVA07 B2.2 2026-10-11: bounded owner training erasure LOCAL_DONE / INTEGRATION_PENDING; whole phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). Append-only 000067 durable keyset cursor with nonce/absence ack transaction, 32-copy/45s attempts, no owner inventory snapshot, OFF revision/deadline retained on retry. Real SQL/S3 65-copy restart 32/32/1, foreign-owner control, bytes restored beside PURGED receipts, 12-way owner claim, cursor rollback/stale nonce and upgrade/replay all green; full privacy SQL race, affected unit/vet/sqlc/diff/secret checks passed. Next: B3 evidence late-writer fences, immutable inventory-before-output and predeadline dispatch; then C wall-time/provider/backup proofs. Collection OFF; no VPS/device/release claim.

- RVA07 B2.1 2026-10-11: bounded training-output disposal LOCAL_DONE / INTEGRATION_PENDING; whole phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). Immediate every-minute cleanup, 32-item/45s pass, 5s copy transfer, nonce lease/terminal ack, append-only 000066 partial due index. Real SQL/S3 provider-failure/restart/abandoned-writer/stale-ack cases passed; 12-way claim uniqueness and 65-row 32/32/1/0 drain passed; 10k terminal receipt EXPLAIN uses partial index/no inventory seq scan; upgrade/replay and full privacy SQL race plus units/vet/sqlc/diff/secret green. Next: B2.2 owner erasure batching, B3 evidence late-writer/deadline fixes, then C proofs. No wall-time/provider/release claim; collection OFF.

- RVA07 A/B1 2026-10-11: truthful cleanup/health LOCAL_DONE / INTEGRATION_PENDING; whole phase IN_PROGRESS. [Evidence](../backend/evidence/community-safety/RVA07_DEADLINE_ENFORCEMENT.md). RED three false-success cases reproduced; GREEN query errors preserve UNKNOWN and partial effects, storage failures error after other cleanup, jobs refuse unknown/partial/overdue acknowledgements. Actual PostGIS observation failures retain deletion markers; affected evidence/privacy/API/worker/ops race suites, vet/diff/secret checks green. Next: B2 bounded training disposal and durable per-copy acknowledgements, then evidence late-writer fencing/deadline dispatch and C proofs. Historical soaks untouched, no deadline/release claim.

- RVA06 2026-10-11: fail-closed minimization correction LOCAL_DONE / INTEGRATION_PENDING; actual content minimizer PENDING, collection OFF. [Evidence](../backend/evidence/community-safety/RVA06_MINIMIZATION_GATE.md). RED valid JPEG incorrectly PASS; GREEN valid/EXIF layouts UNCERTAIN with zero output, malformed/oversize/bomb/cancelled refused; explicit production source refusal has no enable switch. Real SQL proves no inventory creation, affected race suites/vet/diff/secret checks green. Zaflas controller identity/contact supplied by owner recorded in draft terms; rights-channel handling/public activation still unverified. Next: RVA07 bounded cleanup, deadline margin and truthful health. Historical soaks untouched; no VPS/device/release claim.

- RVA05 2026-10-11: inventory/output protocol LOCAL_DONE / INTEGRATION_PENDING. [Evidence](../backend/evidence/community-safety/RVA05_OUTPUT_RECOVERY.md). Append-only 000065 immutable key and nonce-fenced output receipts; pending inventory unselectable, replay repairs original key/date, bounded bytes/time/five attempts, READY only after hash and consent revalidation. Real SQL/private S3 lost-reply/corruption/crash/revoke-during-PUT/stale-cleanup matrix green, 12-way lease and upgrade/queued-revision regressions green; full privacy SQL race, unit/consumers/vet/sqlc/diff/secret checks passed. RVA04 physical-output race evidenced. Next: RVA06 honest minimization/collection OFF gate, then RVA07 bounded recurring cleanup/deadline health. No VPS/device/release claim.

- RVA04 2026-10-11: A/B database checkpoint LOCAL_DONE / INTEGRATION_PENDING; physical-output acceptance continues in RVA05. [Consent fence evidence](../backend/evidence/community-safety/RVA04_CONSENT_FENCE.md). Append-only 000064 durable owner/revision receipts, shared owner transaction lock, exact current consent revalidation at derivative commit and restore-safe selection. OFF excludes all rows immediately; tombstone writes limited to 32 plus recurring 32-row repair, immutable first covering deadline per image. RED stale-commit/active-after-OFF reproduced; real SQL race, paused sanitizer, 65-row bounds/restart/stale restore, regrant deadlines, rollback/upgrade and full privacy SQL race suites green; unit/consumer/vet/sqlc/diff checks green. Next: RVA05 crash-consistent inventory/object output and paused-write race. Collection remains OFF; purge/retention metrics and external proofs remain owed.

- RVA03 2026-10-11: LOCAL_DONE / INTEGRATION_PENDING. [Erasure recovery contract/evidence](../backend/evidence/community-safety/RVA03_ERASURE_RECOVERY.md). Append-only 000063 atomically records account-wide intent including binding-free accounts, backfills historical deleted rows and enables every-minute session-independent repair with expiring nonce-fenced leases/per-contributor checkpoints. FAILED privacy work resumes using its persisted owner reference; storage errors/remaining evidence batches cannot falsely complete. Social/evidence reads bounded; HTTP dispatch-failure acceptance, cancellation, 12-way SQL lease race, restart/stale nonce/backfill, physical cleanup and full four-adapter SQL race suites green; affected unit/consumer/vet/sqlc/format/diff checks green. Next: RVA04 transactional training consent/revocation fence. Late media writes/deadline health and resource-throughput closure remain RVA07/RVA12/final acceptance; no VPS/device/release claim.

- RVA09 2026-10-11: LOCAL_DONE / INTEGRATION_PENDING. [JWKS admission evidence](../backend/evidence/community-safety/RVA09_JWKS_ADMISSION.md). Unknown-key refreshes and failed provider loads bounded per issuer/30s; context-cancellable independent single-flight, map cap and no network-held global mutex. RED 20 invalid tokens→21 fetches; GREEN same/unique bursts≤2, outage budget, provider isolation, cancellation and rotation/TTL/redirect suites pass under race; vet/format/diff green. Next: RVA03 durable account-wide erasure recovery and retryable privacy failures. Link-route/global admission remains explicitly tracked in RVA12.

- RVA02 2026-10-11: LOCAL_DONE / INTEGRATION_PENDING. [Admission contract/evidence](../backend/evidence/community-safety/RVA02_ADMISSION_CONTRACT.md). Append-only 000062 records unlinkable admission timestamps atomically; deleting credentials cannot refund quota, bounded prune/backfill, compatibility stores share admission. RED create/delete bypass closed; account race suites and real SQL 115 racers→100 admissions, expiry/restart-view and active/deleted upgrade/backfill passed; sqlc/vet/format/diff green. Next: RVA09 unknown-kid refresh admission, then RVA03 resumable erasure. RVA00 reconciliation completed by normal main synchronization and canonical-document checkpoint; no runtime phase or external proof inferred from that checkpoint.

- RVA01 2026-10-11: LOCAL_DONE / INTEGRATION_PENDING, bounded API photo ingress replaces production direct-storage PUT issuance. [Contract/evidence](../backend/evidence/community-safety/RVA01_INGRESS_CONTRACT.md). Exact bytes/hash checked before S3; four in-flight transfers, five durable attempts, nonce-fenced actual-byte receipt/replay; append-only 000061 upgrade. Evidence/API/server/OpenAPI unit race checks, three real-SQL ingress tests and both real API/worker/S3 flows passed; sqlc/vet/format/diff/secret checks green. Local disposable project only; no VPS/phone/soak change. Next: RVA02 deletion-independent signup accounting; later provider/device/release acceptance remains owed.

- RVA00 opening 2026-10-11: IN_PROGRESS; user authorized complete security/offline corrections, GitHub publication and protected main merge. Review baseline `603d5ba`; fetched main/dev now `5dc9b2b` (prior PR129 and landing integrated). Preserve prior evidence/soaks and reconcile canonical planning files; synchronize maintained dev before runtime edits. Next: RVA01 actual ingress enforcement, immediate critical tests; no unrelated VPS workloads.

- Independent security/offline reassessment 2026-10-11: REVIEW_LOCAL_DONE / REMEDIATION_NOT_STARTED at immutable tracked dev `603d5ba8acd32da6b5b0de72988ac614e8c18ee6`. [Assessment](../security/SECURITY_OFFLINE_REASSESSMENT_2026-10-11.md), [complete RVA00–14 + CS15–25 correction plan](SECURITY_OFFLINE_CORRECTION_PLAN_2026-10-11.md), [sanitized reproductions](../security/evidence/reassessment-2026-10-11/README.md). Fourteen review items: five HIGH (two conditional training activation blockers), eight MEDIUM and one functional integration blocker; no demonstrated CRITICAL. CS15 A/B1 are planning-only partial work; CS16–25 are not accepted, with reusable partial CS21/22 dependencies. 71 Go packages, eight real-PostGIS packages and 94 selected JVM tests passed; added characterizations reproduced admission/erasure/training/retry/redirect/JWKS gaps and bounded parser heap exhaustion. Passing characterization is not safe behavior. No runtime fixes, VPS/phone, existing-soak changes, publication or release claim; unique disposable local DB removed. Existing SEC05-E/CS10C autonomous-deadline proof and phone/scan obligations remain pending. Next: user chooses implementation agent; then RVA00 current-head/ownership reconciliation, not automatic feature activation.

- PR128/PR129 delivery recovery 2026-10-11: LOCAL_DONE / INTEGRATION_PENDING in isolated `/tmp/abastevo-pr-delivery-20261011` on dev; primary foreign edits preserved. [Failure/repair evidence](../backend/CI_PR128_PR129_RECOVERY.md). Normal current-main merge preserves history; immutable loopback MinIO test fixtures fix the real image-pull failure; cold-pull stdout/stderr parsing and a cleanup-capture defect also repaired, with both real-S3 race matrices and leak regression green; verified renamed-origin allowlist passes 45 flow guards plus batch harness. Required Quick verification and every applicable CI signal must pass on exact head/base before merging PR128 into dev and cumulative PR129 into main. Next: publish repair, reconcile landing, validate and guarded delivery. No deploy/release/device/soak acceptance.

- Security remediation SEC09-E 2026-10-11: LOCAL_DONE / INTEGRATION_PENDING on maintained dev, final protected delivery (merge gated on green). [Evidence](../backend/evidence/community-safety/SEC09E_PROTECTED_DELIVERY.md). Origin repointed to Abastevo per move; fast-forward push of dev (SEC01-D→SEC09-E) updating reused PR #129; foreign-dirty/untracked planning files excluded from every commit. Merge only via finish helper when Quick verification is green; wiki mirror after. Owes: PR checks, SEC05-E soak (due 2026-10-12T19:11:39Z), CS10C soak (due 2026-10-12T16:11:11Z), SEC08-D phone matrix, edge-Caddy port conflict. Next action: merge when green, mirror wiki, run due soak proofs.

- Security remediation SEC09-D 2026-10-11: LOCAL_DONE on maintained dev, staging runbook + rollback doc (no rollout). [Evidence](../backend/evidence/community-safety/SEC09D_STAGING_RUNBOOK.md). Abastevo-only rollout order (db→migrate→api/worker→caddy) with measured-headroom rate bounds (5 req/s history explicitly not a budget), stop conditions (p95/5xx/backlog/overdue/OOM), forward-only migration rollback, and no-shared-workload antiblast. Doc-only; diff-check clean, no secrets/PII. Next microtask: SEC09-E final protected delivery.

- Security remediation SEC09-C 2026-10-11: LOCAL_DONE on maintained dev, integration + fault evidence at 375e213 (no product change). [Evidence](../backend/evidence/community-safety/SEC09C_INTEGRATION_FAULT.md). New real-PostGIS 000060 upgrade proof (applies alone, live family intact, history writable, replay empty); Android domain/application/data/app + assembleDebug re-green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC09-D staging runbook and rollback.

- Security remediation SEC09-B 2026-10-11: LOCAL_DONE on maintained dev, dependency/container scans (no dep change). [Evidence](../backend/evidence/community-safety/SEC09B_DEPENDENCY_SCANS.md). govulncheck clean, cargo audit exit 0 (25 crates), Android catalog pinned with empty dep diff since 1b67367, all images digest-pinned, release minify+shrink with one exported activity and no pprof/debug routes. Scan-only; diff-check clean, no secrets/PII. Next microtask: SEC09-C integration and fault evidence.

- Security remediation SEC09-A 2026-10-11: LOCAL_DONE on maintained dev, current-head review at 29f2725 (one harness race fix). [Evidence](../backend/evidence/community-safety/SEC09A_CURRENT_HEAD_REVIEW.md). 71 unit packages green; real-PostGIS -race green across account/community/privacy/feedback/identity incl. real-HTTP suites; new MaxConns=2 saturation proof (reads proceed behind stuck advisory lock, same-account waits bounded); feedback IDGen harness race fixed. Edge Caddy test env-blocked (port 18081 squatted, pre-existing at 1b67367). Next microtask: SEC09-B dependency and container scans.

- Security remediation SEC08-D 2026-10-11: OWED / PROOF_OWED on maintained dev, real-phone staging matrix frozen but NOT RUN (no device/window). [Evidence](../backend/evidence/community-safety/SEC08D_PHONE_MATRIX.md). 4-case matrix (Fake GPS refusal, TEST_ONLY exclusion, genuine pass, offline capture) + entry criteria recorded; verdict cells empty, nothing fabricated; no photos/coords in Git. Doc-only; diff-check clean. Next microtask: SEC09-A current-head review.

- Security remediation SEC08-C 2026-10-11: LOCAL_DONE on maintained dev, attestation decision doc (no code/dependency change). [Evidence](../backend/evidence/community-safety/SEC08C_ATTESTATION_DECISION.md). Platform attestation declined for this batch (proves app integrity, not GPS; offline captures can't carry verdicts; device-exclusion + privacy/support costs); OS-flag + recompute path stays the only verifier; grep-verified no fingerprint/blacklist/settings-read/GPS-retention/permission introduced. Doc-only; diff-check clean, no secrets/PII/GPS. Next microtask: SEC08-D real-phone staging matrix.

- Security remediation SEC08-B 2026-10-11: LOCAL_DONE on maintained dev, adversarial synthetic client tests (no behavior change). [Evidence](../backend/evidence/community-safety/SEC08B_ADVERSARIAL_CLIENT.md). Separate c-adv/tok-synth-adv client through signed Submit + HTTP route: consistent fabrication lands VERIFIED/NEAR (residual measured); replay converges to one row; 50km teleport keeps bands with suspect flag; stale/coarse refuse forged with zero rows; missing/unreviewed degrade to UNKNOWN; garbage verdict refuses, honest SIMULATED stays silent. 8 new tests; community tree green; build/vet/gofmt/diff-check clean, secrets passed. Next microtask: SEC08-C attestation decision.

- Security remediation SEC08-A 2026-10-11: LOCAL_DONE on maintained dev, location assurance policy doc (no code change). [Evidence](../backend/evidence/community-safety/SEC08A_LOCATION_ASSURANCE.md). Five layers distinguished (request/key, OS mock, geometry, station quality, presence); self-attestation never yields HIGH trust alone (server recompute refuses forged VERIFIED, missing source caps UNKNOWN, only VERIFIED carries proximity); 150m geometry + unknown/manual outcomes preserved; contract versioning rule frozen. Doc-only; diff-check clean, no secrets/PII/GPS. Next microtask: SEC08-B adversarial synthetic client test.

- Security remediation SEC07-C 2026-10-11: LOCAL_DONE on maintained dev, Android refresh serialization (no transport/backend change). [Evidence](../backend/evidence/community-safety/SEC07C_ANDROID_REFRESH.md). 8-racer proof pins max API overlap 1 behind the mutex shape (Direct lock overlaps 8, probe removed); ambiguous replies cost one call with the session kept; new session-invalid verdict clears local authority without key fallback; foreign-account clears. 5 new + denial matrix extended; :application + :domain full green; diff-check clean, no secrets/PII. Next microtask: SEC08-A location assurance policy.

- Security remediation SEC07-B 2026-10-11: LOCAL_DONE on maintained dev, per-issuer JWKS expiry (no session/Android change). [Evidence](../backend/evidence/community-safety/SEC07B_JWKS_EXPIRY.md). Per-issuer stamps (one provider never prolongs the other); exactly one TTL-bypassing refresh on unknown kids (rotation verifies, removal refuses, outage unavailable); redirects never followed (copied-client pin, zero attacker requests). 4 new + pre-existing OIDC suites green; build/vet/gofmt/diff-check clean, secrets passed. Next microtask: SEC07-C Android refresh serialization.

- Security remediation SEC07-A 2026-10-11: LOCAL_DONE on maintained dev, proof-based refresh reuse (no JWKS/Android change). [Evidence](../backend/evidence/community-safety/SEC07A_REFRESH_REUSE.md). Random invalid refresh refuses session-invalid with the family live; history-hit replay revokes with session reuse; migration 000060 hashed history (8-deep, same-tx archive+prune) + sqlc vet/generate clean + 401 mapping. 3 unit + 2 real-PostGIS (-race) new tests; full account unit + integration green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC07-B per-issuer JWKS expiry.

- Security remediation SEC06-E 2026-10-11: LOCAL_DONE on maintained dev, measured histories (no prod-code change). [Evidence](../backend/evidence/community-safety/SEC06E_MEASURED_HISTORIES.md). Counting-DAO pins at 1/100/1k/10k rows: dispatch ≤50, history ≤200, zero listAll on hot paths, retention batch ≤500; 10k pages 1ms/98MiB, flat across 100x growth; offline restart serves head + history, reconnect takes one bounded page; honest 19 676-row parse within ceilings. New 3/3 + :data full green; diff-check clean, no secrets/PII. Next microtask: SEC07-A refresh reuse proof.

- Security remediation SEC06-D 2026-10-11: LOCAL_DONE on maintained dev, retry/refusal/attempt budgets (no sync-flow change). [Evidence](../backend/evidence/community-safety/SEC06D_RETRY_BUDGETS.md). Attempts park at 25 (tick MAX, history kept) with deterministic ±25% jitter; 429 Retry-After honored (bounded, malformed falls back); permanent allowlist parks distinctly, transient keeps backoff; fresh nonces under stable operation id; FIFO fairness across owners. 8 new tests; domain/application/data/app full suites + assembleDebug green; diff-check clean, no secrets/PII. Next microtask: SEC06-E measured histories.

- Security remediation SEC06-C 2026-10-11: LOCAL_DONE on maintained dev, outbox SQL bounds + pages + retention (no retry change). [Evidence](../backend/evidence/community-safety/SEC06C_OUTBOX_BOUNDS.md). Single-find/bounded-page dispatch (keyset first, zero listAll), newest-first history pages, terminal retention (newest 1000, batch 500, amortized on writes); dispatch/owner indexes + migration 9→10 (schema 10.json aligned); owner/origin/revision/lease semantics kept. 5 new proofs + migration test; domain/application/data/app full suites green; diff-check clean, no secrets/PII. Next microtask: SEC06-D retry and retention budgets.

- Security remediation SEC06-B 2026-10-11: LOCAL_DONE on maintained dev, XLSX expansion ceilings (no outbox/retry change). [Evidence](../backend/evidence/community-safety/SEC06B_XLSX_CEILINGS.md). Coordinates validate pre-allocation (ZZZZZZ1 refuses, XFD1048576 parses); entry bytes cap declared+actual, 1M strings/32 KiB texts/1024 worksheets caps, missing entries refuse, init failure closes ZIP. 8 crafted-zip tests (all tiny/fast); honest fixtures (incl. 19 676-row file) re-green; :data full suite green; diff-check clean, no secrets/PII. Next microtask: SEC06-C outbox SQL bounds.

- Security remediation SEC06-A 2026-10-11: LOCAL_DONE on maintained dev, source-origin + transfer bounds (no parser/outbox change). [Evidence](../backend/evidence/community-safety/SEC06A_SOURCE_BOUNDS.md). Approved gov.br origins (HTTPS/exact host/path, no userinfo) gated pre-fetch and re-checked post-redirect; 64 MiB capped copy + atomic publish (partials purge); sanitized names (traversal pre-fetch refusal); 4 MiB capped listing read; loopback seam (production empty). 8 new tests (zero-request refusals, redirect/file-hygiene, oversize, names); :data full suite green; diff-check clean, no secrets/PII. Next microtask: SEC06-B XLSX ceilings.

- Security remediation SEC05-E 2026-10-11: SOAK_RUNNING / PROOF_OWED on maintained dev (time-gated; mechanism proven, wall-clock proof owed). [Evidence](../backend/evidence/community-safety/SEC05E_EVERY_COPY_SOAK.md). Fake-clock every-copy compliance green (all categories deleted+marked, quiet signal); live soak seeded 2026-10-11T19:11:39Z (disposable PostGIS + MinIO, 1 original + 1 final, pre-due audit quiet) with env-gated TestSEC05ESoakVerify due after 2026-10-12T19:11:39Z (tripped pre-sweep → real Sweep → quiet receipt). Do NOT claim compliance before that run. Next microtask: SEC06-A source-origin enforcement (or the due soak proof).

- Security remediation SEC05-D 2026-10-11: LOCAL_DONE on maintained dev, backup assessment + selective exclusion (no soak). [Evidence](../backend/evidence/community-safety/SEC05D_BACKUP_ASSESSMENT.md). Room database excluded from Auto Backup, cloud backup and device transfer (was backup-eligible with private metadata); prefs already excluded (pinned); cacheDir photos need no rule; packs re-download, restore boots fresh-install path. JVM XML pins + :app full suite green; diff-check clean, no secrets/PII. Next microtask: SEC05-E every-copy observation (time-gated).

- Security remediation SEC05-C 2026-10-11: LOCAL_DONE on maintained dev, Android transient-cache cleanup (no backup/soak change). [Evidence](../backend/evidence/community-safety/SEC05C_TRANSIENT_CACHE.md). Cache policy v1: stale `.tmp` purge past 1 h file-age grace (live writers safe, rollback keeps), delete(id) drops tmp sibling, expiry = portable 24 h OR capture >1 h future (skew live, tamper refused+deleted); lifetime survives restart via envelope; powered-off limitation retained. 4 new cache tests; :data full suite green; diff-check clean, no secrets/PII. Next microtask: SEC05-D backup assessment.

- Security remediation SEC05-B 2026-10-11: LOCAL_DONE on maintained dev, scheduling margin + catch-up + alarms (no cadence/schema change). [Evidence](../backend/evidence/community-safety/SEC05B_SCHEDULING_CATCHUP.md). Post-pass Overdue in SweepReport (degrades to zero, never fails cleanup); warn-level overdue alarm with worst-lag stamps; batch-honoring fakes mirror bounded SQL. 250-backlog converges in 3 runs then quiet; namespace signal alarms correctly. Evidence suites green; build/vet/gofmt/diff-check clean, secrets passed. Next microtask: SEC05-C Android transient-cache cleanup.

- Security remediation SEC05-A 2026-10-11: LOCAL_DONE on maintained dev, overdue originals in audit + breaker (no scheduling/Android/soak change). [Evidence](../backend/evidence/community-safety/SEC05A_OVERDUE_ORIGINALS.md). New OverdueOriginals (strict created_at anchor, terminal/job agnostic) + AuditOverdue both namespaces + Unhealthy() + cmd/api breaker trips on either (sqlc vet clean). Originals-alone trip (AUD-06 repro gone), healthy-zero needs both, marked-deleted silences; real-PostGIS 48 h proof. Evidence unit + targeted integration (-race) green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC05-B scheduling margin and catch-up.

- Security remediation SEC04-D 2026-10-11: LOCAL_DONE on maintained dev, replay proofs + erased-owner local cleanup (SEC04 exit; no worker-scope change). [Evidence](../backend/evidence/community-safety/SEC04D_REPLAY_LOCAL_CLEANUP.md). Backend: lost-response convergence, partial-restore scope-exact replay + stamps, 8-way parallel duplicate deletions converge. Android: purgeOwned (fail-closed, no schema change), photo session/cache purge, OwnerLocalErasure orchestration, gateway checkScope send-guard (pre-existing, load-bearing), ViewModel logout/delete wiring (failure keeps queue). 3 backend + 7 Android new tests; domain/application/data/app full suites + backend unit green; diff-check clean, no secrets/PII. Next microtask: SEC05-A overdue inventory.

- Security remediation SEC04-C 2026-10-11: LOCAL_DONE on maintained dev, attribution + aggregates + training split (no worker-scope change). [Evidence](../backend/evidence/community-safety/SEC04C_ATTRIBUTION_TRAINING.md). DeleteAccountKeys (sqlc vet clean) minimizes key verifiers atomically (audit row stays); real-SQL proofs: post-delete key login key-invalid with free handle, social composition (erased rows gone, surviving reply verbatim, stats rebuilt to remaining author, foreign denominators survive, re-erase zero, training counts untouched). Account/feedback/privacy unit + integration (-race) green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC04-D local cleanup + replay proofs.

- Security remediation SEC04-B 2026-10-11: LOCAL_DONE on maintained dev, deletion linkage (no training split, no local sweep). [Evidence](../backend/evidence/community-safety/SEC04B_DELETION_LINKAGE.md). Migration 000059 intents (append-only, sqlc vet clean) written atomically with revoke+unlink; RequestErasure port (nil refuses closed pre-mutation); DeletionReceipt (intent, not completion); cmd/api wires privacy intents per contributor (deterministic submissions, durable jobs) + sync feedback erasure. 4 unit + HTTP receipt + 2 real-PostGIS proofs (capture/revoke/replay + wired ledger/tombstone composition); account/feedback/privacy unit + integration (-race) green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC04-C attribution + aggregates + training split.

- Security remediation SEC04-A 2026-10-11: PLANNING-ONLY, LOCAL (no behavior change). [Inventory](../backend/evidence/community-safety/SEC04A_ERASURE_INVENTORY.md). Frozen linked-owner inventory (account/contributor/social/media/queue/credential/alias/backup), retention exceptions (verifiers 24h, sessions 7d, audit 180d, copies 24h, hashes 90d, ledger persists) and receipt-vs-acknowledgement rule; bindings must be captured before unlinking (AUD-05 root). Wiring rides SEC04-B/C/D. Next microtask: SEC04-B intent + revoke + enqueue.

- Security remediation SEC03-D 2026-10-11: LOCAL_DONE on maintained dev, namespace + monitoring (SEC03 exit; no provider change). [Evidence](../backend/evidence/community-safety/SEC03D_NAMESPACE_MONITORING.md). 8-case namespace matrix (q/ vs f/ vs public/traversal); worker Download bound pin (MaxBytes+1); IntakeHealth (reserved/accepted/rejected declared buckets + oldest incomplete, secrets-free) in SweepReport + worker log, 3 new SQL aggregates (sqlc vet clean); rejected-original reconciliation pin; real-PostGIS health proof exact. Evidence unit + integration (-race) green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC04-A erasure inventory.

- Security remediation SEC03-C 2026-10-11: LOCAL_DONE on maintained dev, provider-compatible matrix over real S3 (no provider/production change). [Evidence](../backend/evidence/community-safety/SEC03C_PROVIDER_MATRIX.md). Disposable loopback MinIO, real SigV4/HTTP: max exact ok, max+1 transport-accepts/bounded-read-refuses, mismatch stored (claim owns it), wrong type/key 403, missing 404/delete-nil, backdated URL 4xx, interrupted stays absent, reuse overwrites (claim decides), 4 parallel complete; full verify loop (honest READY + retrievable final, oversize/mismatch refuse with zero final). Evidence suites green; vet/gofmt/diff-check clean, secrets passed. Next microtask: SEC03-D namespace + monitoring.

- Security remediation SEC03-B 2026-10-11: LOCAL_DONE on maintained dev, transfer-boundary decision + pins (no provider/transport change). [Evidence](../backend/evidence/community-safety/SEC03B_TRANSFER_BOUNDARY.md). Keep direct presigned PUT (UNSIGNED-PAYLOAD proven uncappable; no exact-length PUT without provider proof; no S3 POST on R2) + enforce at verify (MaxBytes+1 one-download bind, Snapshot cap, claim/dims/sanitize gates) + SEC03-A admission + 5-min TTL + quarantine-only keys + sweep reconciliation. 3 new boundary pins (declared-never-widens, substitution-mismatch, absence-transient); evidence suites green; vet/gofmt/diff-check clean, secrets passed. Next microtask: SEC03-C provider-compatible matrix.

- Security remediation SEC03-A 2026-10-11: LOCAL_DONE on maintained dev, atomic reservations + byte budgets (no transfer-boundary change). [Evidence](../backend/evidence/community-safety/SEC03A_ATOMIC_RESERVATION.md). ReserveBudget (count + live ISSUED/VERIFYING bytes, expiry releases) admits with the row in one tx/lock (pg_advisory_xact_lock per contributor); replays converge at full quota, divergent bodies conflict. Frozen MaxReserveBytesPerDay (10x3MiB); new SumLiveBytesSince (sqlc vet clean). Parallel 12-over-9 admits exactly 10 (unit + real PostGIS); byte bind/release proven. Evidence unit + integration (-race) green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC03-B transfer boundary.

- Security remediation SEC02-C 2026-10-11: LOCAL_DONE on maintained dev, global/per-client split + viability + edge proof (no VPS edge change). [Evidence](../backend/evidence/community-safety/SEC02C_BUDGET_SPLIT.md). Process-local GlobalBreaker (hourly ceilings, 503 overloaded + Retry-After, zero disables) sheds before fairness (429); ANPFUEL_GLOBAL_QUOTA_* (default disabled, validated) + cmd/api wiring. Viability pins (10 units; 6/60 + 4/10 headroom); real docker Caddy→API proof PASS (observed loopback billed, direct forgery bills peer). Identity/config unit + integration (-race) green; build/vet/gofmt/diff-check clean, secrets passed. Next microtask: SEC03-A atomic reservations.

- Security remediation SEC02-B 2026-10-11: LOCAL_DONE on maintained dev, trusted-proxy resolver + wiring (no global breaker, no VPS edge change). [Evidence](../backend/evidence/community-safety/SEC02B_TRUSTED_RESOLVER.md). Narrow Resolver (trusted immediate peer only, last-valid chain entry, peer fallback, fail-closed peers) + ANPFUEL_TRUSTED_PROXIES (default trust-nothing, startup-validated) + cmd/api wiring + env examples. 3/3 SEC02-A regressions GREEN + 13-case resolver matrix + parse/config matrices; identity unit + integration (-race) green; build/vet/gofmt/diff-check clean, secrets passed. Next microtask: SEC02-C global/per-client split + viability.

- Security remediation SEC02-A 2026-10-11: LOCAL_DONE on maintained dev, proxy-fairness regressions only (no behavior change). [Evidence](../backend/evidence/community-safety/SEC02A_PROXY_REGRESSIONS.md). 3 skipped tests (shared-bucket flaw + spoof guard + malformed fallback), flaw body proven FAILING unskipped then restored skipped per repo precedent. Suites green; vet/gofmt/diff-check clean, secrets passed. Resolver + budgets ride SEC02-B/C. Next microtask: SEC02-B trusted resolver + wiring.

- Security remediation SEC01-D 2026-10-11: LOCAL_DONE on maintained dev, retry/rollback + stable 429s (no SMTP, no SEC02+ scope). [Evidence](../backend/evidence/community-safety/SEC01D_RETRY_ROLLBACK.md). Mail-failure DeleteCode rollback (zero rows/budget, recovered retry 1/1); cooldown duplicates converge (1 row/1 mail); exhausted global nil with zero growth; hourly rotation re-admits; duplicate comment-ID replays converged, foreign reuse fails closed; account 429s carry Retry-After (aggregate 3600, session 60) and feedback 429s (report exact, comment 60) with stable codes and no identity echo. Unit + real-PostGIS (-race) green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC02-A proxy-fairness regressions.

- Landing PR #128 reconciliation 2026-10-11: LOCAL_RECONCILED / REMOTE_CHECK_PENDING, isolated `codex/landing-reconcile-dev`; [evidence](../release-evidence/landing-redesign.md). Ordinary merge of published dev `47b5464` into landing `04dbdf8`; sole PROGRESS conflict resolved retaining both parents. Typecheck/build/check + 40 landing tests PASS; backend/app and in-progress local dev work preserved. User-authorized landing→dev merge requires current-head Quick verification; main/deploy/index remain pending.

- Security remediation SEC01-C 2026-10-11: LOCAL_DONE on maintained dev, atomic admission + budgets (no SMTP, no SEC01-D matrix). [Evidence](../backend/evidence/community-safety/SEC01C_ATOMIC_ADMISSION.md). Frozen: aggregate mail 20/h, key-usernames 100/h, live sessions 10/account, live comments 100/account; ValidEmail + ParseComment before any store IO (zero lookups for malformed); TryIssue/TryCreateKey/TryCreateFamily/TryInsert under one lock/tx (pg_advisory_xact_lock); purges with exact counts (codes ≤24h after expiry, families ≤7d after absolute). 6/6 SEC01-A regressions GREEN (session rewritten to cap+second-account) + 5 new unit + 4 new real-PostGIS parallel/purge tests; account/feedback unit + integration (-race) green; build/vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: SEC01-D real-SQL flood/rollback/retry proofs.

- Security remediation SEC01-B 2026-10-11: LOCAL_DONE on maintained dev, bounded mailbox + config-gated delivery (no SMTP, no aggregate bounds). [Evidence](../backend/evidence/community-safety/SEC01B_MAILBOX_BOUND.md). Memory outbox ring-capped at 100 (oldest drops); Service.MailEnabled off-by-default answers nil before any store write (zero rows/mail, no oracle); ANPFUEL_MAIL_MODE off|memory (smtp refused); cmd/api wires cap + flag. New cap/gate/config tests green incl. RED pre-GREEN; existing fixtures opt in explicitly; account unit + integration (-race) green; build/vet/gofmt/diff-check clean, secrets passed. Next microtask: SEC01-C atomic admission + budgets.

- Security remediation SEC01-A 2026-10-11: LOCAL_DONE on maintained dev, admission regressions only (no behavior change). [Evidence](../backend/evidence/community-safety/SEC01A_ADMISSION_REGRESSIONS.md). 6 skipped tests (account: aggregate email/usernames/sessions + invalid-address store cost; feedback: per-account flood + invalid-text pipeline cost), each proven FAILING unskipped then restored skipped per repo precedent. Suites green; vet/gofmt/diff-check clean, secrets passed. Bounds + fixes ride SEC01-B/C/D. Next microtask: SEC01-B mailbox bound.

- Security remediation SEC00-C 2026-10-11: PLANNING-ONLY, LOCAL (unpublished by policy; no push this turn). Versioned budget freeze in docs/planning/SEC00C_BUDGET_FREEZE.md (untracked): item/byte/time tables with code-anchored ceilings (quota v1, reserves 10/day, attempts 5, pages ≤100, 3MiB uploads, 8/16/64KiB bodies, OTP 600s, session 900s, challenges 5min, idempotency 7d, sessions 24h, presign 5min, JWKS 3600s, lease 5min/retry 1min, purge TTL 24h) + concurrency posture (3 still-reproducing gaps owned by SEC01–03) + PROPOSED section explicitly unadvertised + current 429/503 semantics. Next microtask: SEC01-A admission regressions.

- Security remediation SEC00-B 2026-10-11: PLANNING-ONLY, LOCAL (unpublished by policy; no push this turn). Route/capability inventory by access class in docs/planning/SEC00B_ROUTE_INVENTORY.md (untracked): anonymous reads, anonymous signed price writes (intentional, not a bypass), account-only social writes (incl. suggestions/claims verified), private owner reads, upload grants, restricted operator CLI (no HTTP). All classes verified against registrations/handler comments/cmd wiring; OpenAPI security:[] means proof-based, not anonymous. Next microtask: SEC00-C budget freeze.

- Security remediation SEC00-A 2026-10-11: PLANNING-ONLY, LOCAL (unpublished by policy; no push this turn). Reconciled dev at 9474af8 (3 ahead of origin/dev; PR129 code-only at a5674aa) and re-ran all 11 audit characterization probes against current source in an isolated worktree (since removed; checkout untouched): 9 unit + 2 real-SQL on a unique disposable PostGIS (since removed) — ALL still reproduce. Record in docs/planning/SEC00A_REPRODUCTION_RECORD.md (untracked). Probes must be inverted into regressions task by task, not blessed. Next microtask: SEC00-B route/capability inventory.

- Community safety CS15-B1 2026-10-11: PLANNING-ONLY, LOCAL (unpublished by policy; no push this turn). Safety-rows decision freeze in docs/planning/OFFLINE_DECISION_FREEZE_CS15B1.md (untracked): helpfulness/consent/promotion/purge/erasure/settings rows with frozen defaults, guest/account prerequisites, offline permit decisions, queue/receipt conflict states and completeness classes; limits verified in code (quota v1, idempotency 7d, job attempts 5, body 64KiB, sweep daily, TTL 24h). Non-safety areas ride CS15-B2; manifests ride CS15-C. Next microtask: soak SLO proof after 2026-10-12T16:11:11Z (then CS15-B2).

- Community safety CS15-A 2026-10-11: PLANNING-ONLY, LOCAL (unpublished by policy; no push this turn). Capability inventory written to docs/planning/OFFLINE_CAPABILITY_INVENTORY.md (untracked): 21 verified routes, 50 use-case files in 20 groups, 12 backend modules, ANP import pipeline, payment absent (grep-verified), consent/helpfulness panels confirmed unplaced. Nothing invented; no outcomes frozen (CS15-B) and no manifests/seeds/budgets (CS15-C). Next microtask: soak SLO proof after 2026-10-12T16:11:11Z (then CS15-B).

- Community safety CS12A 2026-10-11: LOCAL_DONE on maintained dev, consent live-stack integration tests only (no prod-code change). [Evidence](../backend/evidence/community-safety/CS12A_CONSENT_LIVE.md). Fixture mirrors cmd/api (real verifier/registrar/stores, P-256 proofs, disposable PostGIS). 7 tests green under -race: owner flow, live-clock replay convergence, cross-owner isolation, guest 401×3, revoked-key 401 with zero rows, tampered-body refusal with zero rows, stale-terms 400. Passed first run (composition already correct; RED was the missing coverage). Full privacy suites green; vet/gofmt/diff-check clean, secrets passed. No quota wiring (self-limiting singleton, recorded); no per-route metrics framework (recorded). Soak still SOAK_RUNNING. Next microtask: soak SLO proof after 2026-10-12T16:11:11Z.

- Community safety CS11C 2026-10-11: LOCAL_DONE on maintained dev, per-photo opt-in state + row + strings + tests only (no placement/wire/PT). [Evidence](../backend/evidence/community-safety/CS11C_PHOTO_OPT_IN.md). Opt-in state (default-off, consent-gated checks, wipe on clear, blank-safe, lock-guarded) + checkbox row (unchecked default, disabled with gate text, hoisted state, TalkBack helper) + 2 English strings. 10 JVM tests green (6+4) incl. RED pre-GREEN; full app suite + assembleDebug green. Flags ride the future producer payload; server enforces consent+scope at commit regardless. diff-check clean. Soak still SOAK_RUNNING. Next microtask: soak SLO proof after 2026-10-12T16:11:11Z (then CS12).

- Community safety CS11B 2026-10-11: LOCAL_DONE on maintained dev, consent ViewModel/panel/DI/strings only (no placement/per-photo/PT). [Evidence](../backend/evidence/community-safety/CS11B_CONSENT_VIEW.md). ViewModel (Guest/Loading/Content/Updating/Offline/Pending/Rejected; sync-first foreground; toggle grants/revokes then reloads server-exact; single flight; retry) + panel (switch, server state, terms/regrant/pending/delete/rights; guest renders nothing; TalkBack helper; no fixed heights) + 11 English strings + DI (client, binds, use-case; Hilt KSP green). 12 JVM tests green (8+4) incl. RED pre-GREEN; full app/data suites + assembleDebug green. KSP false-start from scripted edit bisected and resolved (byte-identical method builds clean). diff-check clean. Soak still SOAK_RUNNING. Next microtask: CS11C per-photo opt-in at review.

- Community safety CS11A 2026-10-11: LOCAL_DONE on maintained dev, training-consent Android client layers only (no ViewModel/panel/strings/DI/screens). [Evidence](../backend/evidence/community-safety/CS11A_CONSENT_CLIENT.md). Domain ports (frozen kinds, receipt, owner status, gateway) + OkHttp client (3 endpoints, strict parse, frozen privacy.* mapping, no client retries) + SharedPreferences state store (toggle OFF default, monotonic revision, pending revocation) + use case (guest zero-IO, grant fail-closed offline, revoke queues pending, syncPending uploads, per-decision revisions). 16 JVM tests green (8+8) incl. RED pre-GREEN; full data/application/domain suites green. diff-check clean. Soak still SOAK_RUNNING. Next microtask: CS11B consent ViewModel/panel/DI/strings.

- Community safety CS10D 2026-10-11: LOCAL_DONE on maintained dev, TRAINING erasure scope only (no soak claim/VPS/APK). [Evidence](../backend/evidence/community-safety/CS10D_TRAINING_ERASURE.md). ScopeTraining after EVIDENCE + migration 000058 ledger CHECK + EraseTrainingContributor (consent REVOKED past stored revision, derivatives tombstoned, bytes purged immediately, re-runs converge, failures → FAILED) + ListDerivativesByOwner + runScopes/replay/report + worker/ops wiring (explicit refusal when unconfigured). 3 new unit tests + scope-failure test + 5→6 ledger assertions green; real-PostGIS erasure journey proves revoke/tombstone/byte-purge/6 rows/converged repeat; full privacy suites green under -race. vet/gofmt/sqlc/diff-check clean, secrets passed. Soak still SOAK_RUNNING (untouched). Next microtask: soak SLO proof after 2026-10-12T16:11:11Z (then CS11).

- Community safety CS10C 2026-10-11: LOCAL_DONE (mechanics) / SOAK_RUNNING on maintained dev — promotion worker + byte namespace + purge + soak start (no erasure scope/VPS/APK). [Evidence](../backend/evidence/community-safety/CS10C_WORKER_PURGE_SOAK.md). Job `privacy-training-promote` (identifier-only payload, source hash-bind, replay converges, fail-closed) + `Propose` returns minimized bytes for the single namespace write + `privacy-training-sweep-daily` over `SweepRevoked` (due→delete→confirm; missing vs failures split; rows stay REVOKED) + SigV4 `TrainingObjects` + worker wiring (explicit refusal when unconfigured). New unit tests green incl. RED pre-GREEN; full privacy suites green under -race; build/vet/gofmt/diff-check clean, secrets passed. Soak seeded 16:11:11Z on local MinIO (named volume) + scratch PostGIS: overdue copy purged on first sweep (checked=1 purged=1), 3 fresh copies due 2026-10-12T16:11:11Z retained, control alive. SLO proof due after the wall clock — do NOT claim before TestSoakVerify then. Next microtask: erasure TRAINING scope; then soak proof.

- Community safety CS10B 2026-10-11: LOCAL_DONE on maintained dev, derivative persistence + sanitizer adapter only (no job/worker/bytes/soak/APK). [Evidence](../backend/evidence/community-safety/CS10B_DERIVATIVE_STORE.md). Migration 000057 inventory (PK image+revision, ACTIVE/REVOKED, append-only) + sqlc derivatives.sql (regenerated, vet clean) + `PromotionStore` (converged puts, owner-scoped reads, guarded revoke mark) + media-mapped sanitizer (PASS minimizes, all refusals FAIL, UNCERTAIN reserved). 8 new tests green under -race on real PostGIS (round trip, convergence/conflict, 10-way concurrency, revoke flow, 000057-only upgrade, non-destructive pins, JPEG pass/fail-closed); full privacy suites green. vet/gofmt/sqlc/diff-check clean, secrets passed. Job handler deferred to CS10C (no queue producer exists yet). Collection stays OFF. Next microtask: CS10C worker/byte namespace/24h soak/restore drill.

- Community safety CS10A 2026-10-11: LOCAL_DONE on maintained dev, promotion domain + worker contract only (no migration/SQL/adapter/scheduling/purge/soak/APK). [Evidence](../backend/evidence/community-safety/CS10A_PROMOTION_CONTRACT.md). Commit-time consent re-check (revoke between queue and commit refuses), future-only per-image scope, fail-closed verdicts (FAIL/UNCERTAIN refuse, outage writes nothing), identifier-only derivative rows, image-plus-revision convergence, idempotent owner revocation with restore-filter tombstones, shared SelectableForTraining filter. 15 new tests (domain 8 + application 6 + fixture 1) green under -race incl. RED pre-GREEN; build/vet/gofmt/diff-check clean, secrets passed. Collection stays OFF. Next microtask: CS10B derivative persistence + worker wiring.

- Community safety CS09C 2026-10-11: LOCAL_DONE on maintained dev, training-consent signed HTTP + OpenAPI only (no migration/worker/S3/APK). [Evidence](../backend/evidence/community-safety/CS09C_CONSENT_HTTP.md). 3 owner routes (grant/revoke/status, 200 + receipts, anonymous 401 first, strict 64KiB/UTF-8/canonical-revision bodies, frozen privacy.* codes, no-store, no leaks) + cmd/api wiring (guarded store, verifier caller, configured terms digest with provisional default) + OpenAPI 3 paths/4 schemas + 4 apicontract vectors. E2e correction: live-clock identical retries now converge (revision is the identity, instants are stamps); ConsentView gained terms_digest. 8 unit + router e2e on real PostGIS green under -race; CS09A/B suites re-green; apicontract green; build/vet/gofmt/diff-check clean, secrets passed. Collection stays OFF. Next microtask: CS10 promotion/24h expiry/physical deletion.

- Community safety CS09B 2026-10-11: LOCAL_DONE on maintained dev, training-consent persistence only (no HTTP/OpenAPI/worker/S3/APK). [Evidence](../backend/evidence/community-safety/CS09B_CONSENT_STORE.md). Migration 000056 singleton (`privacy_training_consents`, revision-guarded, append-only) + sqlc consent.sql (regenerated, vet clean) + `ConsentStore` (compile-time port assertion; upsert-greater/replay-identical/conflict-divergent/refuse-stale; owner no-oracle reads). 7 new real-PostGIS tests green under -race (round trip, revision guard, 10-way concurrency, owner isolation, application flow incl. stale-grant refusal, 000056-only upgrade with history intact, non-destructive pins); full privacy unit + integration suites green. vet/gofmt/sqlc/diff-check clean, secrets passed. Collection stays OFF. Next microtask: CS09C consent signed HTTP + OpenAPI.

- Community safety CS09A 2026-10-11: LOCAL_DONE on maintained dev, training-consent domain + server contract only (no migration/HTTP/worker/S3/APK). [Evidence](../backend/evidence/community-safety/CS09A_CONSENT_CONTRACT.md). Frozen TRAINING purpose + training-terms-v1, default-OFF absent, revision-ordered singleton (stale refuses, identical replays, divergent conflicts), future-only per-image scope, revoke stops selection atomically with purge signal, owner-bound no-oracle reads. 20 new tests (domain 10 + application 9 + fixture 1) green incl. RED pre-GREEN and -race concurrent revokes; vet/gofmt/build/diff-check clean, secrets passed. Collection stays OFF. Next microtask: CS09B consent persistence + signed HTTP/OpenAPI.

- Security audit 2026-10-11: AUDIT_LOCAL_DONE / REMEDIATION_NOT_STARTED; immutable dev snapshot `954a8d8`, no runtime changes. [Executive/source audit](../security/SECURITY_AUDIT_2026-10-11.md), [SEC00–SEC09 remediation microtasks](SECURITY_REMEDIATION_PLAN_2026-10-11.md), [sanitized reproductions](../security/evidence/audit-2026-10-11/README.md). Recorded 3 HIGH and 9 MEDIUM findings/conditional risks, no demonstrated CRITICAL: aggregate admission, direct-upload bytes and proxy quota fairness first; privacy erasure/retention, Android cache/parser/outbox, refresh/JWKS and GPS assurance next. All 67 tested Go packages and eight real disposable-PostGIS adapter packages passed; bounded probes reproduced defects; Go/Rust advisory scans reported zero findings, static/secret gates passed. Characterization PASS is not safe behavior. Concurrent CS02–CS07A construction reached `d04c96f` during review and is outside the tested snapshot; unfinished Android changes also require review. No VPS/phone, production data, deployment, GitHub/wiki publication or release certification. Next: SEC00 current-head reconciliation under the implementation scope; fixes remain planned.

- Community safety CS08B 2026-10-11: LOCAL_DONE on maintained dev, ViewModel/panel/DI/guest (no screen placement). [Evidence](../backend/evidence/community-safety/CS08B_HELPFULNESS_VIEW.md). ViewModel (Disabled/Guest/Idle/Loading/Content/Submitting/Rejected, stable id, single flight) + panel (counts/choice/notice, Denunciar delegates, guest card) + pure TalkBack helper + 15 strings + under_review DTO + DI bindings. ViewModel 6/6 + a11y 4/4 green; Hilt MissingBinding seen pre-DI, green after; debug compile successful. Next microtask: CS09 consent/inventory/rights.

- Community safety CS08A 2026-10-11: LOCAL_DONE on maintained dev, client layers only (no DI/ViewModel). [Evidence](../backend/evidence/community-safety/CS08A_HELPFULNESS_CLIENT.md). Ports (frozen choices/kinds/shapes) + usecase (flag gate, validations, fixed rejections) + flag store (default OFF) + HTTP client (routes, idempotency, kind mapping, fixed text). Application 6/6 + data MockWebServer 4/4 + full application suite green. diff-check clean. Next microtask: CS08B ViewModel/panel/DI/guest.

- Community safety CS07A 2026-10-11: LOCAL_DONE on maintained dev, harness + local runs only (no staging). [Evidence](../backend/evidence/community-safety/CS07A_BENCH_HARNESS.md). Bench (warmup + tally/cast/flip, mirrors anti-drift) stable 2x: tally p50 0.07ms/p95 0.28ms; cast p50 3.6ms/p95 8.9ms; flip p50 2.9ms/p95 15.2ms. Protocolo staging congelado (5×120s, warmup, teto 5rps, headroom 5%). vet/gofmt/diff-check clean, secrets passed. Next microtask: CS07B staging guard runs (authorized window).

- Community safety CS06 2026-10-11: LOCAL_DONE on maintained dev, isolation proofs only (no route/schema/migration). [Evidence](../backend/evidence/community-safety/CS06_PROJECTIONS_ISOLATION.md). Floor display vectors (unanimous/splits/edges/2^40); full helpfulness lifecycle leaves prices/ratings/inputs byte-identical; static pins (no cross-refs either direction; no DELETE on dispute/vote audit — 35d bounds ledger/media only). Unit + integration green. vet/gofmt/diff-check clean, secrets passed. Next microtask: CS07 benchmark/guardrails.

- Community safety CS05B 2026-10-11: LOCAL_DONE on maintained dev, annotation + adversarial script (no migration). [Evidence](../backend/evidence/community-safety/CS05B_REVIEW_ANNOTATION.md). `ReviewLookup` port + `TallyResult.UnderReview` (nil-safe, fail-closed 503); moderation `OpenReview`; frozen `under_review` + `review_notice` wording; wiring cmd/api + fixture; OpenAPI + vector; `scripts/helpfulness-adversarial.sh` (loopback-only local, https+confirm staging, guards exercised; staging run owed). Unit + e2e (RED pre-wiring → GREEN; dismiss clears) + full HTTP package + contract green. vet/gofmt/diff-check clean, secrets passed. Next microtask: CS06 projections/consensus.

- Community safety CS05A 2026-10-11: LOCAL_DONE on maintained dev, moderation target only (no routes). [Evidence](../backend/evidence/community-safety/CS05A_CONTRIBUTION_MODERATION.md). `CONTRIBUTION` target (P2, UUID) + frozen reasons (SPAM/HARASSMENT/VOTE_GAMING/OFF_TOPIC/OTHER, enforced for new target only) + REVIEW/INVALIDATE/RESOLVE/DISMISS eligibility + consensus job mapping + migration 000055 (000026 precedent) + ops CLI. Hide = REJECTED + recompute (no new writer); proven: invalidate → cast/tally refuse, audit 1/0 intact. Unit + one-open-case + full invalidate-flow integration green. vet/gofmt/sqlc/diff-check clean, secrets passed. Next microtask: CS05B annotation + adversarial pass.

- Community safety CS04 2026-10-11: LOCAL_DONE on maintained dev, read pins + measured plans only. [Evidence](../backend/evidence/community-safety/CS04_FEED_PLANS.md). New: EXPLAIN pins (cheapest index-only, tally pkey single search, rebuild target idx, zero seq scans) over 1500-row load; tiebreak ascending across all 30 pages; 4 orders serve; 8×4 ValidateFeed matrix; FeedItem no-leak. Measured: feed 4.1ms exec / 0.7ms planning; tally 1 index search. RLS absent by design (P08 owns roles); 35d is retention not feed expiry. vet/gofmt/diff-check clean, secrets passed. Next microtask: CS05 moderation queue.

- Community safety CS03B 2026-10-11: LOCAL_DONE on maintained dev, identity hardening only (no migration). [Evidence](../backend/evidence/community-safety/CS03B_LIMITER_ROTATION.md). Fixed 2 real gaps in `HashIPSubject` (IPv4-mapped fork via missing `Unmap()`; `%zone` accepted) caught RED by new pins. Proven: 50 farm subjects → exactly 50 live rows, expiry+sweep → 0 with budgets back; attribution token identical across full rotation (no vote fork; old key dead per existing proof). Full identity unit+integration green. vet/gofmt/diff-check clean, secrets passed. No global subject ceiling (NAT-availability rationale, residual recorded). Next microtask: CS04 feeds/query plans.

- Community safety CS03A 2026-10-11: LOCAL_DONE on maintained dev, quota only (no migration). [Evidence](../backend/evidence/community-safety/CS03A_HELPFULNESS_QUOTA.md). `HelpfulnessPorts.CheckQuota` (fingerprint/write, fail-closed nil) with 429 + Retry-After mapping in cmd/api and fixture. Proven: stable per-account token binds one vote across device keys; shared fingerprint keeps kiosk accounts separate. Fixed two latent tally races exposed by quota wiring (in-tx retry 25P02; stale-snapshot rebuild overwrite) via single-pass bump + advisory-lock serialization. Unit (denial/kiosk/stability) + HTTP 429 + real-Limiter exhaustion (3 ok, 4th denied, tally 3/0) green; CS02A 11/11 re-green incl. 2x race; full unit + HTTP integration green. vet/gofmt/diff-check clean, secrets passed. Next microtask: CS03B cardinality/IP/rotation.

- Community safety CS02B 2026-10-11: LOCAL_DONE on maintained dev, HTTP+OpenAPI only. [Evidence](../backend/evidence/community-safety/CS02B_HELPFULNESS_HTTP.md). 3 versioned routes (cast 201/remove 200/public tally + signed overlay), strict bodies (64KiB, unknown/dup/trailing/UTF-8 rejected), frozen error codes (self-vote 403, stale 409, invalid-vote 400), no voter/key/device/cursor leaks, no-store; real wiring in cmd/api. Binding fix: vote identity binds caller.Token like confirmations (e2e caught author self-vote 201 → now 403). Unit 12/12, full-stack e2e (signed proof, replay, negatives) green, CS02A 10/10 re-green incl. race, apicontract 4 vectors green. vet/gofmt/diff-check clean, secrets passed. Next microtask: CS03 admission/abuse.

- Community safety CS02A 2026-10-11: LOCAL_DONE on maintained dev, store only (no HTTP/OpenAPI). [Evidence](../backend/evidence/community-safety/CS02A_HELPFULNESS_STORE.md). Migration 000054 (votes uniques, tallies with non-negative checks, observation visibility columns) + sqlc queries + `HelpfulnessStore` pg implementation. Real-PostGIS: 10/10 green incl. 20-voter concurrency, flips, removals, crash-replay, per-contributor submission scope, corrupt-tally rebuild, ownership/eligibility, erase+recast, 000054 upgrade, non-destructive guard; concurrent trio also `-race` green. Key fix: delta-upsert rejected by PG CHECK-before-conflict semantics → plain UPDATE + exact rebuild on missing. vet/gofmt/diff-check clean, secrets scan passed; local staticcheck predates go1.27 (env gap, CI pinned). Next microtask: CS02B helpfulness HTTP + OpenAPI.

- Offline community extension 2026-10-11: PLANNING_LOCAL_DONE / IMPLEMENTATION_NOT_STARTED, opening maintained dev `61f7c1b`; existing CI contributor commit preserved. [CS15–CS25 additional phases/rules](COMMUNITY_SAFETY_EXECUTION_PLAN.md), [updated implementation and eight-line microtask prompts](COMMUNITY_SAFETY_EXECUTION_PROMPT.md), [inactive Portuguese offline notice](../product/COMMUNITY_PRIVACY_TERMS_PT_BR.md). Execution order CS00–CS12 → CS15–CS25 → CS13 → CS14 preserves final staging/device GPS acceptance. Reuse Room station cache/outbox; city feed lacks persistent cache-first reads, dispatch scans listAll, current capture needs online two-minute authorization. Plan covers bounded offline city packs/local pages, all-route capability matrix, queued social/price/privacy actions, versioned offline-capture decision, clock/expiry/owner rules, reconnect quotas/idempotency, abuse/resource/recovery tests and actual device/24h evidence. Full coverage means tested outcomes, never unseen fresh prices, instant remote acknowledgement or absolute abuse immunity. No runtime/dependency/VPS/device/GitHub/wiki operation; docs checks only. Next: selected pending microtask under explicit dependency order; all implementation/runtime gates remain pending.

- Landing redesign LR39 2026-10-11: LOCAL_DONE on branch `landing`, final SEO/AI report in [evidence](../release-evidence/landing-redesign.md) (verified vs owed per source, no ranking guarantee). Landing construction batch LR00–LR39 complete; integration/deploy/index owed per LR36–LR38.

- Landing redesign LR38 2026-10-11: LOCAL_DONE on branch `landing`, index follow-up.

- Landing redesign LR37 2026-10-11: RECORDED on branch `landing` — DEPLOY_PENDING, no remote change (no infra approvals requested or received). Owed: integration, protected delivery, then preview/publish/smoke/rollback approvals. Next: LR38 index follow-up.

- Landing redesign LR36 2026-10-11: DECISION on branch `landing` — INTEGRATION_PENDING, no merge.

- Landing redesign LR35 2026-10-11: LOCAL_DONE on branch `landing`, scoped union once at `3da311e`.

- Landing redesign LR34 2026-10-11: LOCAL_DONE on branch `landing`, pending-media sweep.

- Landing redesign LR33 2026-10-11: LOCAL_DONE on branch `landing`, final review.

- Landing redesign LR32 2026-10-11: LOCAL_DONE on branch `landing`, accessibility exit.

- Landing redesign LR31 2026-10-11: LOCAL_DONE on branch `landing`, store readiness.

- Landing redesign LR30 2026-10-11: LOCAL_DONE on branch `landing`, error/redirect pass.

- Landing redesign LR29 2026-10-11: LOCAL_DONE on branch `landing`, performance pass.

- Landing redesign LR28 2026-10-11: LOCAL_DONE on branch `landing`, crawler directives.

- Landing redesign LR27 2026-10-11: LOCAL_DONE on branch `landing`, indexation evidence.

- Landing redesign LR26 2026-10-11: LOCAL_DONE on branch `landing`, navigation hardening.

- Landing redesign LR25 2026-10-11: LOCAL_DONE on branch `landing`, legal copy review.

- Landing redesign LR24 2026-10-11: LOCAL_DONE on branch `landing`, search/recovery/CRUD reference.

- Landing redesign LR23 2026-10-11: LOCAL_DONE on branch `landing`, permission/location pass.

- Landing redesign LR22 2026-10-11: LOCAL_DONE on branch `landing`, offline/error states. Airplane-offline cache + empty-search captures (radios restored off, query cleared, list verified); TU29 page + TU08 error step; manifest 23/69. Build/check green, 35/35 tests. Next: LR23 permission/location pass.

- Landing redesign LR21 2026-10-11: LOCAL_DONE on branch `landing`, seven evidence-backed chapters (TU05/06/07/08/17/23/28 from accepted masters + fresh read-only bound dumps, no new captures). 14 chapter pages live; TU01/03/04/11/12/15/16/18/19/22/24/25/27/29/30 pending (no captures or blocked). Build/check green, 35/35 tests. Next: LR22 empty/error/offline states.

- Landing redesign LR20 2026-10-11: LOCAL_DONE on branch `landing`, vehicle-form lesson TU26. Blank dark form (nothing typed/toggled/saved) + page; suggest/claim/manage entries unobserved, alert OS permission pending. Build/check green, 35/35 tests. Device left on Explorar. Next: LR21 remaining lessons.

- Landing redesign LR19 2026-10-11: LOCAL_DONE on branch `landing`, report-rules lesson TU21. Rules dialog capture (read-only, dismissed) + page; rating/comment/vote composers gated by station verification + empty feed, pending hermetic preproduction. Build/check green, 35/35 tests. Next: LR20 management lessons.

- Landing redesign LR18 2026-10-11: LOCAL_DONE on branch `landing`, photo entry lesson TU14. Capture-entry frame (degraded list, disabled shutter, dev toggle off) + page; shutter never pressed, OCR/outbox pending on synthetic fixtures. Build/check green, 35/35 tests. Next: LR19 participation lessons.

- Landing redesign LR17 2026-10-11: LOCAL_DONE on branch `landing`, account lesson TU13. Profile-connected capture (demo `zanatateste`, no key in pixels) + page; auth/key visuals secure-blocked (hierarchy-verified, never invented); TU11/TU12 visuals pending. Build/check green, 35/35 tests. Next: LR18 photo lessons.

- Landing redesign LR16 2026-10-11: LOCAL_DONE on branch `landing`, driver lessons BUC01/BUC02. Chapter pages TU02/TU09/TU10 (10 steps, native-viewBox overlays, prev/next, hub links flipped, sitemap); full text without JS, no app-write semantics. Build/check green, 35/35 tests. Conta `zanatateste` segue logada para LR17+. Next: LR17 account lessons.

- Landing redesign LR15 2026-10-11: LOCAL_DONE on branch `landing`, tutorial hub. `/como-usar/` with 30 chapters, 4 journeys, status labels, zero dead links; home nav + sitemap + build/check wired. Build/check green, 34/34 tests. Chapter links flip on as pages land (LR16+). Next: LR16 driver lessons.

- Landing redesign LR14 2026-10-11: LOCAL_DONE on branch `landing`, three annotated lessons. `lesson.json` for TU02/TU09/TU10 (10 steps, real bounds from dumps) + municipios-mt master/derivatives (manifest 17, app 51); validator covers shipped lessons. Build/check green, 33/33 tests. Next: LR15 tutorial hub.

- Landing redesign LR13 2026-10-11: LOCAL_DONE on branch `landing`, tutorial data contract. `lesson.json` schema + overlay/prev-next rules in [catalog](../product/LANDING_MOBILE_TUTORIAL_CATALOG.md); pure `validateLesson` (`src/tutorial.ts`, test-only, never shipped) + shared chapter styles; 3 validator tests. Typecheck green, 33/33 tests. Next: LR14 three annotated lessons.

- Landing redesign LR12 2026-10-11: LOCAL_DONE on branch `landing`, source/project/footer pass. Explicit ANP non-affiliation + source separation + no-promise copy; legal/GitHub/MIT destinations preserved and test-guarded. Build/check green, 30/30 tests. Conta `zanatateste` logada no aparelho registrada para LR16+; aparelho intocado neste lote. Next: LR13 tutorial data contract.

- Landing redesign LR11 2026-10-11: LOCAL_DONE on branch `landing`, app gallery + stories. 3 authentic figures (srcset 360/720/1080, lazy, alt, captions with status) + static layout; no carousel/autoplay; emitted-asset test. Build/check green, 30/30 tests. Next: LR12 source/privacy/project/footer.

- Landing redesign LR10 2026-10-11: LOCAL_DONE on branch `landing`, header/hero revision. Candidate H1 + `Conheça o aplicativo`→#aplicativo primary CTA (contract-amended, checks updated), 1152px width, 32–56px H1, solid AA CTA; hero phone-free (test-guarded). Typecheck/build/check green, 29/29 tests. Tutorial anchor wiring owed at LR15 (no orphan links). Next: LR11 gallery/stories.

- Landing redesign LR09 2026-10-11: LOCAL_DONE on branch `landing`, asset build extension. Provenance-gated `assets/app/` copying (48 WebP hash-verified, masters refused, missing/traversal loud) + 4 build tests; typecheck/build/check green, landing tests 29/29. Chapter route registration owed at LR15 with chapter sources. Next: LR10 header/hero.

- Landing redesign LR08 2026-10-11: LOCAL_DONE on branch `landing`, responsive derivatives. 48 deterministic WebP (360/720/1080w, q85, aspect-preserved, no upscale) + provenance in `landing/static/assets/app/`; 360w legible at 100%, rebuild identical, landing tests 25/25 green. Build wiring/alt assignment owed at LR09/LR11. Next: LR09 asset build.

- Landing redesign LR07 2026-10-11: LOCAL_DONE on branch `landing`, manifest selection. 16 entries with roles + claim classes, rejected list, 13 missing rows in manifest; [README selection](../assets/landing/redesign-20261011/README.md). Integrity re-verified 16/16, diff-check clean, secrets passed. Next: LR08 responsive derivatives.

- Landing redesign LR06 2026-10-11: LOCAL_DONE on branch `landing`, conditional states view-only. 2 clean captures (contributions-empty, settings) in manifest (16 total); auth/key screens uncapturable by FLAG_SECURE (recorded, TU11/TU12 visuals pending); fixture-gated states (photo/OCR/outbox/feedback/claim) pending, never staged on user phone. Hash/dim checks, diff-check clean, secrets passed. Next: LR07 manifest selection.

- Landing redesign LR05 2026-10-11: LOCAL_DONE on branch `landing`, public-read marketing set. 6 clean 1220×2712 captures (stations-list, station-detail, station-sources, community-feed, history, week-picker) in media [README](../assets/landing/redesign-20261011/README.md)/manifest (14 total); read-only authorized phone session, Sorriso kept, no mutations; notification + secure-screen frames rejected. 100% visual review + hash/dim checks, diff-check clean, secrets passed. Next: LR06 conditional states.

- Landing redesign LR04 2026-10-11: LOCAL_DONE on branch `landing`, safe capture environment. Standard (pt-BR, 1.0 font, portrait, native res) + seeder ban (`clearAppStorage:262`) + device rule in media [README](../assets/landing/redesign-20261011/README.md)/manifest; no new capture, 8/8 hashes re-verified. Diff-check clean, secrets passed. Next: LR05 marketing capture set (needs device session).

- Landing redesign LR03 2026-10-11: LOCAL_DONE on branch `landing`, visual contract revision. Reference-v2 composition frozen; token supersessions (1152px width, Manrope/Source Sans, teal/pale palette, frame radii + one device shadow) + measured AA contrast + logo provenance in [contract](../product/LANDING_CONTENT_CONTRACT.md). Wireframes well-formed/script-free, diff-check clean, secrets passed. Next: LR04 capture environment.

- Landing redesign LR02 2026-10-11: LOCAL_DONE on branch `landing`, IA/copy freeze. Home anchor order + `/como-usar/` hub + 30 frozen chapter slugs in [contract](../product/LANDING_CONTENT_CONTRACT.md)/[catalog](../product/LANDING_MOBILE_TUTORIAL_CATALOG.md); `/guias/` + legal retained; availability labels fixed. Slug check 30/30 unique, diff-check clean, secrets passed. Next: LR03 visual contract.

- Landing redesign LR01 2026-10-11: LOCAL_DONE on branch `landing`, capability/claim freeze. [Catalog](../product/LANDING_MOBILE_TUTORIAL_CATALOG.md) maps all 25 Routes.kt constants to TU01–TU30; TU20/CS-offline/training-consent/provider-email explicitly gated; [content contract](../product/LANDING_CONTENT_CONTRACT.md) reconciled to username/key primary flow. Route-to-chapter check 25/25, diff-check clean, secrets passed. Next: LR02 IA/copy freeze.

- Landing redesign LR00 2026-10-11: LOCAL_DONE on branch `landing` (evidence `20bb603`, ledger refresh `8d10c22` local; `origin/landing` `20bb603`, `dev`/`origin/dev` `a853708`, `main`/`origin/main` `a349d7c`). [Evidence](../release-evidence/landing-redesign.md). Worktree `.worktrees/landing-refresh` clean; occupied dev checkout untouched. P25/P26 acceptance and preparation artifacts (local fonts, authentic reference/media, 30-chapter catalog) inventoried and re-verified (8/8 capture hashes+dims, 4/4 font hashes+signatures, TU01–TU30 present); no duplicate foundation work. Git-only validation + secret scan, no runtime suite per plan. Next: LR01 capability/claim inventory.

- Landing preparation 2026-10-11: PLANNING_LOCAL_DONE; FONT_INTEGRATION_LOCAL_DONE / INTEGRATION_PENDING; production redesign and complete tutorials NOT_STARTED. Existing **landing** branch, isolated `.worktrees/landing-refresh`, opening `954a8d8`; occupied dev edits untouched. [40 microtasks LR00–LR39](LANDING_REDESIGN_EXECUTION_PLAN.md), [economical-agent prompt](LANDING_REDESIGN_EXECUTION_PROMPT.md), [30-chapter mobile catalog](../product/LANDING_MOBILE_TUTORIAL_CATALOG.md) and [authentic reference/media](../assets/landing/redesign-20261011/README.md). Separate opening/gallery/tutorial sections; two-step red-rectangle preview uses real app pixels. Eight native 1220×2712 captures, guest/blank states, debug 3.1.0/code4, installed SHA unknown; private overlay attempts rejected. Original dark theme restored. Requested local Manrope/Source Sans 3 WOFF2 + OFL/provenance/hash-copy regression integrated; typecheck/build/check and 25 landing tests passed, gate selection 18/18, browser responsive/font checks passed. Zaflas read-only SEO patterns and current official search/AI crawler research recorded; no ranking guarantee. No account/data mutation, backend behavior, deployment, remote tracker/PR/merge/wiki or index submission. Next: LR00 reconcile prepared artifacts on landing, then the next authorized bounded phase; normal reviewed integration into dev and protected dev→main only at authorized delivery.

- Community safety CS01 2026-10-11: LOCAL_DONE on maintained dev at `61f7c1b`, pure domain+contract only. [Evidence](../backend/evidence/community-safety/CS01_HELPFULNESS_CONTRACT.md). Frozen: votes bind observation UUID + decision sequence (stale refuses, never transfers); HELPFUL/NOT_HELPFUL only (VALID refuses); floor-bp share null on zero; self/inactive/hidden/deleted/stale refuse pre-mutation; change rewrites, retry replays, remove idempotent; counts public, MyChoice private; erasure via ContributorRef (store in CS02). New: domain/helpfulness.go, application/helpfulness.go, 4 golden fixtures + pinning test; no migration/HTTP/route/VPS. `go test ./internal/modules/community/...` green incl. 16 new tests (RED seen pre-GREEN), vet/gofmt/diff-check clean, secrets scan passed. Next microtask: CS02 atomic votes/HTTP/projections.

- Community safety CS00 2026-10-11: LOCAL_DONE on maintained dev at `61f7c1b`, audit only. [Evidence](../backend/evidence/community-safety/CS00_AUDIT.md). Verified reuse: community confirm/dispute (000013 uniques, 48h window), feedback VALID/INVALID pattern (000025 one-live-vote, floor pct), moderation one-open-case (000016, P1-P3, 200/500 chars), identity quotas v1 (10/200/60 per hour, keyed IP digests), feed GET /v1/community/feed (route cap 50 vs shared 100, 4 orders, sealed 15min cursor), 24h media/3 MiB reserve/retention TTLs, 100m/100m/s location gates. Missing: HELPFUL tables/routes, contribution+revision target (observations lack revision counter), CONTRIBUTION moderation target, consent/TEST_ONLY inventory. dev contains main (15 ahead, 0 behind). `git diff --check` clean, no secrets/PII, no runtime/VPS/CI change. Next microtask: CS01 helpfulness domain+contract.

- Android CI fix 2026-10-11: LOCAL_DONE on maintained dev for protected PR127, coroutine-isolation correction only. `CommunityPricesViewModelTest` now shares one `StandardTestDispatcher` (`runTest(dispatcher)`, repo idiom from CommunityFeed/Vehicle/CaptureOcr tests) so `viewModelScope` (Main) and `advanceUntilIdle()` run on the same scheduler; bare `runTest` left Main coroutines on an unsynchronized scheduler, the textbook `UncaughtExceptionsBeforeTest` flake seen in run38140418786. Forced no-cache reruns green: debug 249/249, release 241/241, community package 44/44. No production-code, dependency or workflow change. Next: push dev so PR127 re-runs remote checks; merge only with green required checks.

- Community safety planning 2026-10-11: PLANNING_LOCAL_DONE / IMPLEMENTATION_NOT_STARTED on maintained dev, opening `0f3601c`. [CS00–CS14 phased plan](COMMUNITY_SAFETY_EXECUTION_PLAN.md), [implementation-AI prompt](COMMUNITY_SAFETY_EXECUTION_PROMPT.md) and [Portuguese terms/Settings copy draft](../product/COMMUNITY_PRIVACY_TERMS_PT_BR.md). User clarified votes mean contribution helpful/not helpful, independently of price correctness/confidence. Reuse existing votes/reports/identity/feeds/privacy, add bounded admission/storage/query tests and actual all-copy 24h expiry/deletion/restore evidence. Last phase: Abastevo-only staging and connected Android Fake GPS refusal/isolated TEST_ONLY geometry plus genuine-location acceptance. Terms remain NOT_ACTIVE; collection stays OFF. No runtime code, dependency, VPS/device or GitHub/wiki change. Documentation references/consistency, secret surface and diff validation only; no runtime acceptance. Next implementation task: CS00, then bounded sequential phases; PR127 Android failure below still blocks delivery.

- CI recovery 2026-10-11: backend corrections LOCAL_DONE / INTEGRATION_PENDING on maintained dev for protected PR127; final delivery BLOCKED by Android CI. [Failure causes, correction and validation](../backend/CI_PR127_RECOVERY.md). Lowercase worker errors restore staticcheck; backend CI and the full local runner now build/provide the actual locked Rust preparer, without skipping its pipeline integration test. Pinned Go and dependency-cache paths repaired. Targeted worker race/static checks, workflow/shell validation and complete disposable-PostGIS suite passed (80 tested packages, race detector, actual Rust pipeline). At published head `0f3601c`, backend fast/integration and Quick verification passed; Android `test` failed with `UncaughtExceptionsBeforeTest` in `CommunityPricesViewModelTest` (run38140418786). Local JDK17 debug249/release241 tests passed without establishing the remote failure's cause or a fix. No VPS changes. Next delivery task: diagnose/fix actual coroutine isolation failure, validate changed head, then guarded merge only with successful checks and publish merged-SHA wiki; no lucky-rerun acceptance. Wiki preview also has an independent `IMAGE_OUTSIDE_ASSETS` blocker in RST_VPS_HARDENING.md; no wiki publication occurred.

- Daily registry refresh 2026-10-10: LOCAL_DONE / INTEGRATION_PENDING; bounded staging DEPLOYED, behavior13c7c25. [Contract/tests/runbook and sanitized evidence](../backend/REGISTRY_DAILY_SYNC.md). Daily03:00 America/Cuiaba now active,250m/256Mi max, no resident refresh process. Live ANP45617 publication120.8s/189.0MiB cgroup peak; unchanged verification2.9s/20.7MiB,10 curated differences preserved. Real PostGIS recovery/concurrency/source-change tests passed; final photo-retry correction passed15 DB cases. Two actual HTTPS5req/s windows each1500 correct, zero errors/drops, worst route p95 mixed274.7ms/final API473.8ms. Same pod/media volume; API/worker each one intentional restart, storage/DB0, no OOM/unexpected restart; only Abastevo affected. All45617 real prepared station IDs have profiles; one pre-existing non-ANP identity has no profile;24552 synthetic assertions remain distinct. Global MinIO hash varied and is not photo-integrity acceptance. No daily scheduled run/24h soak/max-user/G09 certification inferred. Next: protected dev→main PR127 delivery and merged-SHA wiki when checks allow.

- RST hardening 2026-10-10: corrections LOCAL_DONE / INTEGRATION_PENDING on maintained dev; runtime PARTIAL. [Scope, tests, measurements and reproduction](../backend/RST_VPS_HARDENING.md). PG18/checkpoint and concurrent-throughput fixes; bounded file emission/atomic streaming load, unchanged membership, transient recovery and paged profile publication passed immediate real VPS failure/concurrency/migration/least-privilege tests. Actual ANP import published 45,617 real stations/profiles; 121 municipality variants quarantined; 24,552 synthetic assertions retained separately. Five VPS Rust preparations median4.609s / max113.1MiB RSS under250m CPU. Actual HTTPS pooled5req/s completed one300s window (1,500 correct, p95<=292.2ms); second repetition stopped by host-load reserve5.9409>5.2, so no replicated capacity/max-users acceptance. Only Abastevo modified; existing limits preserved, no observed OOM/unexpected restart. Schema53 temporarily broke old API readiness; compatible API/worker2cd3fb6 override restored200, incident recorded. Selected private raw archive preserved/hash-verified; reproducible aggregate report committed. RST20 24/48h and full RST21 remain NOT_ACCEPTED; UF partition exploratory. No production certification. Next: protected dev→main delivery; then long-harness correction and safe reserved-window measurements without touching other apps.


- Rust RST-20 soak pilot 2026-10-10: HISTORICAL_PILOT / SOAK_OWED, isolated PostGIS only; cost slopes require corrected collectors. [Soak evidence](RUST_STATION_INGESTION_RST20.md). 30-min pilot green (9 imports, 6 retained, worst window p95 71 ms, 0 errors, backlog flat); slopes WAL +9.28/table +3.75/dead +944 per min; row-DELETE retention flagged expensive (1,086 s, owed task); checkpoint counters unverified (gap). 24/48h OWED. RST-21 environment prepped (bundle index + collection checker green). Next: RST-21 decision/report (separately authorized).
- Ops note 2026-10-10: station-load ingest CLI added (`backend/cmd/station-load`, stage-only, no migrations); PR #126 merged dev→main via guarded helper (checks green, no bypass); staging redeployed from merged main (migration 000049–000052 applied, api+worker rebuilt @a349d7c, 3/3 Running, health 200) and the 20k batch is LOADED (registry 19552/430/18 + pmqc 5000/0/0 complete, 24,552 live assertions, exact oracle; API ready throughout). Path notes: tunnel numbers are path-limited (loader ran on-VPS in 69s vs 10min-timeout over tunnel); one partial tunnel run surgically cleaned (10,213 rows + 1 run, zero residue); gofmt applied once to a stale ignored scratch file blocking the local gate (disclosed). Incidental: local dev DB received its 4 pending forward migrations, consistent, no loss.
- Rust RST-19 recovery 2026-10-09: TESTED, isolated PostGIS only. [Recovery evidence](RUST_STATION_INGESTION_RST19.md). Kill/disconnect/duplicate/malformed/checksum/stale faults all fail loudly with invariants held (no loss, no dupes, no false-complete); kill exposed a silent-retry trap, fixed with a 5-line fail-loud reload (no prod callers, replay untouched); reads never error during kill with immediate drain. Disk-full unsupported (shared disk), 48h virtual-clock not executed (owed). Next: RST-20 soak (separately authorized).
- Rust RST-18 capacity 2026-10-09: HISTORICAL_DIAGNOSTIC, Reader-direct isolated DBs only; importer/checkpoint metrics invalid, HTTP capacity unproven by this run. [Capacity evidence](RUST_STATION_INGESTION_RST18.md). Open-arrival reads × 1/4 closed-loop disjoint-edition importers on identical cells: local envelope sustainable at 100 rps + 4 importers, knee bracketed in (100, 200] rps (drops + p95 breach, imports still error-free); VPS staging envelope 20 rps reads-alone with knee in (20, 50] explained by the 1-CPU DB pod cap (tunnel numbers discarded as path-limited; zero leftovers, namespace untouched). Next: RST-19 failure/recovery (separately authorized).
- Rust RST-17 partitions 2026-10-09: EXPLORATORY, lab DDL only; the 2026-10-10 review supersedes adoption/direction language. [Partition evidence](RUST_STATION_INGESTION_RST17.md). A vs UF-LIST B vs hash-16 C on leveled 100k field (3 trials): B totals −14/−18/−23% with repeatable scan wins and no repeatable regression (hypothesis only, adoption not accepted; A stays deployed); C sign-flips including +33% (rejected); BRIN on history +73.8% at 7 ms/24 KB (accepted as direction). Next: RST-18 concurrent load (separately authorized).
- Rust RST-16 spatial reads 2026-10-09: MEASURED, isolated PostGIS only. [Spatial evidence](RUST_STATION_INGESTION_RST16.md). Vincenty oracle agrees exactly at 150m/1km/5km/15km over dense/sparse/empty/cross-UF sites with GiST diagnostics (wide radii sort-bound; border 15km p95 ~167 ms breaches provisional 150 ms, owed to RST-18); authoritative 150m gate allows/refuses 149.9/150/150.1m on real PostGIS with fix-integrity negatives refusing pre-DB (+0.17 ms gate overhead). Next: RST-17 partitions (separately authorized).
- Rust RST-15 city reads 2026-10-09: MEASURED, isolated PostGIS only. [Traffic evidence](RUST_STATION_INGESTION_RST15.md). Nine workloads at raw-SQL vs Reader layers, stable 5/20/50 keyset pagination with tie coverage, closed 1/8/32 + open 20 rps, pinned cache states, plan diagnostics, trigram GIN trial (+85% selective text, accept-pending-migration); 1M spot closed-8 dense p95 201 ms breaches the provisional 100 ms hypothesis (owed revision, RST-18). Next: RST-16 spatial reads (separately authorized).
- Rust RST-14 construction 2026-10-09: MEASURED, isolated PostGIS only. [Construction evidence](RUST_STATION_INGESTION_RST14.md). Real RST-13→loader chain exposed and fixed two contracts (manifest timestamps; duplicate multiset emission for loader-side dedup); per-stage timing + WAL/storage metrics on fresh DBs (tiny ~0.7 ms/1,422 WAL-B per row, 20k ~0.5 ms/852 B, ~1 commit/row); replay/concurrent converge, stale editions fail visibly; publication NOT_AVAILABLE, COPY/delta loaders owed as separate tasks. Next: RST-15 city reads (separately authorized).
- Rust RST-13 preparation 2026-10-09: MEASURED, offline only. [Preparation evidence](RUST_STATION_INGESTION_RST13.md). New `bench_stages` per-stage timing plus additive spill runs/bytes accounting; release matrix 20k/100k/1M (parse 372k→262k rows/s, emit ~450k in-memory/~250k spill-forced, RSS ~3.6 KiB/row); reorder/spill reproduce hashes exactly, deltas publish, accounting reconciles every case. Historical emission buffered all outputs; bounded file emission now tested under hardening (parsed batch remains resident). Next: RST-14 DB construction (separately authorized).
- Rust RST-12 harness 2026-10-09: IMPLEMENTED, tooling only. [Harness evidence](RUST_STATION_INGESTION_RST12.md). Extended `stats.py` (sample schema v1, reconcile, histogram merge, overhead aid, budget/refuse exits) plus new stdlib `drive.py` (closed 1/8/32 + open-rate with counted drops, timeout/wrong/cancel paths); 9/9 loopback checks pass, load smoke 46,546 reqs within budgets. One pre-existing `faults.sh` readiness-after-recovery failure reproduced on clean tree (out of scope, fails closed). Next: RST-13 preparation scaling (separately authorized).
- Rust RST-11 datasets 2026-10-09: IMPLEMENTED, offline synthetic only. [Datasets evidence](RUST_STATION_INGESTION_RST11.md). Deterministic generator/oracle/request traces (`station-datasets-v1`, 200+5 cities, seeds pinned); committed tiny set 30/24/2/4 with frozen hashes; stress-100k reproduces RST-07 98004/1996/0 accounting; empty strata listed, never invented. Next: RST-12 harness (separately authorized).
- Rust RST-10 protocol 2026-10-09: PROTOCOL_FROZEN, no code/DB/network. [Protocol freeze](RUST_STATION_INGESTION_RST10.md). Versioned run-manifest schema, metric dictionary, workload IDs, ceilings/stop conditions and worksheet with provisional budgets as hypotheses; RST-07 pilot as baseline, gaps (8-client evidence, pg_stat_statements) blocking SLO language. Next: RST-11 datasets (separately authorized).
- Rust RST-09 handoff 2026-10-09: IMPLEMENTED, read projection only. [Handoff evidence](RUST_STATION_INGESTION_RST09.md). Directory reads carry source-separated official anchors with run freshness (nil for community-only, no trust mixing); unknown locations stay honest nulls excluded from Nearby; code-exact municipality pagination pinned with rollback runbook (additive only, no migration, canonical data survives removal). Contract spec updated without new warnings. Next: RST-10 protocol (separately authorized) or staged live-source verification.
- Rust RST-08 discovery ops 2026-10-09: IMPLEMENTED, no live fetch. [Operations evidence](RUST_STATION_INGESTION_RST08.md). New `registry-discover` job wraps bounded discovery with snapshot idempotency, terminal quarantine completion and Fail/backoff/dead-letter on provider faults; worker wiring plus disabled daily schedule mirror the reconcile pattern. Unit and real-PostGIS dispatched tests cover bounded retries (exactly 3 hits), crash recovery via lease expiry, outage dead-lettering and no-refetch replay. Next: RST-09 city/profile handoff.
- CI source-enum repair 2026-10-09: FIXED. Migration 000052 restores `dou-editions`/`dou-acts` dropped by 000049/000050 (evolved from the creating migration instead of 000033); enum regression test pins all six sources; dou integration green again.
- Rust benchmark planning extension 2026-10-09: PLANNING_LOCAL_DONE / INTEGRATION_PENDING on isolated `codex/ingestion-bench-plan`, parent `e257ee8`; [RST-10–21 performance campaign](RUST_STATION_BENCHMARK_PLAN.md), linked from the [ingestion plan](RUST_STATION_INGESTION_PLAN.md) and [bounded prompt](RUST_STATION_INGESTION_PROMPT.md). Twelve additional phases define reproducible protocol/corpus/harness, preparation and DB construction, geographic city/spatial reads, index/partition comparison, mixed-load capacity, recovery/soak/cost and regression decisions. Existing RST-00–09/P29 IDs and concurrent RST-06 work preserved. Metrics include explicit denominators, error/drop/censored samples, per-city strata, repeated trials, raw artifacts and provisional budgets; no benchmark execution, measured speedup, implementation, DB change or remote publication. Docs links/phase coverage/whitespace/secret surface reviewed. Next benchmark task: explicitly select RST-10 protocol only; implementation tasks keep their separate current evidence.

- Rust RST-07 bench 2026-10-09: MEASURED, no partitioning. [Benchmark evidence](RUST_STATION_INGESTION_RST07.md). Shared-seed 100k parses identically both sides (98004/1996/0, multiset hash equal, golden recipe pinned); Rust 322k rows/s at 214 MiB RSS, Go 124–152k rows/s at 8.2 MiB heap, 1M needs 2.19 GiB (streaming follow-up owed). Reads at 100k/1M: candidate covering index cuts city-dense p95 83.7→49.2 ms with no regression (adopted, migration 000051); partitions rejected on data. Revised lab targets recorded. Next: RST-08 incremental operations.
- Rust RST-06 review gate 2026-10-09: IMPLEMENTED, staging only. [Review evidence](RUST_STATION_INGESTION_RST06.md). Migration 000050 adds the `review` run source; new `ReviewCandidate` promotes corroborated candidates (same CNPJ, complete runs, unknown quality, 150 m transformed agreement, 90-day window) to reviewed assertions on deterministic snapshots, with rejection staging nothing; CRS math stays in PostGIS/PROJ (measured 4674 shift 0.0000 m, Go-vs-PostGIS scale 0.15%); end-to-end projects reviewed with 0.000 m gap. Directory/community suites green. Next: RST-07 benchmark/index experiment.
- Rust RST-05 loader 2026-10-09: IMPLEMENTED, staging only. [Loader evidence](RUST_STATION_INGESTION_RST05.md). Migration 000049 extends the run source enum with `station-prep`; new `LoadBatch` validates manifests/streams before any run exists and stages per-input runs through existing ownership with idempotent replay, live count agreement, succession links and hard unknown/pending gates; real-PostGIS race tests cover empty/upgrade/concurrent/tampered/ID-retention/no-partial/least-privilege role. Publication and official-lookup scope unchanged. Next: RST-06 location review gate on real PostGIS.
- Rust RST-04 join 2026-10-09: IMPLEMENTED, no network/DB. [Join evidence](RUST_STATION_INGESTION_RST04.md). New `join` module anchors pointed PMQC candidates at exact registry CNPJs with current/predates/stale/conflicting/missing triage, haversine conflict radius, 3-year staleness and coarse-precision flags; fixture evaluates 6 = 1+1+1+2+1 with identical reordered JSONL and no location promotion. 7 unit + 9 parser + 7 join + 9 deltas tests pass; fmt/clippy/test-locked clean, cargo-audit without findings. Next: RST-05 owned Go loader with real-PostGIS tests.
- CI quick gate 2026-10-09: FIXED/MONITORED. `tools/*` classified for full selection with new bounded `scripts/check-rust-fast.sh` (fmt, clippy, locked tests), `make check-rust`, pinned Rust 1.99.0 toolchain in `quick.yml` and gate-harness assertions (18/18 local). Residual local-only `gofmt` stop at ignored foreign scratch `backend/.local/` left untouched; clean CI checkout unaffected. PR #125 OPEN awaiting required check.
- Rust RST-03 deltas 2026-10-09: IMPLEMENTED, no network/DB. [Delta evidence](RUST_STATION_INGESTION_RST03.md). `station-prep 0.2.0` emits deterministic JSONL streams plus hashed batch manifest with atomic temp-rename publication, length-prefixed spill sort with run cleanup, golden canonical checksum and Noop/Publish/Stale replay decisions; 4 unit + 9 parser + 9 deltas tests pass, fmt/clippy/test-locked clean, cargo-audit without findings. Next: RST-04 coordinate-candidate hardening, still no reviewed flag.
- Rust RST-02 parser 2026-10-09: IMPLEMENTED, no network/DB. [Parser evidence](RUST_STATION_INGESTION_RST02.md). New `tools/station-prep/` library parses local 13-column registry and PMQC extracts to typed rows with bounded memory, kernel-compatible CNPJ checks, alias-table municipality resolution, SHA256 dedup, succession links and stable output order; fixtures evaluate 12 = 7+1+4 and 11 = 5+1+5 with byte-identical reordered JSONL. Pinned csv/serde/serde_json/sha2 with tracked lockfile, MIT-compatible licences, rust-version 1.99, zero cargo-audit findings; fmt/clippy/test-locked clean. Next: RST-03 determinism/deltas with spill strategy.
- Rust RST-01 contract 2026-10-09: CONTRACT_FROZEN, no implementation. [Source/batch contract](RUST_STATION_INGESTION_RST01.md). Froze membership-as-evidence mapping, UTF-8/BOM encoding, name-based header quarantine, IBGE alias rules, EPSG:4674 pending candidates and manifest-gated publication; new shared fixtures in `contracts/testdata/station-prep/` (13-column registry, header variant, PMQC assays, IBGE aliases, JSONL outputs, hashed batch manifest). No Cargo/dependency/DB/fetch change. Next: RST-02 pure Rust streaming parser over local files.
- Rust RST-00 audit 2026-10-09: AUDIT_DONE, no implementation. [Audit and minimal contract](RUST_STATION_INGESTION_RST00.md). Verified Go Directory/staging ownership, 13-column ANP header gaps (no COD_IBGE/SITUACAO/coordinates), PMQC EPSG:4674 candidates with unknown accuracy not usable in the 150m rule, existing complete-run-gated (non-atomic-snapshot) reconciliation, and indexed-baseline-first city/nearby reads. No Cargo/dependency/DB/fetch change. Next: RST-01 synthetic source/batch contract with fixtures and acceptance.
- Rust preparation planning 2026-10-09: [national source/location/performance plan](RUST_STATION_INGESTION_PLAN.md) and [economical-agent prompt](RUST_STATION_INGESTION_PROMPT.md) PLANNING_LOCAL_DONE; implementation PLANNED. Verified live 13-column ANP registry header has no coordinates/COD_IBGE/SITUACAO; PMQC metadata offers optional EPSG:4674 points without metre-accuracy evidence. Ten sequential tasks reuse existing Go Directory/Profile/staging, start with offline Rust preparation, and benchmark indexed baseline against optional read-projection partitioning. No crate, dependency, database partition, national fetch or coordinate promotion. Local links/whitespace/secret-surface reviewed. Next: RST-00 audit after app-first checkpoints and explicit implementation selection.

- Premium gasoline extension 2026-10-09: P37-T01/T02 LOCAL_DONE / INTEGRATION_PENDING on isolated `codex/premium-fuel`, parent `e6772c5`; [contract/evidence](../mobile/premium-fuel-category.md). Distinct premium grade preserves persisted additive data, manual selection/filters and exact signed submission. Explicit gasoline premium/brands recognized; bare brands stay manual. Ruby PNG generated then traced to six-path SVG, same 128px gasoline export; [repeatable icon workflow](../assets/fuel-icons/README.md). Full affected unit suites, zero-error lint/APKs, race/real-PostGIS, OpenAPI/static/secret/diff checks and four POCO OCR/Room cases passed. Combined app (249 unit cases) installed without data wipe; three native icon cases also passed. Backend deployment/live premium receipts remain pending. Independent PC06 remains owed. Rust plan delivered; maintained dev reconciled at behavior head `333c5f2`, preserving login. Next runtime step: separately scoped test-service backend publication before live premium sends.

- Owner-scope repair 2026-10-09: P37-T02 LOCAL_DONE / INTEGRATION_PENDING; [contract/evidence](../mobile/p37-owner-scope-expiry.md). Retained account identity survives access expiry without using an expired bearer; logout/key/account/origin changes remain blocked. 112 focused cases, final transport regression, zero-error lint/build/R8 and two native feedback cases passed. Exact-capture recovery found no eligible commands and no receipts; mobile delivery remains OWED. Temporary operator harness removed. Next: user-authorized distinct premium fuel category/icon, then Rust importer planning only.

- User-scoped OCR expansion 2026-10-08: P37-T01 LOCAL_DONE / INTEGRATION_PENDING on maintained dev parent `82aaa22`. [Behavior, exact-value metrics and limits](../mobile/p37-ocr-expanded-validation.md). Twenty hash-checked real photos improved 51→60 exact pairs out of 76, with incorrect associated rows 3→0; sixteen misses and seven incorrect unassigned manual numbers remain explicit. Bounded three-view crop/grayscale reconciliation preserves uncertain/conflicting values for human review. 1,313 executed unit cases (one unrelated opt-in live catalog skip), 32 spatial/view cases, five native POCO cases with 18 format/layout combinations and eight EXIF modes, five scorer cases, zero-error app/data lint/build/static/secret/diff checks passed. No dependency/backend/publication change. Independent PC06 acceptance remains owed; no universal/statistical accuracy claim. Next: finish the confirmed expired-access owner-scope repair and install the combined app without data wipe.

- User-scoped send repair 2026-10-08: P37-T02 LOCAL_DONE / INTEGRATION_PENDING on maintained dev parent `1896cf2`. [Behavior and recovery evidence](../mobile/p37-send-feedback-recovery.md). Visible queued/received/retrying/partial dialogs now offer immediate return to Community; server receipt is separate from validation. 246 app unit and two native POCO dialog cases, debug lint/build/static/secret/diff checks passed; corrected APK installed without data wipe. Existing backend replay/convergence real-PostGIS race tests passed. Explicitly authorized same-photo recovery restored hash-verified 231,563 bytes and replayed one audited dead job: session READY and job DONE; other two old jobs untouched. Mobile five-command retry backoff still pending at checkpoint, so no sent claim. No new submission, backend deployment or remote Git publication. Next: verify existing mobile receipts while completing the requested broader OCR validation.

- User-scoped OCR repair 2026-10-08: P37-T01 split board numbers LOCAL_DONE / INTEGRATION_PENDING on clean maintained dev parent `5bfd214`; actual origin/main already an ancestor. [Behavior and evidence](../mobile/p37-split-price-ocr.md). Connected POCO reproduced only ethanol because the other prices were separate integer/fraction OCR blocks. Bounded same-row recovery now recognizes all four exact values on the original review photo (354ms); the other eleven private corpus images retain their prior suggestions. 17 spatial regressions, 1,296 executed domain/application/data/app cases, two native pixel/corpus/corrupt tests, debug builds, zero-error lint and static/secret/diff checks passed; one unrelated opt-in live catalog case remains skipped. Corrected APK installed successfully with ADB, without account/data wipe. No backend/dependency/deployment or GitHub publication. Next: owner re-read in the corrected APK; human confirmation and prior PC05/PC06/release obligations remain unchanged.

- User-scoped developer validation 2026-10-08: temporary Android/server photo preview, one native key unlock per app session and shared account/login completion UI. [Behavior and evidence](../mobile/android-developer-preview-key-session.md). LOCAL_DONE / INTEGRATION_PENDING to main. Local dev fast-forward completed at d95b189; owned feature 16f3a07 and combined behavior e26d546 preserve all UI/plan commits and maintained dev 60a3fd7, including upload origin and Go security fixes. Fingerprint `9c7c4460d48c2fde18fb81091a6990934fa55cd2914181d2360f746f0773c82b`: 73 focused Android cases, POCO account/media/Keystore cases (account repeated on final APK), race/real-PostGIS/S3/API-worker checks, lint/R8/size (4,224,869 bytes), vet/govulncheck/static/contract/secret/diff checks passed. Final debug APK installed without wiping account/data. Deployment only in `abastevo-temp`: dependency-refreshed API/worker, private storage, backed-up/restored private media, migrations 43–48; unique-photo runtime READY and two prices VALIDATED. Public health ready; repeated smoke exhausted the unchanged public peer challenge quota until 08:00 Cuiaba. Owner extended the preview to seven days: cutoff 2026-10-15 07:45:50 Cuiaba. Owned deadline/restart/config checks and signed no-GPS runtime authorization passed; five private objects restored and verified. Current-source APK reinstalled by ADB without data wipe. Native credential entry and full Android gallery/server walkthrough remain owner QA. No other VPS app mutation or task GitHub publication/production/G09/iOS acceptance. Next: owner UI validation in Curitiba / POSTO PENTEST TEMP after the quota window renews; protected final batch delivery only when authorized.

- Continuation 2026-10-07: P37-PC04B Android LOCAL_DONE / INTEGRATION_PENDING on maintained dev, parent `79005fe`. [Android evidence](../mobile/p37-photo-subset-android.md); [backend slice](../mobile/p37-photo-subset-backend.md). Final serial Gradle `BUILD SUCCESSFUL in 6m 52s` (268 tasks): 61 focused unit tests (application 15, domain 9, data 15, app 22), zero failures/skips; lint debug zero errors; release 4,226,245 bytes without debug hooks; `git diff --check` and secret scan passed. POCO 2311DRK48G/API36: reinstalled current data instrumentation APK and ran isolated Room suites `PhotoReviewOutboxDeviceTest,V8ToV9PhotoOutboxMigrationTest` — `OK (3 tests)`. PC05 consent and expanded PC06 remain pending. No push, PR update, merge, wiki or deployment; no readiness claim beyond local evidence.

- Active execution 2026-10-07: P37-PC04 IN_PROGRESS on maintained dev, parent e41936c. [PC04A backend evidence](../mobile/p37-photo-subset-backend.md): capture-bound reservation/shared evidence and per-product immutable intake LOCAL_DONE / INTEGRATION_PENDING; affected race/PostGIS/HTTP-process/static/contract checks passed. Android transactional review/signed dispatcher/owner receipts are underway. Actual private S3 media, staging profile and PC05 consent/PC06 expanded acceptance remain owed; collection stays OFF. No deployment, merge or production certification.

- Active execution 2026-10-07: P37-PC03 camera/review LOCAL_DONE / INTEGRATION_PENDING on maintained dev, parent df77de6. [Behavior and evidence](../mobile/p37-camera-review.md). 42 scoped unit tests; genuine POCO four-case review suite passed normally and at 160% font; real camera/shutter/return and isolated Keystore verified. Serial lint (zero errors), release/R8/size (4,191,429 bytes), release debug-hook exclusion and static/secret/diff checks passed. Next: PC04 signed shared-photo outbox, then PC05 optional permanent training/deletion and PC06 expanded acceptance. Cellular staging TLS/directory works but the selected profile is unavailable; local host/Wi-Fi TLS chain remains untrusted. User accepted test-only contact placeholder until production. No live photo submission, collection activation, merge or production certification is claimed.


- Active execution 2026-10-07: P37-PC02 native image OCR LOCAL_DONE / INTEGRATION_PENDING, parent c1efc4b. [Measured adapter/model/limits](../mobile/p37-image-ocr.md). POCO API36: actual 12-image run and synthetic/corrupt test passed; 34 unit regressions passed. Exact smoke suggestions 21/40, 19 misses, one conditional LED digit error; p50 331.5ms / p95 769ms. APK model comparison 46.45MB bundled → 4.05MB chosen on-device Play-Services model. Crop/manual confirmation remain essential. Next: PC03 camera eligibility/review/state recovery, PC04 signed shared-photo outbox, PC05 optional training consent and PC06 expanded real acceptance. Staging verified TLS is currently unavailable from this host due network CA interception; no insecure fallback or runtime acceptance claim.

- Active execution 2026-10-07: P37-PC01 camera authorization LOCAL_DONE / INTEGRATION_PENDING on maintained dev, parent a20d484. [Contract and immediate PostGIS/auth/concurrency evidence](../mobile/p37-photo-capture-eligibility.md). Signed two-minute camera / 24-hour submission receipts preserve retry deadlines and bind one owner/key/station/evidence session. Next: P37-PC02 real Android image OCR, then review/outbox/consent and POCO acceptance. User authorized debug-only controlled proximity tests; release/backend trust remains enforced. No image recognition or deployment acceptance inferred.

- Active preparation 2026-10-07: P37-T01–T03 [photo contribution refinement](PHOTO_PRICE_CONTRIBUTION_PLAN.md), PREPARATION_LOCAL_DONE / IMPLEMENTATION_PLANNED on maintained `dev`, source inspected/built `286c418`; opening tree clean, recorded main `a424688` already an ancestor. Private corpus: 1,263 price-visible files / 1,219 unique images from 1,632 originals, preserved and hash-verified; twelve manually annotated samples staged on the user-connected POCO (model 2311DRK48G, API 36). Debug APK build/install/launch and Explore → station detail checked; selected station reports community prices unavailable and disabled update-price action. Existing 16 capture + 3 regex tests passed; no real-image recognition evidence inferred. Camera/proximity/OCR/crop/upload/retention acceptance remain NOT_EXECUTED. User decisions: initial 150m, only detected vertical fuel rows/subsets, no payment inputs, optional permanent training collection with ordinary evidence still 24h; [consent/erasure amendment](../security/AI_TRAINING_IMAGE_CONSENT_PLAN.md) is proposed, not active. No app/backend behavior change or remote publication; historical delivery/PR/check states below were not refreshed. Next source task: P37-PC01 backend photo eligibility/compatibility contract and immediate critical tests after prerequisite confirmation, then real image OCR/review. The explicit device request applies to this slice only; wider release/manual obligations remain owed.

- Active delivery 2026-10-06: P36-T01 integrated with the completed branding work on maintained `dev`. [Combined evidence](../mobile/p36-secure-storage-keyboard-integration.md). LOCAL_DONE / INTEGRATION_PENDING. Merge parents `550958c` and `4dca43c`; preserve both histories and the shared banner/username validation. Combined 303 unit/security/concurrency tests and 5 API-26 emulator tests passed, including real Keystore, signup/Continue/reopen/logout and native keyboard; final lint/APKs, static/secret/diff checks passed. Reuse protected PR [#124](https://github.com/AlexandreZanata/abastevo/pull/124), base `a424688`; next: normal dev push, update cumulative PR, guarded finish with current-head `Quick verification`, then merged-SHA wiki publication. Pending checks stop finalization without polling; no deployment or runtime certification is claimed.

- Prior isolated repair 2026-10-06: P36-T01 secure local key/session custody and global keyboard inset consumption. [Behavior and evidence](../mobile/p36-secure-storage-keyboard.md). LOCAL_DONE / INTEGRATION_PENDING on isolated `codex/account-keyboard-fix`, parent dev `d0a3565`; original dev is occupied by another contributor and remains untouched. 92 unit/security/concurrency tests, 5 API-26 emulator tests, Android lint/APK and static/secret checks passed. Create → automatic login → Continue → fresh rehydration → explicit offline logout verified with synthetic backend and real Keystore. Next: integrate the isolated commit into dev after owner coordination, revalidate combined UI inputs, then protected dev → main delivery; remote/live/hardware/iOS certification is not claimed.

- Prior station-page task 2026-10-06: P35-T02/P36-T02 branded full station-page extension on maintained `dev`, parent `7e22a3a`. [Behavior and evidence](../mobile/p35-station-page.md). Status: LOCAL_DONE / INTEGRATION_PENDING. Exact CNPJ binding, real PostGIS, negative/cache/late-response/auth regressions, 77 final app tests, Android lint/APK, contracts/static/secret checks passed. Source fingerprint and exact commands are recorded; hardware/staging runtime remain OWED.
- Prior authorized account-continuity task: commit `7e22a3a`, [evidence](../mobile/p36-account-continuity.md), LOCAL_DONE / INTEGRATION_PENDING. PR [#124](https://github.com/AlexandreZanata/abastevo/pull/124) remained OPEN at this task's final read, head `7e22a3a`, base main. Prior guarded finish refused pending Quick verification; no merge/wiki occurred. Retain that source work in the cumulative delivery.
- Actual history reconciled: dev/origin/dev contained 11 commits beyond origin/main at opening; the station-page extension adds one atomic source commit. Final fetch verified main `a424688` is still an ancestor; no source history rewritten. The consolidation notes below describe the earlier integrated batch.
- Delivery: reuse existing protected dev → main PR #124 and update its cumulative scope. Required `Quick verification` must apply to the new head/base; prior evidence cannot authorize this head. Next: atomic commit, normal dev push, guarded `bash scripts/git-flow.sh finish --base origin/main --pr 124 --required "Quick verification"`, then one merged-SHA wiki mirror. Missing/pending checks block integration without CI polling. Final remote outcomes belong in PR metadata/local delivery notes, then the next useful commit. No deployment, tag or device execution follows from source acceptance.

- 2026-10-06: repository consolidation FINALIZED (INTEGRATED). Cumulative PR #122 merged to main (merge a424688, head 0a95153); all mobile/backend source now on main.
- All 7 remaining open issues (#13, #53, #60, #61, #62, #119, #120) closed administratively as not planned, each with retained obligations pointing at [the consolidation record](repository-consolidation-20261006.md). At consolidation, 0 open issues and 0 open PRs remained on the remote.
- Old work branches retired locally and remotely (20 local / 17 remote refs, each verified as ancestor of the merge head before deletion). Only main and dev remain, on origin and locally.
- At consolidation, local main and dev were synchronized at a424688 with a clean tree. Subsequent source work follows ADR-020 maintained dev → protected main delivery.
- Production private representation remains contained with 503/no-store while contributor proof/binding and private proof storage stay unaccepted; public profile, prices, community and free account source remain available. Administrative closure never certifies incomplete gates.
- Standing reservations unchanged: G18 NOT_ACCEPTED; iOS DEFERRED_EXPLICIT_RESUME_ONLY; G09 UNCERTIFIED; G24 user-accepted with manual evidence still owed. No deployment, tag, trust bypass or public pilot followed this integration.
