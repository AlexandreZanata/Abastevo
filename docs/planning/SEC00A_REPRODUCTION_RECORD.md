# SEC00-A — Source reconcile and probe reproduction (planning only)

Date: 2026-10-11. Status: PLANNING — reproduction record, not a
fix. Unpublished by policy (planning-only work does not publish).
Probes ran in an isolated worktree at current dev (since removed);
the occupied checkout was never touched.

## Reconciled state

- Maintained `dev` at `9474af8` (3 ahead of origin/dev: CS12A
  test commit plus 2 unpublished planning commits CS15-A/B1).
  PR129 OPEN at `a5674aa` (code only; planning commits withheld).
- Concurrent changes preserved untouched: 1 modified
  (`AI_TRAINING_IMAGE_CONSENT_PLAN.md` overlay) + 5 untracked
  planning/product/audit files from other efforts.
- 24 commits landed since the audit pin
  (`954a8d8`, 2026-10-11): CS00–CS12A community-safety batch
  (helpfulness, moderation, consent, training, erasure, settings
  client, live-stack tests) plus the two planning commits above.
  No behavior fix from the remediation plan has landed yet.

## Reproduction against current dev (all still reproduce)

Unit probes (`-race`, isolated worktree, `TestAudit*`):

- Distinct accounts/mail without aggregate quota — PASS (reproduces).
- Invalid refresh revokes known family — PASS (reproduces).
- JWKS cross-provider TTL extension — PASS (reproduces).
- Non-atomic daily reserve budget (21 vs 10) — PASS (reproduces).
- Failed original deletion absent from overdue audit — PASS (reproduces).
- MaxBytes not bound in PUT signature — PASS (reproduces).
- Forwarded clients share proxy quota — PASS (reproduces).
- Fabricated location passes geometry gate — PASS (reproduces).
- Retention corrected case — PASS (reproduces).

Real-SQL probes (unique disposable PostGIS project
`abastevo-sec00a-repro`, since removed):

- Concurrent daily budget on real PostGIS (21 vs 10) — PASS.
- Comment growth + account-erasure gap (100 comments, text/alias
  resolvable after deletion) — PASS.

Result: 11/11 audit findings still reproduce on current dev.
The CS00–CS12A batch did not (and was never scoped to) fix
them. Per the remediation plan, characterization probes must
be inverted into safety regressions task by task (SEC01+), not
blessed.

## Next

SEC00-B (route/capability inventory) then SEC00-C (budget
freeze), then SEC01-A. No VPS/phone mutation. No live-readiness
inference.
