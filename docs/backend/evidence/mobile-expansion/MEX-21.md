# MEX-21 — Revalidation and conservative station/company association

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `08495125` (maintained dev, PR #144 merge). Date: 2026-10-22.
Depends MEX-19/20 (aggregate satisfied by 20a–f), accepted. Rules
B-BR-MEX-11/12/17; BUC-MEX-07–09. Risk: attribution/jobs/races
(critical: company linkage, revision scoping, no Directory writes).

Before behavior: enforcement source records gain source-backed company
linkage under the current-company policy, reusing the exact-CNPJ
resolution without inventing station, site or historical meaning. Only
records with an admitted company identifier are plannable; records
without one stay source-global and are refused here instead of silently
dropped. Decisions are stored revision-scoped in their own tables (the
sanctions-bound association table is untouched). Revalidation compares
stored decisions with fresh Directory evidence without writing: a bound
link that lost its current binding flags "unbind" (retirement,
succession, suspension, moved station); an unbound record that became
bindable flags "review" (rebound candidate that must not auto-publish
under a frozen key). No Directory write, no fuzzy (name/base-CNPJ/
proximity) binding, no PMQC dated-rule inheritance, no reads/HTTP here
(MEX-23). Recovery: unassociate uncertain links, preserve independently
read facts; withdrawal cascades associations away with the revision.

## Delivered

- `backend/db/migrations/000112_quality_enforcement_company_association.sql`:
  append-only revision and decision tables FK'd to enforcement source
  records (not sanctions), with the closed resolution vocabulary and
  the current-company-only scope frozen in CHECKs.
- `backend/db/queries/quality/enforcement_company_associations.sql`
  (+ generated Go): record context gated on staging enforcement
  revisions with full source-fact census, stage decision, read
  decisions with subject CNPJ, and per-kind counts.
- `backend/internal/modules/quality/adapters/enforcement_company.go`:
  `PlanEnforcementCompanyAssociations` (admitted records only, via the
  existing planner), `StageEnforcementCompanyAssociations`
  (once-per-revision census-checked staging), and
  `RevalidateEnforcementCompanyAssociations` (pure deterministic
  divergence flags; restart-safe by construction).
- `enforcement_company_test.go`: five unit cases — exact current
  binding, source-global refusal, fuzzy-branch refusal, retirement/
  succession/rebound/stable flags with restart determinism, and
  review-only rebound.
- `enforcement_company_integration_test.go`: PostGIS staging of a
  bound decision with census, restage conflict (no fork), withdrawal
  CASCADE empties associations, Directory row count unchanged.

## Actual checks

From this worktree (2026-10-22, parent `08495125`):

- `go test -count=1 -run TestPlanEnforcement|TestRevalidate
  ./internal/modules/quality/adapters/` → PASS, 5 tests.
- `go test -race -count=1 -tags=integration -run
  TestEnforcementCompanyAssociation ./internal/modules/quality/adapters/`
  (loopback disposable PostGIS) → PASS: bound decision staged and read
  back with station, census 1/1, restage conflicts, revision withdrawal
  removes associations, Directory untouched.
- `go test -count=1 ./internal/modules/quality/...` → PASS, all
  packages; `go vet ./...` PASS; `GOTOOLCHAIN=go1.27.1 staticcheck
  ./internal/modules/quality/...` PASS; `sqlc vet` and `sqlc diff`
  PASS (migration 000112 + queries, deterministic generate).
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No OpenAPI, Android, worker, publisher or read-path changes; no
  source activation, no Directory writes, no PII in Git/logs/fixtures.

## Limits

- This unit plans, stages and revalidates associations only. It does
  not publish, serve reads, or bind stations beyond the exact current
  CNPJ evidence. A later read stage (MEX-23) must fence live bindings
  in its own transaction; revalidation flags are advisory until then.
- Rebound candidates ("review") are never auto-bound and never
  republished under a superseded key. Unbinding is a flag here, not a
  deletion of source facts (which stay frozen and reachable).
- Records without an admitted CNPJ are out of scope for linkage by
  design; they remain source-global and reachable through the frozen
  enforcement reads.
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-22 (source-record identity and grouping) needs its own
authorization; it consumes this tested association boundary.
