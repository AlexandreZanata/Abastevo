# P22-T03 — Report/moderation status and support journey

Status: LOCAL_DONE on `codex/phase-22-social-moderation`. Issue: #88. Entry P22-T02 satisfied (ratings transport LOCAL_DONE). Binds B-BR-F07, BUC-C03/C05 and B-BR-C05 to existing controls. No migration, no new endpoint, no new permission, no iOS work.

## What this task adds

- `docs/product/COMMUNITY_MODERATION.md`: user-facing moderation policy — reporting rules/quota, restricted-operator review authority (no public admin path), plain-language visibility states, no-auto-delete guarantee, appeal-as-new-case route with honest pilot support scope, pilot staffing rule (no city opens without recorded coverage), and the frozen block/mute contract: **no user-level mute/block in v1**; blocks stay operator-only contributor blocks via the trust tier. Any future mute must freeze scope/rights/deletion/API here before code exists.
- `CommunityScreen` report & moderation card (en + pt-BR): any active account may report, reports never delete content alone, restricted moderators review with recorded reasons, appeals reopen as a new reviewed case. Accessible headings, no new navigation, no invented contact channel.
- Report-journey tests locking existing behavior: `Reported` without touching content (no delete calls), quota → `QUOTA`, blank reason → local `INVALID` without IO.

## Reused and verified (not rebuilt)

Report transport/quota-converge, moderation cases/actions/audit, `ops moderation act/invalidate/block` + `ops feedback hide/show/export/erase`, trust-tier contributor blocks, 12-month audit retention — all P07/P14-owned and covered below.

## Operator gate (real PostGIS, `-race`)

- Backend `moderation` + `feedback` + `cmd/ops` suites, unit and `-tags=integration` vs local PostGIS: all packages ok — report→case→act→hide/show, appeal-as-new-action, reporter-deletion survival, export/erase determinism.

## Explicitly deferred (not waived)

- Reporter case-status and author-history reads: need new backend endpoints plus privacy review before any consumer — future backend extension, not silently dropped.
- In-screen report buttons on discussion rows: needs the station-identity mapping (same blocker as the T02 discussion wiring).
- In-app help/support channel surface: P23-T02 owns onboarding/rules/help channels; the policy states the re-report route honestly until then.
- Real provider proof: P24-owned (per P22-T01).

## Validation

- `FeedbackViewModelTest` 16/0-fail (3 new report tests).
- Backend moderation/feedback/ops unit + real-PostGIS integration, 0 failures.
- `./gradlew :app:testDebugUnitTest :app:assembleDebug` (affected suites).
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).
