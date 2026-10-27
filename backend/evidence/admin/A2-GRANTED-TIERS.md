# A2 — Granted tiers and capability projection

Date: 2026-10-27. Branch `codex/adm-a2-granted-tiers`, PR #166, stacked on A1
`codex/adm-a1-admin-auth` (PR #164, not merged). Rules: B-BR-ENT-01…12,
BUC-ADM-02/03/04. Planning documents remain in PR #161.

## Delivered

- Migration `000141_entitlement_grants.sql`: grant table, partial unique
  active `(account_id, tier)` index, owner/list indexes and the account-erasure
  trigger (`state=REVOKED`, `revoke_reason='account-erased'`).
- `internal/modules/entitlement/domain`: `tier-catalog-v1`, defensive
  capability derivation, tier/end-at/state validation.
- `internal/modules/entitlement/adapters`: tx-scoped create/revoke/get/list
  store; expired and revoked grants never count.
- Golden `contracts/admin/tier-catalog-v1.json`; test proves the Go catalog
  equals the JSON and `PRO` equals the paid Premium bundle.
- Subscription projection (minimal diff): `adminGrants` beside `webGrants`,
  `Snapshot.AdminCapabilities/AdminValidUntil/Tiers`, `mergeAdmin`,
  `RequireCapability` admin check, `Status.Tiers`, reason `verified-admin`.
- Admin endpoints: account search/detail, grant create (with `replace`),
  bounded grant list with cursor, grant revoke. All mutations use the A1
  transactional audit/idempotency helper.
- Composition in `cmd/api`: capability gates no longer require configured
  payment ports; admin grants work with `premiumService.Configured()==false`.
- `contracts/premium/policy-v1.json` additive `tier_catalog_version`;
  OpenAPI paths/schemas for accounts and grants plus the additive `tiers`
  property on `PremiumStatusResponse`.

## Test map

| Rule / case | Test |
|---|---|
| ENT-01 admin-only mutations | `adminhttp.TestGrant_ENT01_RequiresAdminSession`; routes mounted only inside `RequireSession` |
| ENT-02 ineligible target | `entadapters.TestGrant_ENT02_IneligibleTarget`, `adminadapters.TestGrant_ENT02_AdminIneligible`, `adminhttp.TestGrant_ENT02_ENT03_ErrorMapping` |
| ENT-03 duplicate/replace | `entadapters.TestGrant_ENT03_ConcurrentDuplicateActive`, `entadapters.TestGrant_ENT03_ReplaceRollbackKeepsOld`, `adminadapters.TestGrant_ENT02_ENT03_ReplaceAndAudit` |
| ENT-04 end_at bounds | `entdomain.TestEndAt_ENT04`, `adminadapters.TestGrant_ENT04_EndAtBounds` |
| ENT-05 catalog/no capabilities | `entdomain.TestCatalog_ENT05_MatchesGoldenAndPremium`, `TestCatalog_ENT05_DefensiveCopy`, `entadapters.TestGrant_ENT05_NoCapabilityColumn`, `adminhttp.TestGrant_ENT05_CapabilitiesFieldRejected` |
| ENT-06 expiry | `entdomain.TestGrant_ENT06_ENT07_ActiveWindow`, `subadapters.TestAdminGrant_ENT06_ExpiryBoundary` |
| ENT-07 revoke | `entdomain.TestGrant_ENT06_ENT07_ActiveWindow`, `adminadapters.TestGrant_ENT07_RevokeAgainConflicts`, `subadapters.TestAdminGrant_ENT07_RevokeThenCheckDenies` |
| ENT-08 union | `subdomain.TestAdminGrant_ENT08_UnionOfSources`; `adminGrants` + `mergeAdmin` after `mergeWeb` |
| ENT-09 status/tiers | `subadapters.TestAdminGrant_ENT09_ENT10_PaymentsUnconfiguredAndTiers` (state/reason/capabilities/tiers JSON) |
| ENT-10 payments unconfigured | `subadapters.TestAdminGrant_ENT09_ENT10_PaymentsUnconfiguredAndTiers` (each PRO capability) |
| ENT-11 erasure | `entadapters.TestGrant_ENT11_ErasureRevokes` |
| ENT-12 neutrality | `entdomain.TestNeutrality_ENT12_NoTrustRankingImports` |
| BUC-ADM-02 search | `adminadapters.TestSearchAccounts_BUCADM02`, `adminhttp.TestSearchAccounts_BUCADM02_ShortQuery` |
| BUC-ADM-03 grant | `adminadapters.TestGrant_ENT02_ENT03_ReplaceAndAudit` |
| BUC-ADM-04 revoke | `adminadapters.TestGrant_ENT07_RevokeAgainConflicts` |

## Commands and results

```sh
cd backend
gofmt -l .                          # empty
go vet ./...                        # pass
go test -race -count=1 ./...        # exit 0; 128 packages ok, 0 failures
go test ./internal/platform/apicontract -count=1   # ok
sqlc generate && sqlc diff          # no diff
```

PostGIS integration (disposable loopback PostGIS 18/3.6):

```sh
ANPFUEL_TEST_DATABASE_URL='postgres://...@127.0.0.1:55434/anpfuel?sslmode=disable' \
  go test -race -count=1 -tags=integration \
  ./internal/modules/admin/... ./internal/modules/entitlement/... \
  ./internal/modules/subscription/... ./cmd/api/...
# all packages ok; exit 0
```

Repository gates:

```sh
make quick-verify                   # ok, selection=full, 40s; 0 OpenAPI errors
git diff --check                    # empty
bash scripts/scan-secrets.sh        # no tracked secrets or high-confidence patterns
```

Secret-surface grep from plan section 0 item 6 found no real key, configured
hash, GitHub token or e-mail address. Test keys are constructed at runtime.

## Not done in this PR

- A3 contributors, A4 staff roles and their account-detail fields; the current
  detail returns grants and paid state only.
- Admin web UI and staging validation.
- Live payment ports remain disabled; this PR only proves granted access
  without them.
