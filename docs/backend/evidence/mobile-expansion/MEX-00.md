# MEX-00 — Reconciled construction baseline and ownership

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-20

## Scope and acceptance fixed before editing

The maintainer selected only the first foundation microtask. This closes
[MEX-00](../../../planning/MOBILE_EXPANSION_EXECUTION_PLAN.md), B-BR-MEX-20 /
BUC-MEX-10: inspect committed source, preserved work, existing checkpoints,
tools/default flags and dependent ownership. Acceptance is document/source
inventory validation; no new product behavior, schema, entitlement, source
activation, provider/device access or deployment. EE-00 and MEX-01 are not run.
Risk: source integrity and false acceptance. Recovery is a forward documentation
correction; no product data is touched.

## Exact inputs and reconciliation

- Owned worktree: `.worktrees/mobile-expansion-plan`, branch
  `codex/mobile-expansion-plan`; opening head `35accb9e`, clean.
- Plans: `47fded8c` (MEX) and `35accb9e` (EE amendment), originally based on
  `fb85d4d1`. Captured maintained dev is now
  `a2f8925cc178c0b887a7386b121b8af77b128611`.
- Normal local merge of that dev into the owned worktree, retaining both
  histories, had no conflicts. Its only inherited runtime delta from the
  planning head is the already committed Home/manual-sync correction
  `a2f8925c` (15 files). No new runtime edits are introduced by this task;
  the completed tree's non-documentation files match captured dev byte-for-byte.
- Main local ref is `a349d7c0`, an ancestor of captured dev, but is not evidence
  of current remote main. Cached `origin/main` is `2b9cf1e4`;
  cached `origin/dev` is `a2f8925c`. `git ls-remote --heads origin main dev`
  failed because this runner could not resolve github.com. No fresh remote
  status/CI/protection/synchronization claim is made; refresh before delivery.
- Primary detached checkout `e22824db`: 16 status entries (15 modified files,
  one untracked manual-sync store), including foreign evidence/store work.
  Maintained dev's `main-reconciliation` worktree was clean at capture.
  Neither checkout was switched, staged, stashed or edited.
- Other OPT owner: `/tmp/bfp-opt-feed`, `codex/opt-home-community-feed`, clean at
  `6e8b306a`. It is not merged here. All other occupied worktrees remain intact.

The inventory below refers to captured dev plus committed plan documents.
Historical test results are reused as references, not rerun or upgraded to
current full acceptance. Any new relevant input invalidates affected evidence.

## Canonical ownership and outstanding acceptance

1. **App/staging entry — existing P34–P38 / G09-LOCAL.** Reuse
   [environment contract](../../../mobile/p34-t02-environment.md),
   [synthetic catalog](../../../mobile/p34-t03-test-catalog.md),
   [P38 handoff](../../../mobile/p38-t03-candidate-handoff.md) and newer
   [P35 native evidence](../../../mobile/pagination-sync-device-20261020.md).
   Their pinned commits resolve and are ancestors of captured dev. Newer native
   evidence records actual pagination/sync checks; it does not close the whole
   product/manual/release matrix. Staging configuration/source integration can
   be reused; live TLS/readiness/providers are not probed here. G09 production,
   historical full G18 and each still-owed provider/device gate stay separate.

2. **Vehicle slots — PRM02 owner; MEX-01/07/08 amendments.**
   `domain/.../rule/MaxRegisteredVehiclesRule.kt` still permits three.
   `application/.../usecase/vehicle/SaveVehicleUseCase.kt` performs count then
   save; this is not atomic free-slot admission. Machine policy
   `contracts/premium/policy-v1.json` still has max_vehicles_per_account=3.
   Freeze one free vehicle plus Premium extra slots first; preserve three as
   current technical ceiling and keep legacy/expired data accessible under the
   existing capability matrix. Do not change only the constant. Pure EV
   capability/model work has EE ownership before dependent migrations.

