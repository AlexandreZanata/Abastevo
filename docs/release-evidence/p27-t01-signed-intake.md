# P27-T01 — Signed station suggestions and private status

Status: LOCAL_DONE on `codex/phase-27-station-intake`. Task: P27-T01.
Binds B-BR-D07–D10 and BUC-D03/D04. Signed intake with owner-only
status; anonymous proof alone insufficient; CNPJ knowledge grants
nothing.

## Behavior (TDD, critical-first)

- Migration `000034_station_suggestions.sql` (append-only): private
  rows keyed `(account_id, client_submission_id)`; states
  pending/cancelled (+approved/rejected for T02); owner index.
  sqlc `registry.sql` extended (create/get/get-by-key/list/count/
  cancel); `sqlc vet` + `generate` clean. (`LIMIT @var` needs
  `::int` casts — caught by vet, fixed.)
- Application `IntakeService` + `ParseProposal` (moved
  domain→application: domain stays stdlib-only per
  `TestDomainStdlibOnly`, caught by test): structured validation
  (display/IBGE/UF, kernel CNPJ text, bounded coords with null-island
  refusal, 256-char evidence ref), 20/day quota `[CALIBRATE]`,
  idempotent submit (same key+body replays; same key+changed body
  conflicts — compared semantically because JSONB normalizes bytes,
  caught by integration), owner-only cancel/status/list
  (foreign → not-found, no oracle; double-cancel → closed).
- HTTP `adapters/intake`: session-first on every route (invalid →
  401, suspended/deleted → 403 via composition-mapped
  `ErrAuthorForbidden`); private routes use POST with the session in
  the JSON body, never the URL (mirrors account bindings);
  `no-store` on all responses; missing/foreign share 404.
- OpenAPI: 4 intake paths + `StationSuggestion`/`Proposal` schemas
  (`vacuum` PASS after adding the missing cancel path-param);
  `apicontract` + `check-compat.sh` PASS.
- `cmd/api` wiring with the account-session closure (modules never
  cross-read); full `go build ./...` PASS.

## Validation

- Unit (`-race`): proposal rules, service idempotency/conflict/quota/
  owner-isolation, handler submit/replay/conflict/session/input/
  foreign/cancel/mine/no-store — all PASS.
- Integration (real PostGIS, `-race`): idempotent replay, parallel
  same-station proposals stay two private rows, foreign read/cancel
  fail, cancel-then-read shows cancelled, CNPJ preserved in stored
  proposal. Fresh disposable DB (migrations incl. 000034).
- Regression: full `directory/...` unit + integration PASS (10 pkgs);
  `go vet` clean; account suites untouched (session contract reused).
- `git diff --check` PASS; `scan-secrets.sh` PASS (no PII/secrets;
  synthetic CNPJs).

## Limits and next

- Decisions (approved/rejected), exact-match automation and reviewed
  locations are T02; Android visibility is T03–T04.
- Next: P27-T02 verified decisions, corrections and reviewed locations.
