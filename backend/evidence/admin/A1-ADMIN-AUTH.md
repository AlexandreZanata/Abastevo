# A1 — Admin key authentication, sessions and audit

Date: 2026-10-27. Branch `codex/adm-a1-admin-auth` from `origin/main`
`44b7b915`. Rules: B-BR-ADM-01…12. Plan PR: #161 (planning documents are not
yet on `main`; this branch contains only A0/A1 source, tests and evidence).

## Delivered

- Migration `000140_admin_sessions_audit.sql`: `admin_sessions`,
  `admin_login_attempts` (IP 15-minute windows + singleton global hour row),
  `admin_idempotency`, `admin_audit_events` with an append-only trigger.
- `internal/modules/admin/{domain,application,adapters}`: pure key/reason/
  session rules, transactional login/limiter/audit store, cookie/CSRF/Origin/
  CIDR/body-cap HTTP adapter, exported `RequireSession` middleware.
- Config: `ANPFUEL_ADMIN_KEY_SHA256`, `_NEXT`, `ANPFUEL_ADMIN_ORIGIN`,
  `ANPFUEL_ADMIN_ALLOWED_CIDRS`; malformed values fail startup without echoing
  the value; no hash leaves every `/v1/admin/*` path unmounted.
- `ops admin keygen`: prints the raw key and SHA-256 once to stdout only.
- OpenAPI: `/v1/admin/session` (POST/GET/DELETE) and
  `/v1/admin/sessions/revoke-all`.
- Trusted client IP reuses `identity/adapters/http.Resolver` through a
  composition closure.

## Test map (S1–S20)

| # | Test |
|---|---|
| S1 | `cmd/api.TestAdminRoutes_ADM01_UnconfiguredReturns404` |
| S2 | `config.TestAdminConfig_ADM01_MalformedHashFails` |
| S3 | `adminhttp.TestLogin_ADM05_SetsHardenedCookie` |
| S4 | `adminhttp.TestLogin_ADM03_DenialsAreIndistinguishable` |
| S5 | `adminadapters.TestLogin_ADM04_RateLimitPerIP` + `adminhttp.TestLogin_ADM04_RateLimitedMapping` |
| S6 | `adminadapters.TestLogin_ADM04_GlobalRateLimit` |
| S7 | `adminadapters.TestLogin_ADM04_ConcurrentFailuresExact` |
| S8 | `adminadapters.TestAuthenticate_ADM05_IdleAndAbsoluteExpiry` |
| S9 | `adminadapters.TestLogout_ADM05_ADM10_ReuseAndAudit` |
| S10 | `adminadapters.TestRevokeAll_ADM07` |
| S11 | `adminapp.TestAuthenticate_ADM07RemovedHashInvalidatesSession` |
| S12 | `adminhttp.TestMutation_ADM06_CSRFAndOrigin` |
| S13 | `adminhttp.TestAdminRoutes_ADM11_UserProofDoesNotAuthorize` |
| S14 | `adminhttp.TestAdminRoutes_ADM11_AdminCookieDoesNotAuthorizeUserRoute` |
| S15 | `adminhttp.TestBody_ADM09_UnknownFieldAndLimit` |
| S16 | `adminadapters.TestMutate_ADM12_Idempotency` |
| S17 | `adminadapters.TestAudit_ADM10_AppendOnly` |
| S18 | `adminhttp.TestLogs_ADM02_ADM10_NoSecrets` |
| S19 | `adminhttp.TestAdminHeaders_ADM09_NoStoreNoIndex` |
| S20 | `adminhttp.TestAdminFence_ADM08_OutsideCIDRIs404` |

Additional: `domain.TestKeyFormat_ADM01_ADM02`,
`domain.TestKeyMatch_ADM03_ADM07`, `domain.TestReason_ADM12`,
`domain.TestSessionTiming_ADM05`, `config.TestAdminConfig_ADM06_Origin`,
`config.TestAdminConfig_ADM07_NextHashAccepted`,
`config.TestAdminConfig_ADM08_CIDRFence`,
`ops.TestAdminKeygen_ADM02`.

## Commands and results (all on the branch head before the evidence commit)

```sh
cd backend
gofmt -l .                          # empty
go vet ./...                        # pass
go test -race -count=1 ./...        # exit 0; 127 packages ok, 51 no-test, 0 failures
go test ./internal/platform/apicontract -count=1   # ok
sqlc generate && sqlc diff          # no diff
```

PostGIS integration (disposable loopback PostGIS 18/3.6):

```sh
ANPFUEL_TEST_DATABASE_URL='postgres://...@127.0.0.1:55434/anpfuel?sslmode=disable' \
  go test -race -count=1 -tags=integration \
  ./internal/modules/admin/... ./cmd/api/...
# ok: admin/domain, admin/application, admin/adapters/http,
#     admin/adapters (all migrations from empty), cmd/api; exit 0
```

Repository gates:

```sh
make quick-verify                   # ok, selection=full, 54s; 0 OpenAPI errors
git diff --check                    # empty
bash scripts/scan-secrets.sh        # no tracked secrets or high-confidence patterns
```

Secret-surface grep from plan section 0 item 6 (changed files) found no real
key, configured hash, GitHub token or e-mail address. Synthetic keys are
constructed at test runtime from bytes; no complete `abv_admin_` key literal
is committed. The only raw key ever printed is `ops admin keygen` stdout.

## Not done in this PR

- A2 granted tiers, capability projection, account search/detail and grants.
- Admin web UI and staging validation belong to later fronts/tasks.
- The planning documents (contract/ADR/plan) are still open in PR #161; this
  branch intentionally does not duplicate them.
