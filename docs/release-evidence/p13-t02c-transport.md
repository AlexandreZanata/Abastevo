# P13-T02C — HTTP transport, mail adapter, account paths

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR #27,
draft). Issue: #23 (slice 3 of 3 — P13-T02 COMPLETE, pending phase
merge). SMTP delivery stays out by design (local deterministic adapter
only, per A02).

## What changed

- `account/adapters/http/http.go` — four routes with a narrow `Service`
  port: `POST /v1/accounts/email/codes` (always 202, enumeration-safe),
  `POST /v1/accounts/email/consume` (200 account+session+created),
  `POST /v1/accounts/sessions/refresh` (200 rotated session),
  `POST /v1/accounts/sessions/revoke` (access-validated revoke-all).
  8 KiB body cap, strict JSON content-type, stable `account.*` codes over
  the `ApiError` envelope, every response no-store, error bodies never
  echo addresses/codes/tokens.
- `account/adapters/mail/memory.go` — in-memory `MailSender` recording
  issuances for tests/local runs; no network, no logging of secrets.
- `cmd/api/main.go` — narrow closure wiring (PGStore, memory sink,
  production CSPRNG generators, wall clock) plus an explicit
  `memory-preview` startup log. Process boots unchanged otherwise.
- `contracts/openapi/v1.yaml` — `accounts` tag, `SessionTokens` +
  `AuthResult` schemas, four paths with operationIds and no-store
  contracts. Nine `testdata/api/account-*.json` vectors (valid + pattern/
  required violations).
- Tests: 6 handler suites (202-always, malformed matrix, login flow,
  refresh/revoke incl. reuse→revoked, secret-leak sweep, oversize cap).

## Privacy refinement found in-task

Wrong-code verdicts previously leaked row state when no hash matched
(`consumed`/`expired` on a wrong guess). Now only a hash match (proof of
possession) surfaces row states; anything else is `code-unknown`, with
attempts still burning on the newest live row. Both stores changed
identically; one T02A expectation updated to present the right code.

## Validation (exact commands, this host)

```sh
cd backend && gofmt -l ./... && go vet ./internal/modules/account/... ./cmd/api/ \
  && go test -count=1 ./internal/modules/account/... ./internal/platform/apicontract/... \
  && go test -count=1 -race -tags=integration ./internal/modules/account/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
make quick-verify
git diff --check
```

Outcome: handler suites 6/6, unit + real-DB `-race` integration green,
apicontract (9 new vectors) PASS, vacuum baseline restored (0 errors /
0 warnings / 9 informs — operationId/tags/descriptions fixed in-task),
`gofmt`/`vet` clean, `quick-verify` ok, `diff --check` clean, secrets
scan clean.

## Limits (not claimed)

- Memory mail sink delivers nowhere; SMTP lands with deployment config.
- Per-IP transport throttling is edge/infra scope; domain per-address
  quota + cooldown enforced in-service and tested.
- Suspended/deleted enforcement is P13-T04; provider/native login are
  P13-T03/T05. G12 stays PARTIAL until the P12-T04 macOS run.

## Rollback

Unregister the four routes + delete the account module/paths: the
process boots as before (migration tables unread). No flag needed; no
data at stake pre-launch.

## Next

P13-T03 Google/Apple verification (#24): OIDC adapters, sandbox
evidence, cross-account and email-only refusal integration.
