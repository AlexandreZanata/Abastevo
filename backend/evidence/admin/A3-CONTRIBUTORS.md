# A3 — Contributors

Date: 2026-10-27. Branch `codex/adm-a3-contributors`, PR #167, stacked on A2
`codex/adm-a2-granted-tiers` (PR #166, not merged). Rules: B-BR-CTB-01…14,
BUC-ADM-05…09/11. Planning documents remain in PR #161.

## Delivered

- Migration `000142_contributor_profiles.sql`: profiles, links (max 2
  trigger), GitHub links (unique partial indexes both ways), claims (one open
  claim, 24-hour quota, 30-day expiry), profile-edit ledger, GitHub snapshots
  and the account-erasure trigger.
- `internal/modules/contributor/domain`: GitHub login rules, NFC display
  names, platform/handle allowlist + URL builder, claim code/state machine.
- `internal/modules/contributor/adapters`: PostgreSQL store and a `net/http`
  GitHub client (pagination, optional read-only token, 5 s timeout, 1 MiB
  cap). Tests use `httptest.Server` only.
- User routes: claim create/read/proof/withdraw and profile read/upsert.
- Public `GET /v1/contributors`: consented PUBLIC profiles with an active
  CONTRIBUTOR grant, sealed cursor, max 50, `Cache-Control: public,
  max-age=300`, and CORS only for `ANPFUEL_PUBLIC_WEB_ORIGINS`.
- Admin routes: GitHub snapshot with link/claim state, claims queue,
  approve/reject, direct link, hide/unhide. Approval writes claim decision +
  CONTRIBUTOR grant + GitHub link + audit rows in one transaction.
- Config: `ANPFUEL_GITHUB_REPO`, `ANPFUEL_GITHUB_TOKEN`,
  `ANPFUEL_PUBLIC_WEB_ORIGINS`; placeholders in both env examples.
- Caddy: `/v1/contributors` added to `@shared_cacheable`.
- OpenAPI: public/user/admin contributor paths and schemas.

## Test map

| Rule / use case | Tests |
|---|---|
| CTB-01 snapshot | `adapters.TestGitHubContributors_CTB01`, `TestGitHubFailures_CTB01`, `TestAdminGitHub_CTB01_StaleCache`, `config.TestContributorConfig_CTB01_CTB11` |
| CTB-02/CTB-05 claims and direct link | `TestClaim_CTB03_CTB07_QuotaExpiryAndReplay`, `TestApproval_CTB05_CTB06_AtomicGrantAndLink`, `TestLink_CTB05_Override`, `TestReject_CTB05_CTB07` |
| CTB-03 login/code/quota | `domain.TestGitHubLogin_CTB03`, `TestClaimCode_CTB03`, `adapters.TestClaim_CTB03_CTB07_QuotaExpiryAndReplay` |
| CTB-04 gist proof | `domain.TestClaimCode_CTB03`, `adapters.TestGitHubGist_CTB04`, `TestClaim_CTB04_Proof` |
| CTB-06 link uniqueness | `adapters.TestLink_CTB06_ConcurrentUnique` |
| CTB-07 claim states | `domain.TestClaimState_CTB07`, `TestReject_CTB05_CTB07`, `TestClaim_CTB03_CTB07_QuotaExpiryAndReplay` |
| CTB-08 profile/links | `domain.TestDisplayName_CTB08`, `TestLinks_CTB08`, `adapters.TestProfile_CTB08_CTB12` |
| CTB-09/CTB-10 public list | `adapters.TestPublicList_CTB09_CTB10`, `http.TestPublicContributors_CTB09_CTB10_CTB11` |
| CTB-11 CORS | `http.TestCORS_CTB11`, `TestPublicContributors_CTB09_CTB10_CTB11` |
| CTB-12 edits/hide | `adapters.TestProfile_CTB08_CTB12`, `TestHide_CTB12` |
| CTB-13 erasure | `adapters.TestErasure_CTB13` |
| CTB-14 neutrality | `domain.TestNeutrality_CTB14_NoTrustRankingImports` |
| BUC-ADM-05 | `adapters.TestAdminGitHub_CTB01_StaleCache` |
| BUC-ADM-06 | `TestClaim_CTB03_CTB07_QuotaExpiryAndReplay`, `TestClaim_CTB04_Proof` |
| BUC-ADM-07 | `TestApproval_CTB05_CTB06_AtomicGrantAndLink`, `TestReject_CTB05_CTB07`, `TestLink_CTB05_Override` |
| BUC-ADM-08 | `TestProfile_CTB08_CTB12` |
| BUC-ADM-09 | `TestPublicContributors_CTB09_CTB10_CTB11` |
| BUC-ADM-11 | `TestHide_CTB12` |

Additional HTTP tests: `TestAdminRoutes_CTB_RequireSession`,
`TestUserRoutes_CTB_RequireSession`, `TestServiceError_CTB`.

## Commands and results

```sh
cd backend
gofmt -l .                          # empty
go vet ./...                        # pass
go test -race -count=1 ./...        # exit 0; 131 packages ok, 0 failures
go test ./internal/platform/apicontract -count=1   # ok
sqlc generate && sqlc diff          # no diff
```

PostGIS integration (disposable loopback PostGIS 18/3.6):

```sh
ANPFUEL_TEST_DATABASE_URL='postgres://...@127.0.0.1:55434/anpfuel?sslmode=disable' \
  go test -race -count=1 -tags=integration \
  ./internal/modules/contributor/... ./internal/modules/admin/... \
  ./internal/modules/entitlement/... ./internal/modules/subscription/... ./cmd/api/...
# 17 packages ok, 0 failures; exit 0
```

Repository gates:

```sh
make quick-verify                   # ok, selection=full, 41s; 0 OpenAPI errors
git diff --check                    # empty
bash scripts/scan-secrets.sh        # clean
```

The plan section 0 secret grep found no real key, configured hash, GitHub
token or e-mail address. Test keys/logins are synthetic.

## Implementation notes / not done

- The one-time claim code is stored until the claim reaches a terminal state
  (needed to compare the public gist content); terminal decisions and account
  erasure clear/delete it.
- Direct GitHub link does not itself create a CONTRIBUTOR grant; the admin
  grants the tier through A2 or approves a claim. The contract lists both
  paths separately.
- `roles` is always `[]` until A4 staff roles; `show_roles_publicly` is stored
  but has no effect yet.
- A2 account detail does not yet embed contributor profile/claims; the A3
  admin routes expose them directly.
- Admin web UI and staging validation are later fronts/tasks. Real GitHub is
  never contacted in tests.

## Review fixes (2026-10-27)

- Public `next_cursor` no longer exposes account ids (CTB-10): the keyset
  position is AES-GCM encrypted inside the sealed envelope
  (`TestPublicCursor_CTB10_DoesNotExposeAccountID`).
- Admin envelope errors (idempotency key reused with another body) map to
  `409 admin.conflict` instead of `503` (`TestServiceError_CTB`).
- Claim codes and operation ids are redacted from audit rows (CTB-13,
  `assertAuditHasNoClaimSecrets` in approve/reject tests).
- Approval supersedes a lapsed CONTRIBUTOR grant as `expired` (own audit
  row) instead of failing with `entitlement.exists`
  (`TestApproval_CTB05_LapsedContributorGrant`).
- Owner profile read answers `roles: []`, never `null`, without a profile
  (`TestReadProfile_STF05_NoProfileHasEmptyRoles`).
- Dependency review: `golang.org/x/text` v0.42.0 (Go project, BSD-3-Clause,
  already in the module graph as an indirect dependency) is now imported
  directly for NFC display-name normalization; `go mod tidy` only moves it
  to the direct block. `govulncheck` in quick-verify reports no
  vulnerabilities.
