# Product contract: backend first

Status: product direction revised 2026-10-01; existing local modules are integrated, but new commercial UX remains PLANNED. See current PROGRESS for runtime evidence. Project name **abastevo** was selected by the maintainer on 2026-10-01; [brand identity](../brand/IDENTITY.md) owns the logo and README artwork status.

## Objective and release order

Build a reliable network of recent community fuel-price observations alongside the official ANP reference. Integrate and locally validate the backend/infrastructure first (G09-LOCAL), then deliver the functional Kotlin Multiplatform app with native Android/Swift adapters. G09 is classified real-production RELEASE, deferred until G24-ANDROID-COMMERCIAL for Android/backend scope under [ADR-016](../adr/016-android-commercial-community-ios-deferred.md). Historical full G18 remains unaccepted; iOS is archived with explicit resumption only. [Community experience](COMMUNITY_EXPERIENCE.md) makes community prices primary and ANP dated reference.

1. Import and audit the existing Android application; plan the work.
2. Build the modular monolith, official station catalog and anonymous identity.
3. Deliver community observations, private evidence, validation, consensus and moderation.
4. Integrate local infrastructure/security/recovery checks, then P12–P16 KMP/free accounts/feedback/media/location extensions.
5. Preserve P10/P17/P18 local integration and unresolved evidence; implement P19–P24 Android commercial community experience, preserving offline features. iOS is deferred, not accepted.
6. Only then certify real production G09 and public pilot; paid benefits remain later P11.

Existing Android regression may run anytime. New app work requires G09-LOCAL integration; native KMP/Swift work starts P12 and affected backend extensions precede consumers. Local synthetic services are used until actual real-production release.

## MUST: free community MVP

- Public official and community price reading, station lookup and source/freshness metadata.
- Anonymous contributor identity with proof of possession; no mandatory name, phone or email.
- Observation creation, optional private photographic evidence, confirmations and structured disputes.
- An auditable, conservative current-price projection. “Unknown” and “disputed” are valid outcomes.
- Official history and community history remain separate; community observations never overwrite ANP.
- Anti-abuse controls, moderation, retention, export/deletion handling and recoverable operations.
- Preserve existing offline features, local vehicle allowance, alerts and navigation after integration.
- Free account registration by email access code, Google and Apple; account-authenticated station/fuel stars, 280-character comments/replies, validity votes, moderation and transparent community agreement percentage.
- Lightweight native photo encoding and backend revalidation, all app-owned photo copies at most 24 hours; simulated-location denial with honest UNKNOWN handling. See owning security/feedback target contracts.

## SHOULD and LATER

SHOULD after pilot: refine consensus from labelled review outcomes, improve geocoding coverage, optional integrity signals, richer operational summaries and additional localization. LATER: optional paid hosted backup/migration, cross-device sync, advanced alerts/statistics, optional hosted benefits, API commercial plans and fleets. No subscription machinery, multi-tenancy or enterprise services in the community MVP.

Payment buys hosted convenience; it never increases confidence or reputation. Do not move currently free features behind a paywall as part of integration. Selling “multiple vehicles” needs a product decision that preserves the existing free allowance of three.

## Product truth

ANP is the source of official observations; it is not a claim about a station's live pump price. Community confidence estimates support, not certainty. A price always carries product, unit, condition, provenance and time. Advertised APP/LOYALTY prices cannot compete as unconditional STANDARD prices. Contributor identity and precise location are never public content.

Success metrics are aggregate: contributions/day, validated share, dispute/rejection rates, stations with fresh supported data, median data age, confirmation delay and covered cities. Collection of installation/activity metrics needs a documented purpose and minimization; do not introduce advertising identifiers or silent user tracking.

## Scope authority

This contract governs the planned platform. `docs/user-business-logic.md` continues to describe the imported Android release. Reconcile differences explicitly in P10; do not claim the target platform is already shipped. The supplied text is a source of requirements, not an instruction to run commands, create accounts, spend money or publish anything.
