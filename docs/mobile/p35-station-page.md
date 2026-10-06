# P35/P36 — Branded full station page

Authorized scope: replace legacy/canonical station bottom sheets with a full Android
page; default station icon/banner, source-separated prices, fuel experience ratings,
comments/report menu, and representation information. Maintained dev, parent 7e22a3a.
Risk: canonical identity binding and authenticated social participation. No privilege
transfer, business image upload, schema migration, deployment or iOS work.

## Behavior contract before implementation

- B-BR-SP01: every station projection has the same local Abastevo default artwork. The banner and icon are placeholders, never a station partnership/representation badge. No arbitrary remote image URLs or new tracking/dependency.
- B-BR-SP02: selecting a station navigates to a restorable full page with shared bottom navigation, standard top bar/back, safe drawing insets, scrolling and Material blue/green styling. The route carries only station identity and fuel, no tokens, draft or price snapshot.
- B-BR-SP03: legacy ANP CNPJ resolves only through an exact, validated active Directory identifier; no name-based matching or fabricated UUID. Add an anonymous read-only by-CNPJ contract using existing owned parametrized queries. Unknown/retired identifiers never create a station or enable social writes. Local dated ANP browsing remains available when resolution/network fails.
- B-BR-SP04: selected fuel scopes ratings/comments/prices/capture. Personal 1–5 ratings are motorist experience, not laboratory quality. Guests can read; writes reuse current live auth, existing moderation/idempotency/rate limits and 280-scalar plain text. Blocking social HTTP reads and writes run on IO, never the interface thread. Preserve comments and rating totals separately; cancelled/obsolete target responses cannot repaint another fuel/station.
- B-BR-SP05: comment menu has explicit report confirmation/reason, never automatic removal; truthful receipt/offline/retry states. Source price confidence and feedback stars remain separate. Existing wire reasons/limits govern.
- B-BR-SP06: the owner invitation says “Do you manage this station?” and explains free account, authority verification and independent review. A public station identifier grants nothing. Existing claim routes remain contained by server availability; beta guide never invents a support channel, approval or management access. No private evidence upload from this new information card.

## Use cases

- BUC-SP01: station list → full page → default artwork/name/address → select fuel → source-separated price/history → route or contextual update.
- BUC-SP02: resolved canonical station → experience stars/comments → free sign-in if needed → existing authenticated command → honest success/refusal/retry.
- BUC-SP03: comment ⋮ → report reason → confirm → server acknowledgement; criticism remains independent of representation.
- BUC-SP04: owner invitation → beta guide → existing verification journey when available; no instant claim or fake badge.

## Acceptance evidence

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Tested 2026-10-06 on maintained `dev`, parent `7e22a3a`. Behavior-source SHA256
`9e474e2422212466b26bb672cfd17efc28dd9ba0e790a42e685cba7c886c3459`: SHA256 over sorted changed/new `app/`,
`application/`, `data/`, `domain/`, `backend/`, `contracts/` paths, each path,
NUL, file bytes, NUL. Documentation excluded to avoid self-reference.

### Scoped checks

