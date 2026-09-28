# Product contract: backend first

Status: planning baseline, 2026-09-28. Product name “Postô” is provisional.

## Objective and release order

Build a reliable network of recent community fuel-price observations alongside the official ANP reference. Complete the backend and its production infrastructure, including operational acceptance, before changing or improving the Android app. “Complete” means the bounded MVP release gate G09, not implementing every possible future commercial feature.

1. Import and audit the existing Android application; plan the work.
2. Build the modular monolith, official station catalog and anonymous identity.
3. Deliver community observations, private evidence, validation, consensus and moderation.
4. Complete deployment, backups/restoration, privacy, monitoring, compatibility and load gates.
5. Only then integrate Android and improve contribution/offline experiences.
6. Validate the community pilot before optional accounts, paid sync and billing.

Existing Android builds and tests may run before G09. Production source changes, new screens, CameraX/ML Kit, backend client integration and local schema upgrades begin in P10. Backend tests use synthetic protocol clients until then.

## MUST: free community MVP

- Public official and community price reading, station lookup and source/freshness metadata.
- Anonymous contributor identity with proof of possession; no mandatory name, phone or email.
- Observation creation, optional private photographic evidence, confirmations and structured disputes.
- An auditable, conservative current-price projection. “Unknown” and “disputed” are valid outcomes.
- Official history and community history remain separate; community observations never overwrite ANP.
- Anti-abuse controls, moderation, retention, export/deletion handling and recoverable operations.
- Preserve existing offline features, local vehicle allowance, alerts and navigation after integration.

## SHOULD and LATER

SHOULD after pilot: refine consensus from labelled review outcomes, improve geocoding coverage, optional integrity signals, richer operational summaries and additional localization. LATER: optional accounts, hosted backup/migration, cross-device sync, advanced alerts/statistics, Postô+, API commercial plans and fleets. No subscription machinery, multi-tenancy or enterprise services in the community MVP.

Payment buys hosted convenience; it never increases confidence or reputation. Do not move currently free features behind a paywall as part of integration. Selling “multiple vehicles” needs a product decision that preserves the existing free allowance of three.

## Product truth

ANP is the source of official observations; it is not a claim about a station's live pump price. Community confidence estimates support, not certainty. A price always carries product, unit, condition, provenance and time. Advertised APP/LOYALTY prices cannot compete as unconditional STANDARD prices. Contributor identity and precise location are never public content.

Success metrics are aggregate: contributions/day, validated share, dispute/rejection rates, stations with fresh supported data, median data age, confirmation delay and covered cities. Collection of installation/activity metrics needs a documented purpose and minimization; do not introduce advertising identifiers or silent user tracking.

## Scope authority

This contract governs the planned platform. `docs/user-business-logic.md` continues to describe the imported Android release. Reconcile differences explicitly in P10; do not claim the target platform is already shipped. The supplied text is a source of requirements, not an instruction to run commands, create accounts, spend money or publish anything.
