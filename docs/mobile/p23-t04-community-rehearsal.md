# P23-T04 — Local community rehearsal and business hypotheses

Status: LOCAL_DONE on `codex/phase-23-community-operations`. Issue: #94. Entry P23-T03 satisfied (metrics/operating model LOCAL_DONE). Binds B-BR-C01–C06 and BUC-C01–C05. No migration, no new endpoint, no new permission, no iOS work, no public pilot.

## What this task adds

- `docs/operator/pilot-city.md`: bounded-city opening checklist (frozen scope, named moderation coverage, support route, privacy ops, low-coverage verification), stop/rollback triggers, and the owned unresolved-risk list.
- This rehearsal: one synthetic lifecycle walked end to end against the current tree — every step below ran now on synthetic fixtures only (no real user data, no simulated members).

## Rehearsal — synthetic city day (all suites green on this tree)

- Onboarding + rules: `OnboardingViewModelTest` (4-page walk), Help/rules copy (`RoutesTest`, assemble).
- Contribution: outbox enqueue/replay/status suites (`:data`, `:application`, `:app` contribution tests).
- Correction: comment edit/revision suites (`feedback/application`, HTTP edit flow).
- Report → review → act → appeal: `-race` real-PostGIS `feedback` + `moderation` suites (report/flag/hide/leak, quota, converge, appeals-as-new-action).
- Follow/notification: local opt-in price-drop alerts (`EvaluatePriceDropAlertsUseCaseTest` incl. dedupe); no follows/subscriptions exist by decision (P23-T01).
- Erase/export: footprint + `pgerase` real-PostGIS suites (tombstone, rebuild, concurrent erase, G14 exit).
- No-coverage recovery: `StationDetailRuleTest` UNKNOWN states + dated ANP reference.

## Results on this tree

- Backend `-race -tags=integration` feedback (4 pkgs) + moderation (3 pkgs) vs real PostGIS: 0 failures.
- `:domain:test` 408 + `:application:test` 248 + `:data:testDebugUnitTest` 207 (1 pre-existing live-network skip) + `:app:testDebugUnitTest` 147, 0-fail (`--rerun-tasks`); `:app:assembleDebug` PASS.
- `git diff --check` clean; scoped secret review (synthetic fixtures only).

## Business hypotheses (evaluated, none implemented — P11 owns paid benefits)

- Return alerts and local vehicles: kept free; value hypothesis testable at pilot via opt-in participation (metric DEFINED, device-local).
- Hosted backup/recovery/cross-device sync, advanced stats: plausible paid surface per `OPEN_SOURCE_BUSINESS.md`; willingness unmeasured — no paywall, no prices invented.
- Fleet/merchant API: demand, licensing, privacy, cost and abuse unvalidated — explicitly not built.
- Guardrails restated: payment never buys trust, immunity, confidence tier or placement; provider-verified entitlement only.

## Validation

- Suites above all PASS on the committed tree; runbook links resolve (`operator/pilot-city.md`, `COMMUNITY_METRICS.md`, `COMMUNITY_MODERATION.md`, `OPEN_SOURCE_BUSINESS.md`).
- `git diff --check` clean; secret scan PASS. Phase P23 ready for exit / PR merge.
