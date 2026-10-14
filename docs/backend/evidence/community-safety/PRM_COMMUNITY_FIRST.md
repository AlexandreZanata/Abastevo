# Community-first source construction

Status: IN_PROGRESS

Opening: isolated codex/community-first from origin/dev=origin/main 68483db9. Premium predecessor commits already integrated. Primary detached e22824db and its unrelated dirty evidence adapter preserved. Scoped planning commit cherry-picked as 8e52aa2e, retaining current PROGRESS history. User authorized source implementation and GitHub PR to main; no merge or VPS activation requested.

## PRM15-T01

LOCAL_DONE / INTEGRATION_PENDING: ADR-021 and active contract supersession before behavior. Existing 150m evidence preserved; normal receipt admission, optional observation claims, nearby geometry and price confidence audited separately. Documentation links/whitespace checked. Runtime unchanged at this checkpoint.

## PRM15-T02

LOCAL_DONE / INTEGRATION_PENDING: COMMUNITY_FIRST_POLICY freezes v2/legacy/debug/unknown receipt behavior, server-first rollout, incompatible-client recovery and rollback. Original deadlines and owner/key/station/origin/evidence bindings retained. Documentation diff check PASS; implementation tests belong to PRM18/PRM19.

## PRM15-T03

LOCAL_DONE / INTEGRATION_PENDING: existing consensus thresholds retained under a new version without proximity prerequisite; existing helpfulness/moderator authority/budgets retained. Frozen reviewed-coverage denominator/numerator, strict >90% and owner-reviewed product evidence; PRM22 DEFERRED. No new numeric vote/adoption threshold invented. Scoped document/diff review PASS.

## PRM16-T01

LOCAL_DONE / INTEGRATION_PENDING: new registry wire regression proves eight valid nominal-address rows survive with null latitude/longitude and unknown quality; no coordinate columns required. `CARGO_TARGET_DIR=/tmp/prm-community-rust cargo test --locked --manifest-path tools/station-prep/Cargo.toml community_first_registry`: 1 passed. Existing parser already supports this contract; no speculative parser rewrite.

## PRM16-T02

LOCAL_DONE / INTEGRATION_PENDING: rejected optional PMQC coordinates cannot remove registry identity/address or manufacture points; explicit join regression preserves all eight serialized registry facts. Existing quarantine and deterministic join contract retained. Rust join suite 8/8 PASS; locked prepare_registry build PASS.

## PRM16-T03

LOCAL_DONE / INTEGRATION_PENDING: actual Rust prepare_registry→Go loader/reconciliation/HTTP publication, replay, refresh and overlap cases passed in directory/adapters real disposable-PostGIS suite (25.095s). Prepared refresh preserves UUIDs and reviewed points, absent points stay unknown; no consumer schema/parser change required. `CARGO_TARGET_DIR=/tmp/prm-community-rust bash scripts/check-rust-fast.sh`: fmt/clippy/locked tests PASS. Broader Directory subpackage campaign recorded below.

## PRM17-T01

LOCAL_DONE / INTEGRATION_PENDING: new real-PostGIS regression finds two homonymous address-only establishments, preserves their distinct UUIDs and addresses through stable pagination/detail, and keeps coordinates null/quality unknown. `go test -race -count=1 -tags=integration ./internal/modules/directory/adapters/read -run '^(TestCommunityFirst|TestSearch|TestHandoff)'` PASS (9.830s). Canonical schema/reads already support optional points; no migration required.

## PRM17-T02

LOCAL_DONE / INTEGRATION_PENDING: existing bounded parametrized/literal-escaped name search reused; documented case/accents, page bounds, stable cursor and address disambiguation. Full real-PostGIS Directory campaign `go test -race -count=1 -tags=integration ./internal/modules/directory/adapters/...` PASS (nine package summaries in /tmp/prm-directory-tests.log), including constant-round-trip/failure recovery/query plan/municipal/HTTP/cursor/source trust cases. Scoped read regression PASS above.

## PRM17-T03

LOCAL_DONE / INTEGRATION_PENDING: extended real prepared publication→anonymous HTTP name search→profile verifies nominal address and explicit null coordinates; actual canonical IDs preserved. Targeted TestPreparedPublicationThroughHTTPAndProfiles race/PostGIS PASS (2.042s). Broader Directory nine-package race/PostGIS campaign PASS. Ready for normal capture consumer; no live deployment claimed.

## PRM18-T01

