# AUTH catalog amendment `org-permissions-v5` — reservation and checklist grants

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Prerequisite owned by the AUTH module, delivered as the first of two commits
on `codex/enterprise-f4-fleet007` (based on `origin/dev` `8c2ee654`). It is
needed by EMP-FLEET-007 (reservations and blocks), EMP-CHECK-001 (templates)
and EMP-CHECK-003/004 (incident handling and release). It changes catalog
grants only: no route, migration, stored row or Kotlin source.

Read: `07-permissions.md` §4 (BR-AUTH-001/007), §7.1–§7.4 and the amendment
notes (EDR-037 v2, EDR-043 v3, EDR-048 v4, EDR-063 conformance, EDR-166
auditor); `modules/fleet.md` §5 (UC-FLEET-06/07), §7 and §12;
`modules/maintenance.md` §4 (BR-CHECK-011), §5 (UC-CHECK-04/05/06) and §7;
`contracts/testdata/enterprise/README.md`; the v1–v4 fixtures and every
consumer of the version string.

IDs used: B-BR-EMP-292, B-BR-EMP-293, EDR-190, EDR-191 (all of them).

## Rules frozen before behaviour

Observed before the change: `org-permissions-v4` registers
`fleet.reservation.manage`, `check.template.manage` and `check.incident.manage`
but no role holds them (21 unassigned keys). The fleet module assumes the roles
of `fleet.asset.assign` for reservations and blocks; the maintenance module
proposes OWNER, ADMIN, FLEET_MANAGER for templates and OWNER, ADMIN,
FLEET_MANAGER, SUPERVISOR in scope for incident handling. 07-permissions §7.4
requires an explicit matrix revision and catalog version for any new grant.

Consumers of the version or the fixture (searched across backend, contracts,
docs and Kotlin; Kotlin has none): `orgaccess/domain` (`catalog.go`, matrix,
catalog, legal, settings, ownership and auditor tests), `orgexport/application`
gating test, `cmd/api` authorization inventory test (compares the inventory
with the compiled catalog), `organization/adapters` authorization matrix and
AUTH-011 auditor walk (version pin and oracle), `authorization-routes-v1.json`,
`15-validation.md`. Stored semantics: `org_role_assignments` (000171 and later)
persists the role key, scope and period only; no table, migration or SQL
function stores a permission set, a role-to-permission mapping or a catalog
version (the `catalog_version` columns of 000123/000127 belong to payments).
Therefore no migration is needed (EDR-191).

Rules B-BR-EMP-292 and B-BR-EMP-293 are recorded in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "AUTH
catalog amendment org-permissions-v5 — reservation and checklist grants";
decisions EDR-190 and EDR-191 in [16-decisions](../16-decisions.md).

## Shape

- `contracts/testdata/enterprise/authorization-catalog-v5.psv`: the v4 file
  (86 lines, same row order) with exactly three rows changed, eleven cells
  `-` → `O`; v1–v4 are kept unchanged.
- `orgaccess/domain`: `PermissionsVersion = "org-permissions-v5"`; three rows
  added to the code-frozen main matrix (`fleet.reservation.manage O O O O`,
  `check.template.manage O O O -`, `check.incident.manage O O O O`, other
  columns `-`).
- New `reservation_check_matrix_test.go`: v4→v5 cell retention against an
  independent literal of the eleven grants; compiled coverage and the four
  standard requirements of every amended cell; the reservation and template
  columns equal the `fleet.asset.assign` and `maint.workorder.manage` columns.
- Updated pins (only as the version change requires): matrix test reads v5;
  catalog, legal, settings and auditor tests pin v5; the unassigned-key list
  drops the three keys (18 remain); the v1 retention test also expects the
  eleven v5 cells (9 → 20 explicit amendments); the export gating literal and
  the HTTP matrix oracle pin v5; the route inventory's `permissions_version`
  is v5 with every status unchanged (the three keys stay `NOT_YET_ROUTED`).
- Docs: 07-permissions §7.2 heading, three §7.3 rows, amendment note and the
  conformance paragraph; 15-validation §5 oracle reference; fleet.md §7 row and
  §12 risk; maintenance.md §7 rows; the testdata README section.

## RED

