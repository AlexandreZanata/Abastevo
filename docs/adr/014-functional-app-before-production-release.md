# ADR-014: Functional app before real production release

Date: 2026-09-30. Status: ACCEPTED by explicit user direction; implementation remains separately gated.

## Decision

Keep backend/infrastructure integration first. **G09 is classified RELEASE and DEFERRED_UNTIL_APP_FUNCTIONAL**. Real deployment, immutable-candidate production certification and public pilot happen only after G18 functional Android/iOS acceptance. Local backend validation plus verified integration is **G09-LOCAL**, the entry prerequisite for app development. It is not RELEASE_CERTIFIED. Free accounts and feedback/media/location backend extensions are completed and tested before their app consumers.

This supersedes the production-before-any-mobile restriction in ADR-011/ADR-013, AGENTS and the original roadmap. Existing phase/task IDs and historical evidence remain intact. New phases P12–P18 follow dependency order rather than numeric order: G09-LOCAL → P12 → P13 → P14 → P15 → P16 → P10 local integration → P17 → P18 → P09/G09 production release → P10-T09 public pilot → P11 optional hosted benefits. P11-T02 account linking moves to P13; registration is free, independent of billing.

## Safety and delivery

Immediate critical negative/concurrency/PostGIS/privacy tests, one bounded task/atomic commit/issue, one phase branch/draft PR, quick local/current-head remote checks and guarded merge remain mandatory. Full production evidence is deferred, not waived. Each functional phase has scoped acceptance; G18 validates the integrated app against local synthetic services. macOS/Xcode and real iOS device evidence are mandatory for iOS acceptance; Linux-only results cannot satisfy it.

A phase merge does not publish a stable tag, GitHub Release, deploy a server or certify real operations. Production checks must cover the eventual accounts, social content, 24-hour media policy and location controls, not just the earlier backend. Failed or missing required checks block integration; failed release evidence blocks real launch. Wiki mirrors only merged snapshots, preserving manual pages.

## Consequence

The team can build the app using the tested local backend without paying for real infrastructure early. Real provider compatibility, legal notice review, off-host restore, accepted capacity and launch operations remain open release work. A local photo/storage emulator does not prove real-provider retention. See [delivery plan](../planning/MOBILE_DELIVERY_PLAN.md) and [G09 record](../release-evidence/G09.md).
