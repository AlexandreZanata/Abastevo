# Functional multiplatform delivery plan

Status: PLANNED, 2026-09-30. This delivery defines functionality and testable boundaries; it does not implement new app/account/feedback/media behavior. [ADR-014](../adr/014-functional-app-before-production-release.md) changes the release order; [ADR-015](../adr/015-kotlin-multiplatform-shared-domain.md) defines Kotlin/Swift boundaries. Authoritative microtasks and acceptance are in [ROADMAP](../../ROADMAP.md).

## Entry and sequence

Integrate the already locally tested backend corrections with the required current-head Quick verification to reach G09-LOCAL. G09 remains a real-production **RELEASE**, deferred until functional app acceptance. No server purchase, public deployment, stable tag or release publication is part of this plan.

1. **P12 / G12:** audit toolchains and existing features; extract shared pure Kotlin domain/application logic; Android parity; Swift framework/shell compile and native adapters on macOS.
2. **P13 / G13:** free accounts via email code, Google and Apple, secure linking/recovery and rights. Backend first, then shared/native login consumers. No billing gate.
3. **P14 / G14:** backend station/fuel ratings, 280-character comments/replies, moderation and validity-vote percentage. Contracts before schema/domain/transport; real DB concurrency and negative auth tests immediately.
4. **P15 / G15:** local lightweight encoding and backend sanitization; all photo copies expire within 24 hours; worker/storage/cache deletion and restore proof. Freeze measured KiB/pixel/memory budgets before wire changes.
5. **P16 / G16:** native mock/simulation detection and backend risk/proximity controls, shared denied/unknown behavior and adversarial device tests.
6. **P10 / G10-LOCAL:** complete existing contract/signature/offline/outbox/community features with the new prerequisites. Preserve existing design; no visual redesign. P10-T09 public pilot stays deferred until G09.
7. **P17 / G17:** social features and full Android/iOS parity; real Swift callbacks/camera/location/Keychain/storage integration and lifecycle resilience.
8. **P18 / G18:** integrated functional acceptance against local synthetic services, real devices, security/accessibility basics and performance profiling; freeze support matrix and app-functional candidate.
9. **P09 / G09:** only now select the immutable production candidate and complete real edge/storage/restore/load/privacy/operator certification. It remains deferred until prerequisites and actual evidence pass.
10. **P10-T09:** limited public pilot after certified release. **P11:** separately validated optional hosted paid benefits; free accounts and social participation remain free.

Numeric IDs preserve history; this explicit dependency graph controls order. Each gate requires actual implementation/evidence, not a merged plan. Oversized tasks are split into letter-suffixed coherent slices before work, never into an unbounded phase PR.

## Feature ownership and preservation

P12 inventories every imported feature and existing test/use case; P10 retains official weekly imports, source distinctions, search/filter/sort, comparisons/calculations, three free local vehicles, favorites, settings, navigation, local alerts and offline behavior. The existing [migration matrix](../MIGRATION_PLAN.md) and docs/user-business-logic.md define parity; no feature is silently removed to speed KMP migration.

P13 owns [account rules](../security/FREE_ACCOUNT_ACCESS.md); P14/P17 own [feedback rules](../product/STATION_FUEL_FEEDBACK.md); P15/P16 own [media/location policy](../security/LOCAL_MEDIA_LOCATION_POLICY.md). Public reads and anonymous price identity remain distinct from authenticated social writes. Price consensus, personal star ratings and community comment agreement are separate projections. Payment does not affect any trust calculation.

Modules retain pure domain → application/ports → adapters → presentation. New contexts are account access, station/fuel feedback, media processing and location integrity; avoid speculative services or a module per UI component. Kotlin/Go/Swift fixtures freeze exact money, Unicode length, signatures, dates, offline revisions and error semantics. Current backend v1 compatibility is preserved until each additive contract is implemented/tested.

## Delivery and tests

One task issue/atomic commit; one phase milestone, codex/phase-NN-slug branch and draft PR. Open only the current phase's issues, preserve merged task history, run focused TDD RED/GREEN/REFACTOR and critical failure/concurrency/real-PostGIS checks as changes occur. Phase exit runs specialized checks once, then quick locally and required Quick verification on verified current head/base. Guarded merge preserves commits; wiki mirrors the merged phase once. A mirror failure is WIKI_PENDING, not another application test run.

No complete backend/infra suite after every UI slice. Changed backend auth/privacy/SQL needs immediate appropriate tests. Native modules need their affected compilation/device tests immediately; Linux cannot certify iOS. G18 is an integrated functional/device/performance campaign; G09 is the separate full real-production campaign. Record exact commands/environment/candidate and blockers. New commands named descriptively in tasks must be introduced and proven before claiming completion.

## Exit evidence and limits

Freeze test fixtures and feature checklist; verify supported Android and iPhone devices, native background/offline behavior, account recovery, social authorization/moderation, retry/concurrency, 24-hour photo access and physical deletion, mock-location denials and money/source correctness. Profile memory/time/battery using realistic devices and legible photo fixtures. No premium enrollment requirement, no PII/precise GPS/photo logs. Functional screens use current styling; a later design phase is explicitly outside this delivery.

Outstanding provider/OIDC/device/macOS/legal/real-storage evidence is tracked as blocked acceptance, never green based on mocks. Photo size/memory targets and reply-depth default are proposals to freeze in owning tasks; **280 characters and the three signup methods are confirmed requirements**.
