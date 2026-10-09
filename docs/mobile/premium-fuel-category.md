# Premium gasoline category and icon

Status: LOCAL_DONE
Validation: PASS

User-authorized 2026-10-09 extension of P37-T01/T02. Add a distinct premium
gasoline grade to manual review, vehicles/filters, OCR, signed submission,
receipts and source-separated community reads. National Rust ingestion is a
separate planning-only task; no importer implementation follows from this slice.

## B-BR-PREMIUM-01–06 / BUC-PREMIUM-01–04

1. Preserve historical Android `GASOLINE_PREMIUM` = common additived gasoline;
   never rename persisted values or reinterpret existing prices. Append
   `GASOLINE_PREMIUM_GRADE`, wire-identical, unit L, to represent true premium.
2. Premium is a grade, Podium is a commercial brand. This slice adds the grade,
   not brand-specific price groups or an availability inventory. ANP official
   rows stay distinct; absence of premium survey data must stay empty.
3. Recognize explicit `GASOLINA PREMIUM`, `GASOLINA PODIUM`, `GASOLINA OCTAPRO`
   and `GASOLINA V-POWER RACING` (including split OCR label lines). Bare Podium
   or premium is ambiguous because diesel may carry those labels. Explicit
   diesel wins its own valid S10/S500 grade; unknown diesel specifications and
   contradictory mixed-fuel labels require manual review, never coercion.
4. Premium, common and additived prices coexist independently; repeated
   conflicting premium prices remain manual. Preserve exact milli-BRL, units,
   conditions, owner/proof/receipt/idempotency and photo deadlines unchanged.
5. Existing unknown-enum rejection remains; new clients require a backend
   accepting the additive wire value before live premium submission. No
   production deployment or staging readiness can be inferred from source tests.
6. New icon uses ruby/magenta #C2185B (not an existing fuel icon ink), matching pump,
   droplet, hose, weight and size; a crown distinguishes premium. Generate PNG
   first, preserve original, trace real SVG paths and export the Android PNG.
   White interior details are neutral cutouts, not another category color.

BUC-01: select premium manually → exact frozen premium-grade command.
BUC-02: read a mixed gasoline board → distinct correct products and amounts.
BUC-03: ambiguous brand/grade → manual choice, never automatic gasoline.
BUC-04: submit/retry premium → same owned fact and source-separated receipt/feed;
invalid units/amounts and unknown legacy wire names are refused.

Official ANP label parsing and its seven-product storage constraint are not
expanded with invented survey labels. Community storage already uses TEXT
products and generic exact-money rows, so no migration is required for this
additive category. Existing reviewed station coordinates and 150m rule remain
unchanged.

## Validation and delivery boundary

Isolated worktree `codex/premium-fuel`, source parent `e6772c5`; shared checkout
login edits were preserved. Domain RED proved the missing grade; backend RED
rejected it as unknown before implementation. Final domain suite: 484 cases,
including 37 spatial OCR cases. Application/data/app full unit suites passed;
one unrelated opt-in live catalog test remains skipped. A subsequent focused
signed-transport regression also passed after adding the premium wire case.
Eight localized labels, legacy additive mapping and light/dark contrast remain
covered. All backend consumers compile; affected Go vet and OpenAPI compatibility
checks passed, including the valid premium vector and legacy-name refusal.

Real disposable PostGIS + race detector passed the Community suites, kernel,
official filters and API contracts. Shared-photo concurrent eight-way replay
stores exactly one premium fact/job; changed-money retry is refused. City feed
and current-price reads keep common/additive/premium independently exact, with
zero invented official observations. No SQL/schema migration was necessary;
ANP source-label parsing and official storage constraints remain unchanged.

On the connected POCO (API36), four final native cases passed in 4.35s: three
Room lifecycle/atomic-claim cases including premium payload/receipt after reopen,
and actual ML Kit recognition of four explicit premium gasoline labels on mixed
boards plus an ambiguous bare-Podium board. Generated pixels/synthetic database
only; no contribution or personal image was submitted by these tests. This is
regression evidence, not national OCR accuracy certification.

Commands (repository root except Go from backend/):

```sh
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :data:lintDebug :app:assembleDebug :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest --no-parallel --max-workers=2 --console=plain
./gradlew :app:testDebugUnitTest --tests '*FuelProductTintContrastTest' :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest --no-parallel --max-workers=2 --console=plain
./gradlew :data:testDebugUnitTest --tests '*ContributionUploadHttpClientTest' --no-parallel --max-workers=2 --console=plain
go test -race -count=1 -tags=integration ./internal/modules/community/... ./internal/modules/kernel ./internal/modules/official/application ./internal/platform/apicontract
go test -run '^$' ./...
go vet ./internal/modules/kernel ./internal/modules/community/... ./internal/modules/official/application ./internal/platform/apicontract
bash scripts/check-compat.sh
bash scripts/check-mobile.sh --static-only
bash scripts/scan-secrets.sh
git diff --check
```

The first Go integration invocation passed write/race suites but found a test
compile error in the new read assertion; it was fixed and the complete affected
read package passed with real PostGIS/race. Final lint reports have zero errors.
The icon's PNG→SVG process, exact prompts/tools and measured geometry fidelity
are recorded in [the asset workflow](../assets/fuel-icons/README.md). Final ruby
silhouette IoU 99.7754%, ink 99.4679%, white details 99.3403%; six SVG paths, no
embedded raster, 128x128 Android export. Final color resources/APK were rebuilt.

LOCAL_DONE / INTEGRATION_PENDING: backend deployment and main-app installation
were not performed in this isolated slice. The service must accept the new wire
grade before a client can successfully send it; no live premium receipt or
availability inventory is claimed. Keep the tested APK as a reviewable artifact.
Existing independent PC06/consent/release acceptance remains OWED.

Behavior/asset/contract source fingerprint: `cd50642f9ea76ef6cd6480c8b16e2c69a66ad452eda9202bc1a9da2929be017a` over 32 paths.
