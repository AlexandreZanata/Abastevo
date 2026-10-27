# A4 — Staff roles and audit read

Date: 2026-10-27. Branch `codex/adm-a4-staff-roles`, stacked on A3
`codex/adm-a3-contributors` (PR #167, not merged). Rules: B-BR-STF-01…06,
BUC-ADM-10/12. Planning documents remain in PR #161.

## Delivered

- Migration `000143_staff_roles.sql`: `staff_assignments`, one active
  assignment per (account, role), optional end date and the account-erasure
  trigger (`account-erased`).
- `internal/modules/staff/domain`: frozen `staff-roles-v1` catalog (code and
  pt-BR labels), role/end-at validation.
- Golden `contracts/admin/staff-roles-v1.json`.
- Staff store/application: transactional assign/revoke with the shared admin
  reason/idempotency/audit helper.
- Admin routes: `GET /v1/admin/roles`, `POST /v1/admin/accounts/{id}/roles`,
  `POST /v1/admin/role-assignments/{id}/revoke`.
- `GET /v1/admin/audit?action=&target_id=&cursor=` with bounded pagination.
- Owner read through `POST /v1/contributor/profile/read` now returns active
  role labels.
- Public contributor list includes role labels only when the profile is
  public and `show_roles_publicly` is true.
- OpenAPI paths/schemas for staff roles and audit.

## Test map

| Rule / use case | Tests |
|---|---|
| STF-01 catalog code-only | `staffdomain.TestCatalog_STF01_MatchesGolden`, `staffhttp.TestCatalog_STF01_NoMutationRoutes`, `TestCatalog_STF01_Response` |
| STF-02 assign/revoke | `staffadapters.TestAssign_STF02` (active target, duplicate, bounds, revoke), `http.TestServiceError_STF` |
| STF-03 no permission | roles are not part of any authorization path; `staffhttp.TestRoles_STF04_DoNotAuthorizeAdmin` |
| STF-04 no admin powers | `staffhttp.TestRoles_STF04_DoNotAuthorizeAdmin` (each role, admin route without admin session) |
| STF-05 owner/public labels | `contributoradapters.TestProfile_CTB08_CTB12` (owner labels), `contributorhttp.TestPublicContributors_CTB09_CTB10_CTB11` (`show_roles_publicly` true/false) |
| STF-06 erasure | `staffadapters.TestErasure_STF06` |
| BUC-ADM-10 | `TestAssign_STF02`, `staffhttp.TestCatalog_STF01_Response` |
| BUC-ADM-12 | `staffadapters.TestAudit_STF_BUCADM12` (action/target filters + cursor) |

## Commands and results

```sh
cd backend
gofmt -l .                          # empty
go vet ./...                        # pass
go test -race -count=1 ./...        # exit 0; 133 packages ok, 0 failures
go test ./internal/platform/apicontract -count=1   # ok
sqlc generate && sqlc diff          # no diff
```

PostGIS integration (disposable loopback PostGIS 18/3.6):

```sh
ANPFUEL_TEST_DATABASE_URL='postgres://...@127.0.0.1:55434/anpfuel?sslmode=disable' \
  go test -race -count=1 -tags=integration \
  ./internal/modules/staff/... ./internal/modules/contributor/... \
  ./internal/modules/admin/... ./internal/modules/entitlement/... \
  ./internal/modules/subscription/... ./cmd/api/...
# 20 packages ok, 0 failures; exit 0
```

Repository gates:

```sh
make quick-verify                   # ok, selection=full, 153s; 0 OpenAPI errors
git diff --check                    # empty
bash scripts/scan-secrets.sh        # clean
```

Plan section 0 secret grep found no real key, configured hash, GitHub token
or e-mail address.

## Not done / notes

- No consumer endpoint enforces a staff role yet (STF-03); each future
  consumer must add its own authorization and negative tests.
- Roles never change trust, moderation, ranking or feedback weight.
- Admin web UI and staging validation remain later fronts/tasks.
- Role labels are translated in code; the catalog is not runtime editable.
