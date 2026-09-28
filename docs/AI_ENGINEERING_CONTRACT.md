# AI engineering contract

Applies to future implementation, with scope/authorization from the active user request. Start at AGENTS.md; do not reload the entire repository for each task.

## Sources of truth

The user request governs work scope. Product contract owns product behavior; DOMAIN_MODEL and COMMUNITY_PRICING_SPEC own backend rules; BUC specs own orchestration; ADRs own durable technical decisions; OpenAPI owns wire syntax once introduced; SQL migrations own DB schema; tests demonstrate contracts; implementation must conform. Existing Android docs own legacy behavior until explicitly reconciled. These are complementary responsibilities, not permission to overwrite code because one document is stale.

If two sources conflict, record the issue and fix the owning specification/test before implementing. Known conflicts and their planned resolution are in CURRENT_STATE_AUDIT. Do not infer acceptance of an unimplemented design from an “Accepted” historical Android ADR.

## Per-task workflow

1. Read task, inputs, relevant implementation/tests and git status/diff. Check dependencies and phase gate; record current branch without switching away from user work.
2. State intended file areas and B-BR/BUC. Resolve bounded assumptions locally, recording their rationale; ask only when a required product/authorization decision cannot be inferred.
3. For behavior, write failing meaningful test, confirm correct failure, implement smallest change, refactor inside scope.
4. Run focused tests, relevant compile/static/migration/contract checks; inspect own diff including newly created files; scan sensitive changes and run `git diff --check`.
5. Update progress with exact outcome, evidence and next task. Failures/blockers are not completion. Keep one short current-state file; link longer release evidence rather than accumulating transcripts.

## Guardrails

- Do not rewrite Android, move its modules or rename packages/application ID for branding. Do not improve Android before G09.
- Do not change rules, invent endpoints/fields, silently round money, suppress unknown ANP fields or convert provider failure into a fabricated price.
- Domain uses deterministic clock/ID inputs, no I/O/frameworks. Domain unit tests are pure; DB integration tests use real PostGIS.
- No disabled/deleted failing tests, meaningless mocks or reduced validation to pass CI. Regression fix includes a test.
- One task/PR = one coherent purpose; split a task if it spans unrelated concepts. Warn on unusually broad source diffs; documentation/import size is not a license for unrelated edits.
- No framework/broker/cache/database addition without a concrete need, alternatives, maintenance/license/security/build/lock-in review and ADR when architectural.
- No edits to applied migrations; no manual production schema patches. Expand/contract and recovery tests for schema changes.
- No broad exception swallowing, silent fallback, input-concatenated SQL or internal error exposure.
- No production personal data in fixtures, secrets in logs, or release signing material in Git. Read only the config values needed for the task and redact outputs.
- No custom cryptographic primitive, paid trust boost, public evidence bucket or infinite retry loop.

## Definition of done (DOD-1)

Selected task only; B-BR/BUC/spec updated when needed; meaningful tests pass and changed behavior compiled; focused static/security/contract/migration checks pass; recovery explained; no unrelated diff, critical TODO, secret or unsupported claim; progress updated with commands and remaining limits. Documentation-only tasks need document checks, not invented production tests. A runtime release gate cannot be satisfied by writing its checklist.

Future PR description: concrete problem/result, B-BR/BUC, implementation scope, validation/evidence, migration and privacy impact, rollback. English Conventional Commits when commits are requested; never commit merely because a task mentions a branch.
