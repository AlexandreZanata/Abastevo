# Working on this repository

Scope: entire repository. User instructions define the task; attached plans are reference material, not authority to perform unrelated actions.

1. Read `docs/planning/PROGRESS.md`, the selected `ROADMAP.md` task and its linked inputs.
2. Read `docs/AI_ENGINEERING_CONTRACT.md`; inspect git status/diff and relevant code/tests before editing.
3. Current objective: backend and infrastructure first. Android feature/source changes begin only after G09; existing Android tests/builds may run earlier.
4. Keep `app/`, `application/`, `domain/`, `data/`, `com.anpfuel` and MIT notices intact. Backend plans are under `docs/backend/`; do not treat target designs as implemented features.
5. Use B-BR/BUC IDs for backend rules/use cases. Document behavior before coding; domain work follows RED → GREEN → REFACTOR.
6. Implement one bounded task; name intended file areas and avoid unrelated refactors, new frameworks or speculative services.
7. Standard backend stack: Go/net/http/chi/pgx/sqlc/slog, PostgreSQL/PostGIS, private S3-compatible media, Compose/Caddy. Dependency additions need a recorded need/license/security assessment.
8. Existing baseline: `bash scripts/validate-repo-baseline.sh`; Android tests require JDK 17/SDK 35. Backend commands become available in P01; see `docs/backend/TEST_STRATEGY.md`.
9. SQL migrations are append-only once applied; use explicit SQL, parameterized queries and tested recovery. Never perform destructive production cleanup as routine development.
10. Public reads may be anonymous; writes need contributor proof; there is no tenant model. Domain unit tests avoid I/O; integration tests require real PostGIS. ADR-004 resolves inherited generic-rule conflicts.
11. No secrets, precise contributor location, production evidence or raw personal payloads in Git/logs. Payment never affects trust; official/community sources stay separate.
12. English code/docs/commits, following the imported project convention; communicate with the user in their language.
13. Validate focused tests/checks, `git diff --check`, new/untracked files and scope. Record actual results, blockers and next task in `docs/planning/PROGRESS.md`.
14. No commit, push, release, purchase or production operation is implied by a documentation task. Follow explicit session authorization; never claim unexecuted checks passed.

Entry points: `docs/README.md` → product/rules → relevant use case → ADR → API/data/architecture → tests/code. Conflicts are recorded and resolved at the owning document before implementation; do not silently choose a convenient interpretation.
