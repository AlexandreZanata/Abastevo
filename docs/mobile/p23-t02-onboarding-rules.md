# P23-T02 — Contributor onboarding and community rules

Status: LOCAL_DONE on `codex/phase-23-community-operations`. Issue: #92. Entry P23-T01 satisfied (duplicate-alert suppression LOCAL_DONE). Binds B-BR-C01–C06 and BUC-C01–C05. No migration, no backend change, no new permission, no iOS work.

## What this task adds

- Onboarding 4th intro page (contributor guidance, before the week-choice page): photograph panels only from safe public spots, keep totals legible, pick the exact condition, never invent prices or people. `PAGE_COUNT` 3→4, explicit page mapping, en + pt-BR.
- Help screen (`Routes.HELP`, reached from the Perfil expert-tools list): four static sections — contribute-well, community rules, report/appeal route, privacy — grounded in `docs/product/COMMUNITY_MODERATION.md`. No ViewModel, no network, no account needed; accessible headings; en + pt-BR.
- Tests: intro pager walks 4 steps ending on week choice (clamp + back), `Routes.HELP` constant.

## Deliberately not built

- Invite flow: not implemented — no measured need, and invites risk identity exposure/spam. If ever proposed: opt-in only, no contact upload, rate-limited, abuse-audited. Until then, invite code must not exist.
- No fake members/activity anywhere; Help content is static policy text, never simulated community.

## Validation

- `:app:testDebugUnitTest` 147/0-fail (`--rerun-tasks`, +2); `:app:assembleDebug` PASS (route + resources link).
- `git diff --check` clean; scoped secret review (static copy only, no PII/GPS/secrets).
