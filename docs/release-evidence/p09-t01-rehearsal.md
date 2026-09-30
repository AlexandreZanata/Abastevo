# P09-T01 local release rehearsal (disposable drill)

Date: 2026-09-30. Candidate: `4ce5aaf79aa3bf8d8cf31c05635dfa2b30714556`
(P08 merge). Rehearsal tree is candidate-equivalent: core product paths
`backend/cmd`, `backend/internal`, `backend/db`, `backend/go.*` and
`contracts/openapi` match the candidate; only the harness itself
(`backend/testdata/e2e`, `infra/scripts/rehearse.sh`,
`scripts/tests/test-rehearse.sh`, `Makefile` entry) plus docs differ.

## What ran

- `make test-rehearse` on the disposable dev PostGIS
  (`infra/compose.dev.yml`, loopback API `127.0.0.1:18093`):
  happy path plus 3 mutant refusals, all PASS.
- Happy path: 2 synthetic stations (`c000…` namespace, scrubbed after),
  6 public reads + 4 denied paths, source separation intact (B-BR-001),
  no secret material in the API log (B-BR-011).
  - `GET /health/live`, `/health/ready` → 200
  - `GET /v1/stations?q=Rehearsal&limit=20` → 200 with rehearsal rows
  - `GET /v1/stations/{id}` → 200
  - `GET /v1/stations/{id}/prices?fuel_product=GASOLINE_REGULAR` → 200,
    official `ANP` vs community `COMMUNITY` separated; empty community
    stays null (UNKNOWN), never official substitution
  - `GET /v1/stations/{id}/official-prices?limit=5` → 200 separate history
  - Denied: bad UUID 400, unknown station 404, unauthenticated
    `POST /v1/observations` 401, nearby without coords 400.
- Full signed-write BUC-002…007 flows via existing suites on the same
  disposable DB: `go test -race -count=1 -tags=integration ./...` →
  57 packages `ok`, 0 `FAIL` (register, observe/validate, evidence,
  confirm/dispute/consensus, moderation, export/erase, jobs, migrations).
- `make quick-verify` → ok 8s (selection=full: Makefile + scripts changed).
- `gofmt -l backend/` empty; `git diff --check` clean; secret scans clean.

## RED proof

Two rehearsal bugs failed before the fix (GREEN on restore): the search
query (`rehearse` vs `Rehearsal`) returned zero rows, and the response
key check used `id` instead of the wire field `station_id`. Mutants for
missing binary, wrong candidate and dev-guard bypass are all refused.

## Limits (explicit non-claims)

- Local disposable drill only: 2 rows, loopback, ephemeral cursor secret.
  Passing here proves the harness and cross-module serving, not
  production capacity.
- Deployed staging acceptance (30-minute load, R2, TLS/edge, provisioned
  host) stays pending; nothing provisioned in this task.
- Photo upload end-to-end needs staging R2; local evidence relies on the
  SigV4/media unit + integration suites (P05) plus this read smoke.
- Worker crash/kill-and-drain and backup/restore replay keep their
  dedicated P03/P06/P08 evidence; this rehearsal does not re-run them.
- G09 certification needs P09-T02/T03 on the same candidate; this task
  alone does not certify the release.