LOCAL_DONE / INTEGRATION_PENDING: RED no-GPS regression failed with capture-ineligible before change. GREEN normal capture issues photo-capture-community-v2 from mandatory active canonical-station port, without geometry; optional claims still verified against forgery. HTTP accepts absent location, rejects malformed UUID/body; append-only 000091 admits v2 while retaining v1/debug. Camera/24h deadlines unchanged. Application/HTTP race suites PASS (1.031s/1.030s); capture real-PostGIS ownership/replay/concurrency/address-only/legacy/unknown-policy/inactive-station/migration-replay campaign PASS (4.233s). Fixture follow-up correctly restored hidden owner/key fields when seeding legacy receipts; no production rule weakened.

## PRM18-T02

LOCAL_DONE / INTEGRATION_PENDING: legacy/v2 receipt-use matrix checks wrong owner/key/station/policy/session, immutable capture instant, expiry and future timestamps for both policies. Added forged optional claim and missing station-port refusal; absent location remains valid. Application/evidence application race suites PASS (1.053s/1.078s), preserving reservation/body/owner/media-capacity gates and all existing binding/replay behavior. Downstream geometric trust is replaced in PRM20 before Android consumption.

## PRM18-T03

LOCAL_DONE / INTEGRATION_PENDING: extended actual signed API/worker campaign runs the capture-bound two-fuel upload/submit flow in normal no-GPS mode as well as debug preview, checking provenance, private owner reads, validated observation decisions and malformed evidence refusal. `go test -race -count=1 -tags=integration ./cmd/api -run '^TestPublicProcess(IdentityAndSignedWrites|DevelopmentPhotoPreview)$'` PASS (22.549s) with unique disposable PostGIS and private MinIO. Receipt/observation decisions are not price-consensus certification. No VPS/device mutation or live retention proof claimed.

## PRM20-T01 — executed before Android consumers

LOCAL_DONE / INTEGRATION_PENDING: RED v2 consensus regression refused unknown version; GREEN v2 validated-media/independent-support confidence is identical without/with proximity, while v1 golden replay retains its original behavior and one supporter stays LOW. Runtime signal derivation uses signals-community-v2 without station-geometry reads or geometric penalties; other evidence risks retained. Worker photo input no longer depends on location-signals lookup. Existing thresholds/actor reduction/expiry/conflicts unchanged. Append-only 000092 removes only geometric projection penalties, preserves historical audit inputs and marks old price keys due for the existing bounded sweeper (no bulk job insertion). Real migration rollback/replay and retained no-photo-risk tests PASS. Domain/application/worker race suites PASS; full community/moderation/feedback race/PostGIS campaign PASS (11 packages, /tmp/prm-community-moderation.log). No SQL changes to historical audit/official facts.

## PRM20-T02 backend reference checkpoint

Backend contract LOCAL_DONE / Android wiring IN_PROGRESS. Inspection found the current feed/station price icons were local-only toggles and fabricated an extra supporter on tap, while the existing helpfulness client used unsigned transport. Added optional opaque visible VALIDATED observation/revision reference to the existing public feed query (one SQL round trip), without attribution/media. Real read/HTTP race/PostGIS suites PASS; positive/absent/hidden revision regression and sqlc vet PASS. Existing helpfulness change/remove/exact tally, structured dispute/report, operator action and privacy/concurrency suites already passed; Android must wire those actual signed outcomes before this task closes.

## PRM19-T01

LOCAL_DONE / INTEGRATION_PENDING: normal city-scoped nominal picker reuses Directory with debounce/cancellation/generation protection, homonym address/CNPJ disambiguation, explicit selection and existing manual city navigation. No developer flag/key needed for lookup. Domain/codec now retain nominal address (RED codec test reproduced loss; GREEN); Room 12→13 appends address_json and preserves existing catalogs/vehicles/outboxes, with generated schema13. Actual host SQLite upgrade/rollback/null-point/data/address round-trip PASS; native MigrationTestHelper case added (execution OWED). Domain/application tests PASS; full data JVM and 26 capture ViewModel JVM cases PASS. Initial environment mismatch corrected to existing /data/dev/android/sdk/Sdk; new test import fixed without weakening assertions. Camera admission switches in T02.

## PRM19-T02 — 2026-10-14

LOCAL_DONE / INTEGRATION_PENDING: RED normal camera test reproduced the fresh-fix prerequisite. GREEN uses signed authorizeCommunity without requesting GPS, accepts address-only selection, and rejects legacy/unknown fresh response policy. Refused server/expired receipt/changed owner/target and duplicate-tap guards retained. Explicit city/post changes cannot silently reinstate a previous contextual target. Normal Compose entry no longer launches location permission; actionable authorization copy updated in eight locales. Focused PhotoCaptureHttpClient (3) and CaptureOcrViewModel (26) JVM tests PASS, Gradle BUILD SUCCESSFUL (23s). Legacy persisted drafts remain server-compatible; no receipt renewal/developer fallback.

## PRM20-T02 Android signed actions checkpoint