`go test -json -count=1 ./internal/modules/orgaccess/domain/ ./internal/modules/orgexport/application/ ./cmd/api/`
with the v5 fixture, tests and inventory but the v4 source →
`/data/dev/worktrees/f4-logs/catalog5-red.jsonl`: 10 top-level tests failed
(orgaccess/domain 7: auditor column, version pin, unassigned list, legal and
settings pins, `TestCatalogMatrix` with 11 failing subtests, the new v5 grant
test; orgexport 1; cmd/api 2 inventory/version tests), 68 top-level and 1,172
subtests passed. The new v4→v5 retention test compares the two independent
fixtures only, so it passed in RED by construction (it guards the oracle, not
the source).

## GREEN

- After the source change, `TestCatalogV3RetainsEveryOtherV1Cell` failed
  (the v1 comparison did not know the v5 cells); it was extended with the
  same independent literal, not loosened (exact count 20).
- `go test -json -race -count=1 ./internal/modules/orgaccess/... ./internal/modules/orgexport/... ./cmd/api/`
  → `catalog5-green.jsonl` and rerun: 10 packages ok, 128 top-level / 1,291
  subtests, 0 failures.
- PostGIS focused (`-race -tags=integration`, with a throwaway MinIO for the
  auditor walk's storage): `TestAuthorizationMatrixActualCompanyHTTP`,
  `TestAuditorReadWriteMatrixActualCompanyHTTP`,
  `TestExportAndContentGatesActualCompanyHTTP`, `TestExportDataAuthorityFailsClosed`
  → `catalog5-db-focused.jsonl`: 4/4 top-level, 3,225 subtests passed.
- Race/integration union `./internal/modules/organization/adapters
  ./internal/modules/orgaccess/... ./internal/modules/orgexport/...
  ./internal/modules/fleet/... ./cmd/api/` (`-p 2`) →
  `catalog5-db-union.jsonl`: 16 packages ok, 736 top-level / 5,641 subtests
  passed, 0 failures, 0 skips (`organization/adapters` 998.3 s, run with the
  throwaway MinIO so the S3-backed auditor and export suites ran).

## Validation

- Toolchain `gofmt -l` (go1.27.4) empty on orgaccess, orgexport, organization
  and cmd/api; `go build ./...`; `go vet` plain and `-tags=integration` on the
  touched packages clean.
- `GOTOOLCHAIN=go1.27.1 staticcheck` on orgaccess, orgexport and cmd/api, and
  with `-tags=integration` on orgaccess and organization/adapters: no findings
  (`catalog5-staticcheck.log`).
- No SQL changed: `sqlc` not applicable.
- `python3 docs/enterprise/tools/check_plan.py` → 0 problems; `git diff --check`
  clean; staged diff reviewed for secrets/PII: catalog cells, version strings
  and prose only.
- The untagged whole-module run is executed once at the end of the
  EMP-FLEET-007 commit, which contains this one.

## Negatives mapped to tests

| Negative | Test |
|---|---|
| Any v4 cell other than the eleven changes | `TestCatalogV5RetainsEveryV4CellExceptTheRecordedGrants`, `TestCatalogV3RetainsEveryOtherV1Cell` |
| AUDITOR gains a key | `TestAuditorHoldsExactlyTheDocumentedReadOnlySet` (unchanged column, v5 pin) |
| Another unassigned key becomes granted | `TestRegisteredMatrixGapsRemainDeniedWithoutInheritance` (18 keys) |
| A grant carries an extra or missing restriction | `TestCatalogV5GrantsReservationAndCheckKeysOnlyToTheRecordedRoles`, `TestCatalogMatrix` |
| Inventory or HTTP oracle silently on the old version | `cmd/api` inventory tests, `TestAuthorizationMatrixActualCompanyHTTP` |
| Role assignability changes | `TestCanAssignRoleEscalation` unchanged and passing (OWNER and ADMIN gain every new key) |

## Retained

- Routes for the three keys: `fleet.reservation.manage` by EMP-FLEET-007 (the
  next commit); `check.template.manage` by EMP-CHECK-001;
  `check.incident.manage` by EMP-CHECK-003/004, including the BR-CHECK-011
  author exclusion (`SELF_APPROVAL`) as a record rule.
- `fleet.supplier.manage` and the other 17 unassigned keys need their own
  explicit amendments.
- Request of reservations through `trip.mission.manage` alone (UC-FLEET-06)
  belongs to the mission linkage of EMP-TRIP-023.
- Production grants, panel and Android remain separate; no milestone or gate
  is claimed.
