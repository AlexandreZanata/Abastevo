# P23-T03 — Aggregate metrics and pilot operating model

Status: LOCAL_DONE on `codex/phase-23-community-operations`. Issue: #93. Entry P23-T02 satisfied (onboarding/rules LOCAL_DONE). Binds B-BR-C01–C06 and BUC-C01–C05. No migration, no backend change, no new permission, no iOS work.

## What this task adds

- `docs/product/COMMUNITY_METRICS.md`: eight frozen aggregate definitions (purpose, denominator/window, minimum aggregation, retention/deletion, collection status), low-coverage honesty policy, per-city staffing checklist, costs from measured utilization only. Definitions freeze before instrumentation: nothing new is collected.
- Verified posture: zero Android analytics SDKs (dependency scan), backend label policy with pinned no-ID test, consent points already opt-in (session, OS grants).

## Deliberately not built

- No new instrumentation, dashboards or tracking: DEFINED rows stay uncollected until a pilot plan names their source; COLLECTED rows reuse existing infra/case/queue sources only.
- No budgets, headcounts or coverage promises invented; pilot numbers attach in the pilot plan (P23-T04 rehearsal owns stop/rollback).

## Validation (docs-only slice: links/state checks)

- Referenced runbooks/policies resolve (`operator/feedback-moderation.md`, `operator/monitoring.md`, `product/COMMUNITY_MODERATION.md`); metric statuses match actual code state (no analytics deps; label test exists; cases/queue CLI exist).
- `git diff --check` clean; scoped secret review (definitions only, no PII/IDs/budgets).