LOCAL_DONE / INTEGRATION_PENDING: replaced feed/station local fake supporter increments with the existing helpfulness panel and signed cast/change/remove/exact tally, separate price confirmation and explicit structured report reasons. Production DI always supplies PhotoProofTransport; its signed query canonicalization is tested against the actual request bytes. Public references only target visible VALIDATED revisions; no attribution/private media is added. Existing flag defaults ON for this free policy; explicit stored OFF remains respected. Owner/origin/revision generations, stable ambiguous-retry IDs and expected original-owner scope prevent account switches from dispatching or displaying foreign results. No entitlement gate or new moderation authority. Signed MockWebServer/use-case/ViewModel and actual API-worker/PostGIS campaign passed; remaining final source acceptance below.

## PRM20-T03 safety and recovery

LOCAL_DONE / INTEGRATION_PENDING: full existing community/moderation/feedback real-PostGIS race campaign passed, covering auth/revocation, exact reversal, hidden/deleted visibility, operator authority/privacy, replay/concurrency, projection recovery and bounded safety isolation. New actual-process TestPublicProcessCommunityActions proves signed vote→flip→own-choice tally→remove→structured dispute without GPS (9.494s); original capture/rotation campaigns remain intact. Initially adding all extra calls to the rotation campaign exhausted its existing 60-challenge budget; isolated the independent social campaign instead of increasing the limit. New negative consensus cases preserve LOW without validated media, reject missing/distinct media misuse and retain blocked/moderation gates. RED zero-clock capture regression reproduced an attempted insert; GREEN explicitly refuses before station authorization/insertion. Affected application/domain race suites passed (1.015s/1.020s). No quota or privacy gate weakened.

## PRM19-T03 durable source acceptance

LOCAL_DONE / INTEGRATION_PENDING. Existing production outbox/WorkManager/receipt suites plus new capture state tests cover atomic reviewed subsets/stable IDs, saved capture deadlines, server received versus validation, retry/offline refusal, account/origin isolation, camera cancellation/permission delay, stale query/city/target results and actual address-only selection. Unsupported old/unknown fresh policy now has a distinct localized recoverable service message, with no GPS/developer fallback. All eight locale resources compile and lint. Room 12→13 was exercised against actual host SQLite; native migration and Compose suites compile but have NOT been executed. Device evidence remains OWED.

## PRM21-T01 final source matrix — 2026-10-14

Status: LOCAL_DONE
Validation: PASS

Tested behavior revision: e2a58f25 (preceded by backend safety 6003ed56 and ASCII UUID correction 24fc6ee0); subsequent acceptance documentation does not change behavior. Base: integrated origin/dev=origin/main 68483db9. No new dependency. API/worker sources share this candidate; migrations 000091/000092 are tested locally, not deployed. Local Rust prepare_registry SHA-256: 7adf679ec353e1db13cbb8ae9fb2c3defde11f0affa611b1040ed4f02766914f. Android debug APK SHA-256: d52fb04ec883df3adbe443c481608858abebd67f1226fbf7e03656c242c1bd8c; unsigned release: cb6f60e7181e2b587e89853f4e840c47b8f879ad40e63aa7f28283a70d3b6e54. These local artifacts are not deployment/release certification.

Requirement → tasks → meaningful acceptance:

- CF01 optional coordinates/nominal address → PRM16/PRM17: Rust registry/join regressions, actual preparer→Go loader→reconciliation→anonymous HTTP, homonyms/stable UUID pagination, null-point profiles and actual SQLite address cache migration.
- CF02/CF03 no initial GPS/150m admission/trust → PRM18/PRM19/PRM20-T01: signed normal capture with no claim/address-only station; claim forgery/owner/key/station/expiry refusal; actual API→private S3→worker validation; v1 replay and v2 no-point/near/far consensus parity, independent media/support and nongeometric risks retained.
- CF04 normal nominal selection → PRM19: manual city, debounced query/generation cancellation, same-name address disambiguation, no developer flag; no GPS camera admission, changed target and late uncancellable responses refuse, camera permission/receipt deadline retained.
- CF05/CF07 free community controls and unchanged security → PRM20: real signed helpfulness/query proof, exact flip/remove/tally, original-account dispatch/late-response guards, distinct confirmation/report commands, actual-process social campaign; full existing moderator/feedback/visibility/privacy/replay/concurrency/erasure suites and safety isolation remain green. No price weighting from likes or payment.
- Legacy compatibility → PRM18/PRM19/PRM21: original v1/debug/v2 binding and immutable deadlines, unknown policy refusal/recoverable app state, append-only migration upgrade/replay/rollback. Existing quota and capacity limits unchanged.
- CF06 future proximity → PRM15/PRM21: strict >90% reviewed national coverage plus product validation and new explicit authorization; no automatic toggle or measured coverage claim. PRM22 remains DEFERRED.

