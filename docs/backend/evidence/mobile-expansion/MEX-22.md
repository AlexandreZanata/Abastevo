# MEX-22 — Source-record identity, safe grouping and date semantics

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `a325890d` (maintained dev, PR #145 head). Date: 2026-10-22.
Depends MEX-17/20 (aggregate satisfied by 20a–f)/21, accepted. Rules
B-BR-MEX-10/12/13; BUC-MEX-07/08. Risk: factual loss/double count
(correlation must retain every distinct row and never invent cases).

Before behavior: enforcement observations gain explicit correlation
semantics for the read stage (MEX-23) to consume. A source document
groups the observations the source states under it — the only
documented case link; no name, company, address, procedure or proximity
inference ever forms a case. Distinct results sharing a document stay
distinct observations in one case, never merged or deduplicated. Counts
stay explicit: observations (distinct retained rows) versus cases
(distinct documents); a repeated snapshot raises neither. The date
basis is declared per observation (document day or UNKNOWN); the source
states no extraction, occurrence or finality clock here. Case identity
is stable across revisions while the document number is stable; a
corrected result changes the observation but not the case. Recovery:
remove unsupported case links, retain minimized raw facts.

## Delivered

- `backend/internal/modules/quality/domain/enforcement_case.go`:
  `EnforcementObservation` (minimal correlation view),
  `EnforcementCase` (document-scoped group with basis and retained
  observations in first-seen order), `ObservationCounts` (explicit
  observations vs cases), `StableCaseKey` (cross-revision case
  identity, scoped by profile; empty document or profile forms no
  case), `DateBasis` (document day or UNKNOWN, never guessed), and
  `GroupEnforcementRecords` (pure deterministic grouping with loss
  accounting: every distinct observation retained exactly once).
- `backend/internal/modules/quality/domain/enforcement_case_test.go`:
  five cases — distinct results preserved in one case, repeated
  snapshot raises no count, correction keeps the case with both
  observations, missing date/document handled as UNKNOWN/counted but
  ungrouped, and case-key determinism/scoping/collision-freedom.

## Actual checks

From this worktree (2026-10-22, parent `a325890d`):

- `go test -count=1 -run TestGroupEnforcement|TestStableCaseKey
  ./internal/modules/quality/domain/` → PASS, 5 tests.
- Full-source grouping proof (local, pinned private workbook via the
  trusted preparer and `LoadEnforcement`, then grouping; temporary
  verification, not committed): 244,247 observations grouped into
  141,290 cases with all 244,247 observations retained, matching the
  independent document census exactly; 33.2 s.
- `go test -count=1 ./internal/modules/quality/domain/
  ./internal/modules/quality/application/` → PASS; `go vet
  ./internal/modules/quality/...` PASS; `GOTOOLCHAIN=go1.27.1
  staticcheck ./internal/modules/quality/domain/` PASS.
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No storage, migration, HTTP, OpenAPI, worker or publisher changes;
  no source activation, no PII in Git/logs/fixtures.

## Limits

- This unit defines correlation semantics only. It stores nothing,
  serves no reads and assigns no company/station links (MEX-21 owns
  association; MEX-23 owns the read API that consumes these types).
- The only case basis is the source-stated document number. Records
  with an empty document number are counted as observations but form
  no case; where issued-case identity is absent, callers must count
  labelled records instead of inventing cases.
- A corrected observation is a new observation in a stable case; the
  grouping does not itself version, withdraw or republish (revision
  lifecycle stays with publication).
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-23 (national source-record read API) needs its own
authorization; it consumes this tested correlation boundary.
