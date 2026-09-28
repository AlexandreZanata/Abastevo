# Current execution state

- Updated: 2026-09-28, P02-T01 LOCAL_DONE; P01 INTEGRATED.
- Branch: `codex/phase-02-official-catalog`, based on `8c21945`; clean.
- Phase P01+G01-FLOW INTEGRATED: PR #2 merged `2706ba4` → `8c21945` (match-head-commit, required Quick verification SUCCESS); PR #1 auto-closed (commits contained); branches `codex/phase-01-delivery-flow` and `codex/phase-01-delivery-plan` deleted locally + remotely, verified; merge recorded in PR #2 metadata; wiki WIKI_PENDING. Post-merge main CI runs automatically.
- Main protection ACTIVE: strict, required `[Quick verification]`, PR required, enforce-admins, no force/deletion.
- P02 entry G01 + G01-FLOW satisfied. Exit G02 pending; no Android modifications.
- P02-T01: LOCAL_DONE — fixtures + loader tests; quick-packages extended.
- P02-T02: LOCAL_DONE — kernel values, 94 subtests, RED→GREEN.
- P02-T03: LOCAL_DONE — `db/migrations/000002` (stations/historied identifiers/location revisions, partial unique active index, GiST + municipality indexes, append-only RESTRICT) with per-owner sqlc schemas so models never bleed; `db/queries/directory` + `modules/directory/domain` (B-BR-014 projection rule, stdlib-only) + `adapters` repository (atomic ON CONFLICT resolve, crypto UUIDs, WKT mapping); integration on real PostGIS: stable UUID, 16-worker same-CNPJ convergence, alias retire with history, missing/centroid never projected, reviewed projection + geography roundtrip; RED proven by dropping ON CONFLICT, GREEN on restore; manifest extended.
- G02: P02-T04…T08 NOT STARTED.
- Next: **P02-T04 — Bounded ANP parser**.
- Issues/milestone/PR/wiki: flow draft PR pending; wiki once per merged phase; no invented IDs.
- G09 NOT STARTED; P10 BLOCKED BY G09; roadmap 78 tasks.

## Evidence pointers

- [Prior P01 command outcomes preserved verbatim](history/P01_FOUNDATION_EVIDENCE.md).
- [Android baseline re-validation](BASELINE_VALIDATION.md#p01-t01-re-validation--2026-09-28).
- [Workflow policy](DELIVERY_WORKFLOW.md), [CI plan](CI_PLAN.md), [fast execution card](FAST_EXECUTION.md), [ADR-013](../adr/013-fast-phase-delivery.md).
- [Phase record template](templates/PHASE_RECORD.md): use one compact state and task evidence entries, not a growing full transcript in this file.