Actual acceptance commands/results (disposable fixture environment described below):

- `CARGO_TARGET_DIR=/tmp/prm-community-rust bash scripts/check-rust-fast.sh`: PASS format/clippy/locked tests; no subsequent Rust changes.
- From backend with ANPFUEL_TEST_DATABASE_URL, private test-storage variables and ANPFUEL_TEST_PREPARER_PATH: `go test -race -count=1 -tags=integration ./internal/modules/directory/adapters/...`: PASS nine package summaries. New nominal prepared HTTP/regression selectors PASS separately. Full community/moderation/feedback adapters campaign: PASS eleven packages; capture migration/concurrency, signal migration rollback/replay and read/HTTP public-reference suites PASS as recorded at their task checkpoints.
- `go test -race -count=1 -tags=integration ./cmd/api -run '^TestPublicProcess(IdentityAndSignedWrites|DevelopmentPhotoPreview)$'`: PASS final community policy (24.252s). `go test -race -count=1 -tags=integration ./cmd/api -run '^TestPublicProcessCommunityActions$'`: PASS (9.494s). Existing challenge quota retained after the test-composition failure described above.
- `go test -race ./internal/modules/community/application ./internal/modules/community/domain`: PASS after zero-clock and negative consensus tests. `bash scripts/check-backend-fast.sh`: PASS final functional Go source; later changes only clarify two legacy/worker comments. Build/vet/unit/staticcheck/sqlc vet+drift/OpenAPI/secret checks pass; existing OpenAPI lint reports 329 warnings/91 informs and zero errors.
- `govulncheck ./...`: no vulnerabilities found. `bash scripts/check-mobile.sh --static-only`: PASS.
- With ANDROID_HOME and ANDROID_SDK_ROOT both /data/dev/android/sdk/Sdk: `./gradlew :domain:test :domain:jacocoTestCoverageVerification :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :app:assembleDebug :app:assembleRelease :data:assembleDebugAndroidTest :app:assembleDebugAndroidTest --console=plain`: BUILD SUCCESSFUL (2m21s; 269 tasks). Domain 537/application 353/data 366/app 324 discovered; 1,580 total, 1,578 executed, zero failures/errors and two pre-existing explicit opt-in data tests skipped. Native APK compilation is not native execution. Lint zero errors; release APK unsigned.

Coverage gate RED initially exposed 86.63% domain instructions, below the unchanged 90% requirement. Added meaningful nominal/feed/money/profile/owner-envelope negative cases, not exclusions or a reduced threshold. A new Unicode-digit UUID refusal test failed RED and demonstrated canonical identity validation accepted non-ASCII digits; fixed both station/draft UUID validators to ASCII hexadecimal. Final instruction coverage is 20,589 covered / 22,838 total = 90.1524%; actual jacocoTestCoverageVerification PASS. All domain cases reran after the fix. A first Android rerun used conflicting SDK environment paths; resolved by using the host's actual configured SDK, without project configuration changes.

Disposable PostGIS/private MinIO project: prm-community-20261013 only; synthetic fixtures/loopback credentials were outside Git. No staging/provider/device records or contributor GPS/photos introduced. Raw machine logs remain local /tmp/prm-*; versioned source tests and these exact acceptance results are the durable evidence.

## PRM21-T02 operational handoff

Concrete server/worker/schema/Android ordering, incompatibility recovery, immutable-receipt rollback and explicit synthetic staging/device matrix are [prepared here](PRM_COMMUNITY_FIRST_ROLLOUT.md). Operational execution is OWED: no staging deployment, device install, live retention/provider/capacity observation or operator proof was performed for this batch. No G09/public pilot/commercial Premium/iOS completion is inferred.

## PRM21-T03 source closure and publication handoff

Source acceptance LOCAL_DONE / INTEGRATION_PENDING. PRM15–PRM20 source tasks and PRM21-T01/T03 are complete locally. PRM21-T02 is PREPARED / OPERATIONAL_ACCEPTANCE_OWED, not falsely marked complete. PRM22 is DEFERRED. Existing Premium/OneSignal/commercial obligations remain owned by their original plan/evidence; this free-community batch does not implement or accept them.

The maintained dev checkout was occupied, so the isolated cumulative branch preserves its edits and all meaningful source commits. User authorized pushing this batch and opening its PR to main; no merge/deployment/wiki authorized here. Required remote Quick verification and current head/base review still gate integration; no CI polling/wait is part of construction. Wiki mirrors only after a separately authorized actual merge. Final document-link/task inventory, diff/secret review and current-main reconciliation are recorded by the closure commit and PR metadata.
