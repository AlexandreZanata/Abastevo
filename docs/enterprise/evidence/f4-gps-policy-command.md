# Tracking policy command (EMP-GPS-002 activation prerequisite)

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F4 task in worktree branch `codex/enterprise-f4-gps005`, built on
the F4 head `0912b48a` (main `b0ab7d33` + TRIP-009/013, GPS-001, OFFL-005,
TRIP-011, TRIP-007, TRIP-014/018, GPS-004/012, UGC safety). It is the
coordinator-assigned prerequisite of the EMP-GPS-002 activation: since
EMP-GPS-004 (migration 000240) nothing wrote `org_tracking_policies`, so no
organization could turn tracking on (GPS-004 and GPS-012 retained lists). Under
the owner decision EDR-271 (tracking implemented for real; activation per
organization only with its employee notice and terms in force; real-device
acceptance OWED) an organization administrator can now record the policy and
the employee notice version that must be in force. The coordinator integrates
it and owns PROGRESS, the F4 ledger, pushes and the PR. Synthetic data only;
no emulator, device or physical phone; no Android code.

Rules read: the F4 ledger (EDR-271 owner decision, GPS-001/004/012 entries and
retained lists); the [GPS-001](f4-gps001-tracking-session-domain.md),
[GPS-004](f4-gps004-track-intake.md) and
[GPS-012](f4-gps012-location-access-audit.md) evidence; the
[tracking specification](../modules/gps-navigation.md) (§3 "Tracking settings"
entity, BR-GPS-006 capability and policy, BR-GPS-034 notice at first use and at
each start, §7 roles — `org.settings.manage` "Tracking policy … OWNER, ADMIN,
changes are audited", SCR-GPS-09); [permissions](../07-permissions.md) §7.2/7.3
(`org.settings.read` O for OWNER, ADMIN, FLEET_MANAGER, SUPERVISOR, FINANCE,
APPROVER, AUDITOR; `org.settings.manage` O for OWNER, ADMIN; no `gps.*` key
configures tracking); [privacy](../11-privacy-security.md) LV-03 (notice to
employees, in-app at first entry and at each tracking start) and SCR-PRIV-01;
the [ORG-008 versioned settings](f1-org008-versioned-settings.md) evidence
(strict fixed-key typed values, heads, designations, `org.setup` admission);
the EMP-CHECK-001 template policy command (receipt, audit, CAS under the root
lock) and the EMP-TRIP-014 closure routes as code precedents. The only notice
mechanisms in code are the fixed invitation notice constants
(`membership/application/redemption.go`); no organization notice registry
exists.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-467..469, BUC-EMP-237..238, EDR-302..303, all
used; migration 000247 used.

Rules B-BR-EMP-467 (only a fresh organization administrator changes the
policy), B-BR-EMP-468 (enabled only with the employee notice in force) and
B-BR-EMP-469 (the read states the stored and the effective policy) with
BUC-EMP-237 (an administrator turns tracking on) and BUC-EMP-238 (the policy
cannot be changed around its owners) are in the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-GPS-002 prerequisite — Tracking policy command (2026-11-15)", written
before any behaviour.

Decisions in [16-decisions](../16-decisions.md), taken unattended as the
recommended routine choices:

- EDR-302: the writer lives in the `tracking` module (`PolicyService`,
  `PolicyStore`, two company routes) under the catalog keys the specification
  names (`org.settings.manage` fresh for the change, `org.settings.read` for
  the read; no key invented); `TrackingPolicyAuthority` re-proves the company
  authority over the organization itself; the existing `org.setup` admission
  class already holds `org.settings.manage`; the `revision` is optional
  (absent = no version yet) because the shared gateway admits only revisions
  of at least 1; ORG-008 settings are not reused because their strict
  fixed-key values would duplicate the 000240 in-force rule; the organization
  declares its own notice version and in-force instant (no notice text is
  stored; showing it stays with EMP-GPS-002/015); a future notice instant is
  refused instead of scheduled.
- EDR-303: migration 000247 adds only the insert-only, forced-RLS receipt
  `org_tracking_policy_commands`; the 000240 table, guard and in-force
  function are unchanged, so the GPS-004 fixture versions stay valid.

## Shape frozen before behaviour

- `tracking/application/policy.go`: `PolicyReadPermission`,
  `PolicyManagePermission`, `PolicyAuditAction` (`gps.policy.changed`),
  `PolicyTargetType` (`tracking_policy`), `PolicyAction`
  (`PolicyRead`/`PolicyManage`), `PolicySettingsPolicy`, errors
  `ErrPolicyDisabled`, `ErrPolicyUnauthorized`, `ErrPolicyConflict`,
  `ErrPolicyOperationReuse`, `PolicyField` with the frozen codes
  (`REQUIRED_WHEN_ENABLED`, `ABSENT_WHEN_OFF`, `NOTICE_VERSION_FORMAT`,
  `NOT_IN_FORCE`, `OFF_OPTIONAL_OR_EXPECTED`), `PolicyActor`,
  `PolicyCommandInput`/`PolicyCommand` (`NewPolicyCommand`, `Snapshot`,
  `Enabled`, `Digest`), `PolicyAck`, `PolicyView`, ports `PolicyAuthority`
  and `PolicyRepository`, `PolicyService` (`Change`, `Read`). Every private
  value prints redacted.
- `tracking/adapters/policy.go`: `PolicyStore` (raw parametrized SQL),
  `PolicyAuditDefinitions`.
- `tracking/adapters/http/policy.go`: `PolicyReadPath`, `PolicyChangePath`,
  `PolicyRecords`, `RegisterPolicy` (closed request bodies, organization
  target resolver).
- `organization/adapters/tracking_policy_authority.go`:
  `TrackingPolicyAuthority` (company proof, ORG-level target, ACTIVE root for
  the change).
- `db/migrations/000247_organization_tracking_policy_commands.sql` (as
  committed): `org_tracking_policy_commands` with its guard (scope; the
  receipt names the newest version; an enabled version's notice in force at
  its effective instant; an ACTIVE author membership of the actor account),
  forced RLS, statement trigger refusing UPDATE/DELETE/TRUNCATE (the 000240
  `org_tracking_keep_history`), `REVOKE ALL FROM PUBLIC`.
- Shared files: `internal/platform/orgapi/errors.go`
  (`TrackingPolicyValidation`, one function),
  `internal/platform/enterprisesuite/routes.go` (composition and audit
  definitions), `contracts/openapi/v1.yaml` (two paths, four schemas),
  `contracts/testdata/enterprise/authorization-routes-v1.json` (two routes),
  `cmd/api/authorization_inventory_test.go` (normal routes 153 → 155),
  `cmd/api/enterprise_router_test.go` (mounted list),
  `organization/adapters/auditor_matrix_http_integration_test.go` (positive
  read, 155 / 32 / 123), `internal/platform/apicontract/tracking_test.go`.
  The authorization matrix is unchanged (300 cells): both keys keep their
  representatives (`settings/read`, `settings/change`).

## RED

Tests were written against the stubs (`NewPolicyCommand`, the service and
the store returning not-implemented/unavailable), the migration as written
and, for the contract, the previous OpenAPI and inventory files.

- Application (`go test -race -run TrackingPolicy`), log
  `/data/dev/worktrees/f4-logs/gpspolicy-app-red.jsonl`: 4 top-level, 3
  failing. The passing test pins the fixed access policies, which are part of
  the shape (constant function), and is not claimed as RED.
- PostGIS (`-tags=integration -race -run TestTrackingPolicy`), logs
  `gpspolicy-db-red.jsonl`/`.stderr`: 4 top-level, 3 failing (the composed
  read answered 503; the race saw no lock wait; the upgrade test passed its
  whole migration rehearsal and then failed on the route). The schema-guard
  test passed against the migration as written (it exercises only the
  migration) and is not claimed as RED.
- Contract, with the OpenAPI and inventory changes reverted
  (`gpspolicy-contract.patch`), logs `gpspolicy-contract-red-cmd.jsonl` and
  `gpspolicy-contract-red.jsonl`: cmd/api 2/2 failing (unclassified route,
  undocumented production route) and apicontract 2/2 failing (undocumented
  route, missing policy routes).

## GREEN and REFACTOR

- Application: 4/4 (`go test -race -count=1 ./internal/modules/tracking/...`).
- PostGIS focused, log `gpspolicy-db-green.jsonl`: 4/4 under `-race`. The
  first run (`gpspolicy-db-green1.jsonl`) failed only on a test query casting
  (`uuid = text`), fixed in the test.
- cmd/api and apicontract packages pass (contract RED → GREEN by restoring the
  patch).
- REFACTOR: none beyond gofmt of two test files.

Named negatives mapped to tests (real PostGIS through the actual composed
company router unless noted):

- Non-admin: `TestTrackingPolicyChangeAndReadThroughComposedCompanyHTTP`
  (FLEET_MANAGER, SUPERVISOR, FINANCE, APPROVER, AUDITOR, a DRIVER and a
  UNIT-scoped SUPERVISOR → 403 `FORBIDDEN` `org.settings.manage`; DRIVER read →
  403 `org.settings.read`), each with an unchanged private and policy state.
- Foreign organization: same test (read and change on an unknown
  organization → 404 `NOT_FOUND`); `TestTrackingPolicyCommandSchemaGuards`
  (another scope cannot write a receipt, `42501`, and sees none).
- Policy enabled without a notice in force: same HTTP test (missing version,
  missing instant, malformed version, instant one hour in the future → 422
  `VALIDATION_FAILED` with `notice_version`/`notice_in_force_at` and the frozen
  code; notice fields with `OFF` → `ABSENT_WHEN_OFF`); the store re-checks the
  instant against the database's effective instant and the receipt guard
  refuses a receipt for a version whose notice was not in force (`23514`,
  schema test); unit `TestTrackingPolicyCommandShapeNoticeAndDigest`.
- Stale revision (CAS): HTTP test (a premature revision 1 before any version,
  the owner's stale revision 1 after the administrator wrote revision 2 →
  409 `CONFLICT`); `TestTrackingPolicyConcurrentChangesConvergeInBothOrders`
  (two administrators queued behind a held organization row lock on the same
  revision, both orders: the first queued writes, the other is `CONFLICT`
  with no version, receipt or audit event); the upgrade test (a change
  ignoring the pre-upgrade version → `CONFLICT`).
- AUDITOR: HTTP test (reads, cannot change) and the AUTH-011 walk (positive
  read of `tracking/policy/read`, `FORBIDDEN` change).
- Replay and reuse: HTTP test (same operation and body → `replayed: true`,
  same revision and effective instant, no second version or audit event;
  same operation with another body → 409 `OPERATION_ID_REUSED`).
- Effect on the real intake: HTTP test (before any version a
  `gps.session.start` is `REJECTED FORBIDDEN`; after the owner's `EXPECTED`
  the session opens under policy revision 1; after `OFF` the open session
  keeps accepting batches, BR-GPS-006).
- Insert-only and RLS: schema test (`UPDATE`/`DELETE` by the runtime login
  and `TRUNCATE` by the administrator → `55000`; forced RLS; a second receipt
  of one version `23505`; a receipt of an older version, of another account,
  with a malformed digest → `23514`).
- Populated upgrade: `TestTrackingPolicyPopulatedUpgradeFailureRecoveryAndPreviousImage`
  (a 000240 policy version and a recording session before 000247, ordinary
  non-superuser migrator, injected `1/0` leaves no table, function or row
  change, recovery applies once, replay no-op, previous image compatible, the
  receipt table owned by the migrator with forced RLS, the route then writes
  revision 2 and the session keeps recording, the remaining embedded chain
  applies).
- No value echoed: refusals carry only the field and frozen code; the
  submitted version text never appears in an error.

## Validation

Commands from `backend/` with toolchain go1.27.4 (`gps005-env.sh`),
`GOCACHE=/data/dev/cache/f4-go-build`, the shared PostGIS (isolated child
databases):

```sh
gofmt -l ./internal ./cmd ./db                                       # empty
go build ./... && go vet ./... && go vet -tags=integration ./...     # clean
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck ./internal/modules/tracking/... ./internal/platform/enterprisesuite/... \
  ./internal/platform/orgapi/... ./internal/platform/apicontract/... ./cmd/api/...
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck -tags=integration ./internal/modules/tracking/... \
  ./internal/modules/organization/adapters/... ./internal/platform/enterprisesuite/...   # clean (gpspolicy-static.log)
/home/iiii/go/bin/sqlc vet && /home/iiii/go/bin/sqlc diff            # no diff (no query or sqlc.yaml change)
go test -race -count=1 ./internal/modules/tracking/...
go test -count=1 ./cmd/api/ ./internal/platform/apicontract/
go test -tags=integration -race -count=1 -p 2 -json -timeout=90m ./internal/modules/organization/adapters \
  ./internal/modules/tracking/... ./internal/modules/trip/... ./internal/modules/orgsync/... ./internal/platform/... ./cmd/api/... ./cmd/worker/...
python3 docs/enterprise/tools/check_plan.py
git diff --cached --check
```

Results:

- gofmt empty; build and vet (both tags) clean; staticcheck plain and
  integration clean (`gpspolicy-static.log` empty); `sqlc vet` and `sqlc diff`
  clean; plan check 348 tasks, 545 rules, 167 use cases, 145 screens, 0
  problems; `git diff --check` clean.
- OpenAPI lint (`vacuum`, `gpspolicy-vacuum.log`): 0 errors, 1 109 warnings and
  681 informs — the same counts as the base head `0912b48a`
  (`gpspolicy-vacuum-base.log`); the two request-body descriptions it first
  flagged were added.
- Race/integration union on the shared PostGIS with a throwaway MinIO
  (`f4-gps005-s3`), logs `gpspolicy-db-union.jsonl`/`.stderr`/`.exit`
  (01:03–01:48Z): 33 packages, 1 139 top-level and 8 283 named subtests. The
  only failures are the 5 known `cmd/worker` tests that need the pinned Rust
  preparer CLIs (not present on this machine) and one
  `TestContactChangeConcurrentRedemptionResendAndConfirmation/confirm`
  (`register unavailable` in the employee edit racing an invitation
  confirmation, code this task does not touch, under the load of several
  agents); rerun alone three times it passes 3/3
  (`gpspolicy-contact-rerun.jsonl`), and it passed in the 52 earlier F4 union
  logs that contain it.
- The untagged `go test -count=1 ./...` runs once at the end of the
  EMP-GPS-005 commit, on the tree that contains this commit (as GPS-004 did).

## Retained obligations

- The panel screen SCR-GPS-09 and the other tracking settings of §3
  (freshness limits, divergence thresholds, long-stationary limit, raw-fix
  retention) are not part of this command; they belong to their owners
  (GPS-007/006/010/011).
- The notice text and its display at first use and each start
  (BR-GPS-034, LV-03) stay with EMP-GPS-002/015; the command stores only the
  version and in-force instant the organization declares.
- The capability side of BR-GPS-006 (an inactive organizations capability)
  is enforced by the change's company authority; the intake reads only the
  policy in force (unchanged from EMP-GPS-004).
- Production grants for `org_tracking_policies` (runtime `SELECT, INSERT`)
  and `org_tracking_policy_commands` (`SELECT, INSERT`) are not seeded, as for
  every enterprise table.
- No scheduled (future) notice: a future in-force instant is refused.
- Real-device acceptance stays OWED (EDR-271). No M4 or F4 acceptance is
  claimed.
