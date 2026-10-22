# MEX-20c — Compatible enforcement storage

State: LOCAL_DONE / INTEGRATION_PENDING. Parent: 2dffb82e.

B-BR-MEX-10/11/12/16 and BUC-MEX-07: keep every distinct minimized
source fact, no person identifier/name/address, no inferred station/company
relationship. A document day is not an occurrence, decision or extraction day.
Use an additive quality-owned native context/record ledger alongside retained
legacy rows; same-revision foreign keys and locked staging fence publication
and withdrawal. No source activation.

Acceptance: prior-schema retained-row hashes/replay, count/date/field refusals,
real PostGIS rollback, staging-state/foreign-stream refusal and writer locks;
deterministic sqlc, targeted race/vet/static and secret/diff review.

## Executed evidence

The forged accepted/duplicate census test failed on the submitted bridge and
passes after enforcing actual accepted/repeat counts; an empty manifest no
longer dereferences absent date bounds. Context records preserve calendar only.
Migration 000111 adds quality-owned context and minimized native records; legacy
tables and keys are untouched. SHA-256 binds minimized content, not a claim of
mathematical injectivity. The opaque native tuple hash is not independently
recomputed from excluded raw personal identifiers.

`go test -race -tags=integration ./internal/modules/quality/adapters -run
TestEnforcementStorage -count=1`: 3 tests PASS, with upgrade/replay retained-row
hash, invalid context/rows, six concurrent inserts (one winner), late rollback,
foreign/published refusal and an observed PostgreSQL blocking withdrawal race.
Application admission 13 refusals plus explicit empty input and prepared
regressions PASS; race, vet, staticcheck, sqlc vet/diff and diff-check PASS.
Analysis uses installed staticcheck with Go1.27.1; runtime remains Go1.27.3.
No new dependency, raw private fields, activation, PR or release.
