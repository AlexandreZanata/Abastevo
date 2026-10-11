# CS15-A — Offline capability inventory (planning only)

Date: 2026-10-11. Status: PLANNING — coverage inventory, not an
assertion that any row has passed. Unpublished by policy
(AGENTS.md: planning-only work does not publish); rides a future
code delivery instead of its own push.

Method: every row verified by listing the owning path
(`Routes.kt`, `application/usecase/*`, `backend/internal/modules/*`,
UI dirs) on maintained `dev`. Nothing invented: surfaces without
an owning path are recorded as absent, and offline outcomes are
left to CS15-B (no verdicts frozen here).

Legend: Owner = verified code path. Scenario = offline-exercise
ID reserved for CS15-C manifests.

## A — Navigation (app/.../navigation/Routes.kt, all present)

| # | Route | Owner UI area | Scenario |
|---|-------|---------------|----------|
| A01 | onboarding | ui/onboarding | OFF-NAV-01 |
| A02 | auth | ui/auth | OFF-NAV-02 |
| A03 | account | ui/account | OFF-NAV-03 |
| A04 | home (= explore) | ui/home | OFF-NAV-04 |
| A05 | community | ui/community | OFF-NAV-05 |
| A06 | profile | ui/profile | OFF-NAV-06 |
| A07 | search | ui/search | OFF-NAV-07 |
| A08 | location | ui/location | OFF-NAV-08 |
| A09 | prices | ui/prices | OFF-NAV-09 |
| A10 | history | ui/history | OFF-NAV-10 |
| A11 | stations, stations/{fuelProduct} | ui/stations | OFF-NAV-11 |
| A12 | settings | ui/settings | OFF-NAV-12 |
| A13 | help | ui/help | OFF-NAV-13 |
| A14 | vehicles | ui/vehicle | OFF-NAV-14 |
| A15 | week_picker | ui/weekpicker | OFF-NAV-15 |
| A16 | capture, capture?stationId&fuel | app/capture (CaptureScreen) | OFF-NAV-16 |
| A17 | contributions | route in Routes.kt; hosting screen unconfirmed (verify in CS15-B) | OFF-NAV-17 |
| A18 | station-claim/{stationId} | ui/stations | OFF-NAV-18 |
| A19 | station-management/{stationId} | ui/stations | OFF-NAV-19 |
| A20 | station/{stationKey}, station-profile/{stationId} | ui/stationprofile | OFF-NAV-20 |
| A21 | suggest | ui/suggest | OFF-NAV-21 |

## B — Use cases (application/usecase/*, all files listed)

| # | Surface | Owner files | Scenario |
|---|---------|-------------|----------|
| B01 | Price alerts | alert/ConfigurePriceDropAlertUseCase, EvaluatePriceDropAlertsUseCase | OFF-UC-01 |
| B02 | Capture confirm | capture/ConfirmPriceCaptureUseCase | OFF-UC-02 |
| B03 | Community feed/groups/votes/helpfulness | community/ (4 files incl. SubmitHelpfulnessVoteUseCase) | OFF-UC-03 |
| B04 | Contribution enqueue/dispatch/owned | contribution/ (3 files; outbox queue exists) | OFF-UC-04 |
| B05 | Station directory + CNPJ resolve | directory/ (2 files) | OFF-UC-05 |
| B06 | Feedback page/submit | feedback/ (2 files) | OFF-UC-06 |
| B07 | Anonymous device flow | identity/AnonymousDeviceFlow | OFF-UC-07 |
| B08 | Station intake | intake/StationIntakeUseCases | OFF-UC-08 |
| B09 | Location resolve/search/select | location/ (3 files) | OFF-UC-09 |
| B10 | Start destination | navigation/ResolveAppStartDestinationUseCase | OFF-UC-10 |
| B11 | Connectivity observe | network/ObserveNetworkConnectivityUseCase | OFF-UC-11 |
| B12 | Onboarding/location-prompt/week-sync | onboarding/ (3 files) | OFF-UC-12 |
| B13 | Municipality/station prices + history | price/ (3 files) | OFF-UC-13 |
| B14 | Training consent grant/revoke/status | privacy/TrainingConsentUseCase (CS11A; offline-safe) | OFF-UC-14 |
| B15 | Profile claim/export/managed actions | profile/ (4 files) | OFF-UC-15 |
| B16 | Data readiness | readiness/GetDataReadinessUseCase | OFF-UC-16 |
| B17 | Settings/preferences/cache/storage/retention | settings/ (5 files) | OFF-UC-17 |
| B18 | Station navigation query + nearest price | station/ (2 files) | OFF-UC-18 |
| B19 | Week catalog/download/select/sync tables | sync/ (6 files; cache-first sync) | OFF-UC-19 |
| B20 | Vehicle CRUD + tank estimates | vehicle/ (5 files) | OFF-UC-20 |

## C — Backend modules (backend/internal/modules/*) + ANP import

account, community, directory, evidence, feedback, identity,
kernel, moderation, official, privacy, stationprofile, trust
(12 dirs, all present). Imported ANP tools: `official` module
(`adapters/anp`, `importer.go`, jobs) — ANP price-table import
and survey-week sync. Payment surfaces: none exist in
app/data/domain/application (grep for billing/purchase empty) —
recorded absent, not deferred.

## D — Settings / consent / privacy UX state (verified)

- Training-consent panel + per-photo opt-in row exist as
  components (CS11B/C) but are NOT placed: no reference in
  `ui/settings/SettingsScreen.kt` or `app/capture/CaptureScreen.kt`
  (grep verified). Scenario OFF-UX-01 (placement) is CS11-screen work.
- Helpfulness panel likewise unplaced (CS08B limit, unchanged).
- Server consent endpoints live with signed/guest/revoked-key/
  tampered/replay/terms coverage (CS12A). Scenario OFF-UX-02.

## Explicitly NOT claimed

No offline outcome frozen (CS15-B owns permit/queue/conflict
verdicts, cache/queue limits, stale display, prerequisites).
No scenario manifests, seeds, devices, ceilings or budgets
(CS15-C owns them). No exit: this file is the reviewable
starting point CS15 requires, nothing more.
