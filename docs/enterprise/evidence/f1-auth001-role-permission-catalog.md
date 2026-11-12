# F1 EMP-AUTH-001 fixed role and permission catalog

Status: LOCAL_DONE
Validation: PASS

2026-11-11. Pure-domain/source acceptance, INTEGRATION_PENDING through
[PR202](https://github.com/AlexandreZanata/Abastevo/pull/202). F1/M1 remain open.
This is a catalog of descriptors, not actual request authorization or grants.

## Entry and source interpretation

The owner requested the next enterprise task and GitHub publication. Entry
head b05da1a2 (IDN-003) matched remote dev/open PR202; main ef96cd13 remains
its ancestor. The isolated codex/enterprise-f0-f1 worktree entered clean;
occupied local dev and unrelated checkout work remain preserved. AUTH-001
depends on accepted PLAT-001 and is the next eligible foundation before
AUTH-002's real role assignments and locked last-owner policy.

Read current progress/fast card, task, permissions §§4/7, EDR-008, validation
matrix/VAL-21 and existing staff/station catalog boundaries. B-BR-EMP-45/46,
BUC-EMP-23 and permission §7.4 interpretation precede behavior. The oracle
was pinned from the source tables before the compiled source rows. Initial
RED: the safe empty system catalog refused the expected roles/keys; the
complete source catalog then passes all cells.

The literal registry has **84 keys**. An initial three-segment extraction
missed its stock.reconcile entry; literal inspection corrected the draft
oracle/count before GREEN. This exact published two-segment key is retained
as a compatibility exception, without inventing a rename or grant. The
matrix covers 61 keys; the other **23** stay registered and denied for all
roles. Proposed module permission rows are not imported as authority.
Later grants require explicit matrix and catalog version changes.

The station table's "by assignment only" means organization OWNER/ADMIN
does not represent a station. Main-table agreement permissions apply on the
fleet side; STATION_ADMIN's applies on the station side under live authority
(station-commerce §7). This keeps the party boundary explicit even though
the permission key is shared. No new policy or broader scope is invented.

## Implemented source

orgaccess/domain confirms the planned module name. SystemCatalog exposes
org-roles-v1 and org-permissions-v1 with all 13 canonical private roles.
Definitions are code-owned/validated; no runtime or custom registration
API exists. Unknown roles/permissions, duplicate roles/keys/grants, malformed
source, unregistered references and misplaced footnotes fail closed. Invalid
compiled source fails startup rather than installing partial empty defaults.
Zero catalog/rule values grant nothing. Errors never echo input.

Lookup and Rules return immutable descriptors with originating role,
coverage and sorted requirements. O stays within the live assignment's
scope. S covers only own records and assigned assets. The four footnotes
retain admin's OWNER exclusion, driver own trips, auditor recorded tracks
and fleet-side agreement restrictions. All station rules additionally need
live station authority; separate station/fleet agreement side markers are
preserved when reading multiple role views.

Every positive descriptor retains ACTIVE_MEMBERSHIP, LIVE_SCOPED_ASSIGNMENT,
ORGANIZATION_AND_PLAN and RECORD_RULES. OWNER has explicit S cells and no
record-rule, self-review or station exemption. The OWNER role definition
states that assigning it needs a current OWNER. Management/read keys are
separate; paid tiers, aliases and wildcards never become roles or permissions.
The existing administrator/manager representation names are metadata only,
and existing staff/account/station roles are not replaced or extended.

Lists are sorted fresh copies; rule slices and requirement slices are copied.
The shared source maps are read-only after validated construction. This
platform catalog holds no account or private organization selection, so it
does not accept an OrgScope as authority. Actual assignments and request
checks remain scoped owning-command/service work.

## Validation

The [oracle](../../../contracts/testdata/enterprise/authorization-catalog-v1.psv)
has 84 permission rows × 13 roles = **1,092 cells**, including all denied
and unassigned cells. The consumer checks header/count/uniqueness, exact
known lookups, complete coverage and sorted requirement sets without skips.
Production rows are separately transcribed source constants, not loaded
from the test oracle or a runtime configuration file.

Additional tests cover every matrix footnote/party boundary, zero and unknown
values, all 23 gaps, no management/read inheritance or subscription role,
station aliases, OWNER's restrictions, malformed/duplicate source, partial
catalog refusal, independent role descriptors, copied input/output slices
and 32 concurrent readers mutating only their own returned views.

Go module toolchain 1.27.4; staticcheck v0.8.1 uses Go 1.27.1. From backend/:

```sh
go test -race ./internal/modules/orgaccess/domain -coverprofile=/tmp/enterprise-auth001-coverage.out -count=1
go test -race ./internal/modules/orgaccess/domain ./internal/modules/membership/domain ./internal/modules/staff/domain ./internal/modules/stationprofile/domain ./internal/modules/kernel -count=1
go vet ./internal/modules/orgaccess/domain ./internal/modules/membership/domain ./internal/modules/staff/domain ./internal/modules/stationprofile/domain ./internal/modules/kernel
staticcheck ./internal/modules/orgaccess/domain ./internal/modules/membership/domain ./internal/modules/staff/domain ./internal/modules/stationprofile/domain ./internal/modules/kernel
go build ./cmd/api ./cmd/worker ./cmd/ops
```

All pass without skips. New domain statement coverage: **100%**. Existing
membership/staff/station/kernel regressions preserve the prior boundaries.
No SQL, applied migration, account/session flow or production composition
changes; prior IDN-003 database evidence remains scoped to its unchanged
inputs. No new PostGIS/native/aggregate run is claimed for this pure catalog.

Go formatting, plan verifier/three regressions, added Markdown links,
git diff --check and staged Gitleaks review pass. The verifier retains
348 tasks, 545 rules, 167 use cases, 145 screens and zero problems. The new
domain package enters the required quick behavioral manifest. No dependency
or licensing surface is added. Rollback removes the unused catalog before
consumers, preserving earlier private data and source/history evidence.

## Next and retained gates

AUTH-002 is now eligible after AUTH-001/IDN-003. It must persist actual
assignments and install the real locked role/last-owner SQL policy; the
existing membership policy remains UNAVAILABLE. AUTH-003 owns current
authorization, AUTH-009 owns complete route/membership/scope matrix proof.
The catalog cell suite does not certify those runtime gates. The 23 missing
grant rows remain explicit work for their owning approved matrix revisions.

F1/M1 remain open. Existing single-family revocation decisions, real AUTH/IDN
providers, production non-bypass grants, composed-router gate, vault inventory/
sweep/revoked-device cleanup, native acceptance, EMP-DOC-014, staging lifecycle/
backup roles, PLAT-012/GOV-014, aggregate Kotlin coverage and legal receipt
gates remain owed. Publication updates the cumulative PR; no merge, VPS
rollout or production certification is inferred.
