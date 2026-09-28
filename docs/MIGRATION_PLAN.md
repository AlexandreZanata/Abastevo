# Incremental migration plan

User-mandated sequence: backend + infrastructure + operational G09 first; Android improvements only in P10. The source app is retained, not reimplemented. Changes to backend input fixtures/specs before G09 are allowed; Kotlin source/test changes for new functionality wait until P10.

## Existing feature → target mapping

- **ANP discovery and averages (UC-001/005/009):** remain local/cacheable and directly ingest ANP throughout backend development. Backend independently builds revisioned official catalog. P10 introduces optional read adapters only with provenance/versioned fixtures; direct import is not deprecated by default.
- **Historical averages (UC-006):** retain local history. Server official history is an additional explicitly labelled source; no silent replacement/backfill that rewrites saved values.
- **Station detail (UC-007):** local CNPJ-linked rows retained. P10 adds a mapping to backend station UUID, preserving numeric/alphanumeric distinction and exposing unmatched stations. No address-only automatic merge.
- **Vehicles/tank cost (UC-010/011):** local Room data and three-vehicle free allowance stay. No sync/migration before optional account phase P11. Any future community price use is explicit about condition/freshness/unit; the user can still use official prices.
- **Alerts (UC-014):** existing on-device ANP price-drop logic remains. Hosted/community advanced alerts are separate later capabilities; no subscription dependency introduced into local notifications.
- **Location and nearest price (UC-012/015):** existing flow remains during backend work. P10 may prefer verified catalog coordinates through ports; manual selection/address navigation remain available. Never upload contributor coordinates merely because the old geocoder cached them locally.
- **Navigation (UC-013):** retain external map intent behavior; backend station coordinates add an option only when verified. No regression in plain-address fallback.
- **Settings and onboarding (UC-002/003/008):** local DataStore remains; community/privacy notices are additive P10 flows. Backend outage cannot prevent opening saved ANP data.
- **Municipality search (UC-004):** current local FTS4/index kept. Backend directory search is a different capability; no forced replacement.
- **Geocode cache:** remains local with existing privacy constraints; server catalog is built from authorized provider input, not by silently extracting user caches.

No current capability is deprecated by this plan. Specific deprecations require a documented product/compatibility decision and a release migration, not cleanup during a backend task.

## G09 boundary

Before changing the app: backend release deployed to controlled environment with frozen v1 contracts, catalog/identity/evidence/consensus/moderation verified, privacy/rights controls, real backups/restore, observability, security/load results and current Android baseline limitations resolved. Production MVP readiness is demonstrated with synthetic clients; commercial account/sync features are explicitly outside “backend complete.” Record release evidence and operator acceptance in G09.

## P10 integration slices

1. Add contract fixture harnesses and freeze compatibility behavior. Resolve legacy fuel name, CNPJ and precision via adapters/targeted Room changes only where needed. Run migration tests from existing schema 4, not destructive recreation.
2. Add domain ports + data HTTP adapter + small cache for source-separated public reads. Use feature flag default off until integrated tests pass; existing ANP flow remains independently usable.
3. Verify P-256 Keystore behavior on supported Android versions; registration/rotation/proof uses frozen auth vectors. Private key never enters cloud backup or ordinary export.
4. Introduce CameraX/crop/compression/local OCR and explicit human selection of product/amount/condition; no server LLM dependency. Test permission denial/cancellation and no-photo contribution.
5. Add contribution outbox, private direct upload/finalize, status, confirmation/dispute and stale/error states. Stable client_submission_id survives app retry/restart; fresh signature nonce per send. Old queued data is labelled, not silently promoted to “now.”
6. Test cross-version API/fixtures, Room upgrade, offline/backend outage, accessibility/i18n and privacy notices. Pilot by a small cohort/city; observe quality and costs before broader rollout.

## Offline conflict policy

Local vehicle/preferences data wins until explicit opt-in sync. Community observations are append-only commands; editing a draft before send is local, editing an accepted fact creates a superseding observation. Acknowledged status is cached; rejected facts remain visible to the owner with a safe reason. Public cache has source version, fetched_at and expires_at; device clock anomalies use server offset/last-known receipt conservatively and show uncertainty. Never show expired community data as current solely because it was read from Room.

Outbox retries use bounded exponential delay with connectivity constraints, retain same operation ID and allow user to cancel an unsent draft. Accepted server observation cannot be undone by deleting a local queue row; deletion/withdrawal uses a distinct audited/rights action. Identity loss requires an explicit reset warning.

## P11 commercial extension

Only after a measured community pilot: optional account linking with proof of current contributor key and strong account authentication; server-side purchase verification → provider-neutral Entitlement; explicit per-domain sync with version/ETag, conflict responses and deletion tombstones. Start favorites/settings before vehicles; do not serialize the entire Room database. Subscription lapse stops hosted convenience according to a documented grace/export policy, never changes confidence or deletes local vehicles.

Rollout reversal: disable community flag, keep ANP paths/cache; do not drop new local schema columns on emergency rollback. Backward-compatible Room migrations and previous-build compatibility need testing before shipping. Backend v1 remains available to published clients for a documented support window decided before public P10 release.