3. **Private cloud/backup — PRM02–04, PRM08, PRM09-T01.** Reuse existing
   `backend/internal/modules/cloudsync`, `subscription`, their actual API root
   wiring in `backend/cmd/api/main.go`, and the owner-scoped Premium command /
   cursor Room stores. [PRM evidence](../premium/PRM_NEXT_TEN.md) contains local
   checkpoints through PRM09-T01; all 30 unique explicit "tested working tree
   atop" commit pins resolve and are ancestors of captured dev. The older
   [gate matrix](../premium/PRM08_T05_GATE_MATRIX.md) remains historical (its
   migration counts/PR state are not today's state). Complete Android transport,
   consent/conflict/restore journeys and real Play/object/push provider proofs
   remain obligations. MEX-14–16 / EE-24 extend these owners, not new replicas.

4. **Personal ledger/reports/alerts — PRM05–08.** Reuse actual `ledger`,
   `metrics`, `alerts` modules and API root composition; pure Kotlin
   `domain/.../premium/PremiumConsumption.kt` and PRM09 stores already exist.
   PRM05/06/07 checkpoint coverage is recorded in the same PRM evidence; its
   checkpoint commits `e89ab020` (report API), `1482ba59` (alerts pipeline),
   `c2b69cfc` (backend checkpoint) and `a104ed80` (Android ports) are included.
   Complete Android ledger/report/cloud journeys are not accepted merely from
   those ports. MEX-09–16 and EE-11/15/16/20/23/24 own exact math, integration
   repairs, charging entries and reports without duplicate financial services.

5. **Fast places — existing Room/FTS; MEX-03–06.**
   `data/.../repository/MunicipalitySearchRepositoryImpl.kt` has batched key
   lookup but its approximate fallback still loads `findAll()`.
   `application/.../usecase/location/SearchMunicipalityUseCase.kt` gates on
   imported weeks and calls catalog enrichment per result. Existing search
   tests live in `application/src/test/.../usecase/location/`.
   Measure before changing: bounded candidates, batch enrichment and separating
   location existence from price availability are still required. EE discovery
   consumes accepted MEX places; no separate city catalog is needed.

6. **Official quality/fines — existing FQ; MEX-17–27.** Reuse actual Rust
   preparers in `tools/station-prep`, prepared/native bridges, frozen quality
   leases, jobs, Directory ports and Android quality readers.
   [FQ07R6D](../fuel-quality/FQ07R6D_COMPANY_WORKER_COMPOSITION.md), commit
   `0e68021b`, records seven-resource worker census 35,911 = 35,612 accepted +
   293 CPF quarantines + six metadata rows. These are historical source results,
   not a fresh national census. [FQ07R6E1](../fuel-quality/FQ07R6E1_SCOPED_COMPANY_READS.md),
   commit `c9ffbbf6`, has scoped read implementation but only two new test
   functions in `quality/adapters/read/company_scope_integration_test.go`.
   Missing demonstrated read-lock concurrency, scoped EXPLAIN and required
   failure/negative acceptance remain MEX-18's responsibility before exposure.
   MEX-19 owns HTTP/root/cursor/ETag; MEX-20a–f own real enforcement XLSX,
   strict bridge/storage/publication/worker/full-source acceptance. Do not
   confuse an existing synthetic enforcement adapter with real XLSX acceptance.
   Revalidation/rebound, source-global filters, safe current-company attribution
   and Android occurrence journeys still need their own accepted checkpoints.

7. **Home/Community efficiency — OPT owns implementation; MEX-32 coordinates.**
   Captured dev still has `delay(15_000)` in `CommunityFeedViewModel` and the
   fixed-frequency `feed_auto_refresh` copy. Separate OPT commits `f60bb3e2`
   (inventory), `8de22c24` (copy) and `6e8b306a` (Room snapshots) exist and
   resolve, but are not ancestors of captured dev. Read their actual diff and
   `docs/mobile/opt-home-community-feed-phase3.md` on that branch before reuse.
   Its claimed LOCAL_DONE is not acceptance by this audit: it persists responses
   still contracted no-store and explicitly skips OPT-A02. Reconcile that with
   [OPT's explicit caching prerequisite](../../../planning/HOME_OFFLINE_EFFICIENCY_PLAN.md)
   and immediate real SQLite upgrade/reopen/rollback requirements. Evidence
   lists only unit migration checks and leaves native reopen/upgrade owed.
   Review before integration; do not duplicate the code or silently waive gates.
   Room is 16 on captured dev versus proposed 17 on that OPT branch; allocate
   future additive versions from the reconciled schema, not stale plan numbers.
   Preserve `a2f8925c` manual-sync correction alongside that future reconciliation.

8. **National exploration / EV support — new MEX/EE scope.** Existing station
   and price readers are building blocks, not acceptance of the master query
   engine. MEX-28–31 own comparable liquid-price projections/typed questions;
   EE-25 extends that owner. `Vehicle` still requires tankCapacity/FuelProduct,
   and FuelProduct contains liquid/gas enums with legacy premium semantics.
   No accepted EV implementation checkpoint was found. EE-00 must perform the
   detailed EV/carrier/schema inventory next; EE-01 must qualify actual source
   rights/files. Research is not feed approval, electricity is not a new liquid
   fuel enum, and no tariff/default/live-availability behavior is added here.

## Source defaults versus observed runtime

Inspected `backend/internal/platform/config/config.go`: quality reads/reports
default true; imports, PMQC/fines discovery and revalidation schedules default
false. ANP discovery defaults true. Actual deployed overrides are UNKNOWN in
this audit; no environment files, secrets, databases or VPS were inspected.
`app/build.gradle.kts` QUALITY_UI_ENABLED and data QUALITY_CACHE_ENABLED /
QUALITY_OUTBOX_ENABLED default true through Gradle properties. No EV provider
feed was configured or activated. No real payment readiness follows from API
wiring; unconfigured capability/provider checks remain authoritative.

## Tools and exact verification boundaries

Available: Python 3.12.2, Rust/Cargo 1.99.0, JDK21, Gradle wrapper8.10.2,
sqlc1.31.1, staticcheck0.8.1, gitleaks8.30.1 and Docker CLI29.1.3.
Root `go version` is1.22.2; inside backend it selects pinned1.27.2 correctly.
`./gradlew --offline --version` initially hit a read-only wrapper lock; the
same version-only command succeeded with normal sandbox escalation. No build
or device action followed. Docker CLI presence is not daemon/PostGIS readiness.

Verified command definitions, not executed product acceptance in this task:

- `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest`;
  affected `:app:lintDebug :app:assembleDebug` as defined by
  [test strategy](../../TEST_STRATEGY.md).
- `cargo +1.99.0 build --locked --manifest-path tools/station-prep/Cargo.toml --bin prepare_registry --bin prepare_quality --bin prepare_sanctions --bin prepare_fine_history --target-dir /tmp/abastevo-integration-rust`.
- From backend, documented disposable DB/preparer variables first, then
  `go test -race -tags=integration ./internal/modules/quality/... -count=1`
  and `go test ./internal/platform/apicontract -count=1`.
  [Backend README](../../../../backend/README.md) owns exact variables and DB
  provisioning; don't assume a port is disposable or expose credentials.
- `sqlc vet`, affected gofmt/build/vet/static checks and
  `GOTOOLCHAIN=go1.27.1 staticcheck` per backend tooling guidance when affected.
  Do not use compile-only selectors as behavior proof or run native commands
  with clearPackageData on a personal phone under this task's authority.

Actual MEX-00 validation: 17 audit commit pins resolve and 14 P38 handoff pins
remain ancestors of captured dev, plus the 30 PRM evidence bases above; dependency
JSON 78-node/77-unit DAG and full topological order; 26 new relative links resolve;
runtime-tree equality to captured dev; scoped document secret review plus
`bash scripts/scan-secrets.sh`; `git diff --check` and staged-path review.
No runtime tests are claimed for this documentation-only audit. No remote
publication, CI query, PR, merge to main, wiki, source activation or device run.

## Next boundary

Stop after this task as requested. [Continuation instructions](../../../planning/MOBILE_EXPANSION_CONTINUATION.md)
start with MEX-01, or independently EE-00, on refreshed accepted inputs. The
[combined dependency map](../../../planning/EV_MEX_DEPENDENCIES.json) remains
planning metadata; only MEX-00 changes to LOCAL_DONE in this audit.