- RED: directory endpoint returned 404 for valid/invalid identifiers before its route existed; domain and application identity tests failed before their implementations existed. View-model negative cases exposed foreign identity acceptance and unrecovered transport exceptions, then passed after guards were added.
- GREEN domain/application/data: `./gradlew :domain:test --tests '*StationPageIdentityTest' :application:test --tests '*ResolveStationByCnpjUseCaseTest' --tests '*GetServerStationsUseCaseTest' --tests '*GetFeedbackPageUseCaseTest' --tests '*SubmitFeedbackUseCaseTest' :data:testDebugUnitTest --tests '*DirectoryStation*Test' --tests '*FeedbackHttpClientTest' --console=plain` (executed in the combined scoped command with app checks): domain 2; resolver 7; directory list 7; feedback reads 8/writes 8; HTTP directory 5, codec 3, repository 2, Room cache 4, authenticated feedback HTTP 11; all passed.
- Final Android: `./gradlew :app:testDebugUnitTest --tests '*StationPageViewModelTest' --tests '*StationsViewModelTest' --tests '*StationsServerDiscoveryTest' --tests '*RoutesTest' --tests '*NavigationTabTest' --tests '*FeedbackViewModelTest' --tests '*StationProfileViewModelTest' --tests '*CommunityPriceDisplayTest' --tests '*StationPriceRowA11yTest' --tests '*CommunityFeedViewModelTest' :app:lintDebug :app:assembleDebug --console=plain` passed: 77 tests; lint 0 errors/184 warnings; debug APK generated at `app/build/outputs/apk/debug/app-debug.apk`.
- Cases include route restoration/injection rejection, full numeric/alphanumeric identifier shape, unknown/foreign/retired identifiers, no creation on read, stale identity write denial, ambiguous/broken cache, previous-fuel late responses, local dated ANP survival during outage, and no old fuel price replay after selecting another fuel. The public social read dispatcher test reproduced a Main-thread read before the IO correction; stats and comments then passed on the separate IO dispatcher. Existing signed feedback tests cover expiry/refusal/retry/idempotency and blank/oversized text.
- Backend: `cd backend && go test -race -count=1 ./internal/modules/directory/... ./internal/modules/feedback/... ./internal/platform/apicontract && go vet ./internal/modules/directory/...` passed. Final added cache/anonymous-read checks: `go test -race -count=1 ./internal/modules/directory/adapters/http` passed, including public ETag/304, identity-header independence and no-store negative responses.
- Disposable real PostGIS using `infra/compose.validation.yml`, a unique validation project and all 42 existing migrations via `go run ./cmd/migrate`: `go test -race -count=1 -tags=integration ./internal/modules/directory/adapters/read ./internal/modules/directory/adapters/http` passed. Exact numeric/alphanumeric resolution and retired-identifier/non-creation cases included. Fixture/network removed; no live database touched. Existing parameterized ResolveActiveIdentifier query reused; no migration or SQL change.
- `/home/iiii/go/bin/vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check` passed: 0 errors, 58 warnings/40 informational findings. New public operation uses the existing Station projection and bounded cache contract.
- `bash scripts/check-mobile.sh --static-only`, `bash scripts/scan-secrets.sh`, `bash scripts/check-security.sh --static-only`, `git diff --check` passed. All 8 locale XML files parse with unique names, 31 new station-page strings and matching format placeholders. No lint suppression, dependency, remote art fetch, token/PII logging or private document collection added.

### Result and limits

ANP and canonical list entries, plus city-feed entries with their selected fuel,
open the full station page. Shared bottom navigation, standard top bar, safe drawing
insets, scrollable cards and keyboard padding follow the app wrappers. The local
SVG-derived banner/icon are explicitly beta default artwork. Station domain
projections carry local artwork identity; the public backend does not advertise
station-uploaded images or invented partnership metadata.

Personal ratings are submitted explicitly after selecting 1–5 stars. They remain
separate from price confidence. Rating totals and comments use separate retained
view models keyed by station/fuel/account. Reporting uses comment menu, reason,
confirmation and honest receipt; it does not hide criticism or confer management.
Disabled/expired/offline states preserve server authorization and stable retries.
Public representation badges reuse ProfileBadgeRule and require a permitted source.

The owner card explains the free account/authority/review journey. Representation
management remains unavailable in this beta; the guide does not submit an application,
collect documents, invent a contact channel or activate unaccepted private endpoints.
Existing server 503/no-store containment and public-source independence remain intact.

Hardware/emulator visual QA (light/dark, narrow screens, large fonts, TalkBack,
safe areas, maps, keyboard, back navigation and process restoration) and staging
runtime remain OWED under the standing deferral. JVM/lint checks do not certify
those behaviors. No device, deployment, production account, tag, iOS work or public
pilot was executed. G09 remains UNCERTIFIED. This accepts only the scoped source
extension, not all P35/P36 or live representation readiness.

Rollback: revert this atomic source commit through protected dev → main delivery;
preserve existing account keys, migrations and signed-feedback rules. Backend and
app should ship together for legacy CNPJ resolution; older servers return honest
unavailable/pending participation without fabricating a canonical station. Next:
reuse PR #124, current-head/base Quick verification and guarded finish, then one
merged-SHA wiki mirror. No incomplete tracker receives a false completion claim.
