# EMP-PRIV-005 — Access log for sensitive reads: documents, location and exports

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

One bounded F3 API task on `codex/enterprise-f3-priv005` (cut C1, P0, size S).
It adds a scoped sensitive-access audit contract to the platform audit module
and routes the existing EMP-PLAT-006 private document issue/read path through
it. No HTTP route, OpenAPI change, job, producer, app or panel surface is
added. Export and location reads have no producer yet.

Sources read: the EMP-PRIV-005 task block, BR-AUD-001..006, BR-AUTH-011/013/
014, BR-PRIV-004/008/021, UC-PRIV-05, BR-RPT-003/004/005, UC-RPT-01/05,
BR-GPS-030/033/039, EMP-RPT-001, EMP-GPS-012 and EMP-BACK-003 blocks, the
authorization catalog v4 (`doc.document.view_content`, `rpt.export.run`,
`gps.location.read_live`, `gps.track.read`, `audit.event.read`), the
`orgaudit` module and its migration `000165`, the `orgmedia` module, and the
evidence of EMP-PLAT-003 and EMP-PLAT-006.

## Rules frozen before behaviour

Pre-allocated IDs: B-BR-EMP-207..210, BUC-EMP-106..107, EDR-146..147. All are
used; none are left reserved.

Proposed contract section title: "Enterprise sensitive-access log
(EMP-PRIV-005)".

Rules B-BR-EMP-207, B-BR-EMP-208, B-BR-EMP-209, B-BR-EMP-210, BUC-EMP-106, BUC-EMP-107 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-PRIV-005 — Sensitive-access log for documents, exports and location" (frozen before behaviour; text moved verbatim at integration).

Decisions EDR-146, EDR-147 are recorded in [16-decisions](../16-decisions.md).

## Implementation

- `backend/internal/modules/orgaudit/domain/access.go` — the closed
  `AccessCategory` set with `Permission()`, `TargetType()` and `Action(stage)`;
  `SensitiveAccess` (UUID references, closed codes and a 64-hex binding)
  with `Validate()` and `Event(scope)`, whose `orgaudit.access.v1` digest
  covers organization, account, membership, category, stage, target, grant,
  request and binding; `AccessDefinitions(...)` (seven actions, reason
  `member.authorized` only; unknown or repeated categories return nil so the
  catalog refuses composition); `ErrReplayed`.
- `backend/internal/modules/orgaudit/application/access.go` — the
  `AccessRecorder` port and its contract: call after scope, membership,
  session and category permission; serve only after nil; every error serves
  nothing. It names the current consumer and the two categories without a
  producer.
- `backend/internal/modules/orgaudit/adapters/access.go` — `AccessLog` over
  the existing `Runner`: it reserves the generic event and inserts the detail
  in the callback, so both commit or roll back together. Errors are bounded to
  `ErrInvalid`, `ErrReused`, `ErrReplayed` or `ErrUnavailable`.
- `backend/db/migrations/000202_org_sensitive_access_events.sql` (additive,
  append-only chain; floor stays `000132`) — table
  `org_sensitive_access_events` (org, operation, category, stage, membership,
  grant), organization-root and composite audit-event foreign keys, closed
  CHECKs, a unique grant key per organization for `ISSUED`, forced RLS with
  USING/WITH CHECK, `REVOKE ALL FROM PUBLIC`, the existing append-only
  statement trigger, a row guard (scope, event action/target type/reason,
  live/capability shape, member-bound grant of the same category, target and
  actor) and a deferred constraint trigger on `org_audit_events` that refuses
  any sensitive action committed without its detail. Runtime logins need
  `SELECT, INSERT` on both tables; no grant is seeded.
- `backend/db/queries/orgaudit/access.sql` (+ generated `access.sql.go`,
  `models.go`) and the orgaudit stanza of `backend/sqlc.yaml`.
- `orgmedia`: `AccessAudit` now maps issue/read to `DOCUMENT_CONTENT`
  `ISSUED`/`SERVED` and records through `AccessLog` with the same runner;
  `AuditDefinitions()` returns the document part of the sensitive vocabulary
  (unchanged action names). `Issue` records before the capability is minted.
  A new `ErrForbidden` passes through the authority boundary for a member
  without `doc.document.view_content` (BR-AUTH-013); everything else stays
  `ErrNotFound`/`ErrUnavailable` as before. The S3 integration fixture grants
  the new table to its runtime login.

Export and location reads: the categories, permissions, target types and
actions are part of the contract and tested through `AccessLog`, but no
export download or location read exists, so nothing produces them yet. No
route, OpenAPI operation or producer was added.

## Validation

Toolchain Go 1.27.4 (`GOCACHE=/data/dev/cache/f3-go-build`), staticcheck
under Go 1.27.1, sqlc v1.31.1; commands run from `backend/` unless noted.
PostGIS: the owned disposable control database on `127.0.0.1:32775`
(PostGIS 3.6) through the `orgtest` harness, which created and dropped its
own child databases and ordinary runtime logins.

RED (exported API compiled with `errNotImplemented`/zero stubs, orgmedia
`ErrForbidden` declared but not mapped, no migration):

- Unit, `go test -race -count=1 -json ./internal/modules/orgaudit/... ./internal/modules/orgmedia/...`:
  34 top-level tests, 7 fail (the four new domain tests,
  `TestAccessLogFailsClosedOnEveryRecordOutcome`,
  `TestAccessRecordPrecedesCapabilityMinting` — the token was minted before
  the record — and `TestMissingContentPermissionIsForbiddenWithoutRecord` —
  the denial surfaced as `ErrUnavailable`), 27 existing pass; 34 subtests
  pass; no compile error, no skip. Log
  `/data/dev/worktrees/f3-logs/priv005-go-red.jsonl`.
- Integration, `-race -tags=integration` on `orgaudit/adapters` and the three
  new orgmedia PostGIS tests: 18 top-level, 10 fail (`relation
  "org_sensitive_access_events" does not exist`, missing `000202` file, not
  implemented), 8 existing pass. Log
  `/data/dev/worktrees/f3-logs/priv005-go-red-integration.jsonl`.

GREEN:

- Unit, same command with `-covermode=atomic -coverprofile`: 34/34
  top-level and 34/34 subtests pass, 0 failures, 0 skips. Coverage:
  `orgaudit/domain` 100.0%, `orgaudit/application` 100.0%,
  `orgaudit/adapters` 46.1% (store paths are integration-only),
  `orgmedia/domain` 100.0%, `orgmedia/application` 93.2%, `orgmedia/adapters`
  90.6%. Log `/data/dev/worktrees/f3-logs/priv005-go-green.jsonl`, profile
  `/data/dev/worktrees/f3-logs/priv005-go-unit-cover.out`.
- Integration, `go test -race -count=1 -tags=integration -covermode=atomic -coverprofile=... ./internal/modules/orgaudit/...`
  plus the orgmedia adapters selection
  `-run 'TestDocumentContentReadsAreRecordedThroughTheSensitiveAccessLog|TestDeniedDocumentReadsServeNothingAndLeaveNoRecord|TestRefusedAccessRecordServesNothing|TestAccessAudit|TestJPEG|TestPrivateContent|TestS3Configuration|TestCursor|Test.*Token'`
  (every orgmedia adapter test except the three that require S3): 31/31
  top-level and 23/23 subtests pass, 0 failures, 0 skips; `orgaudit/adapters`
  12.4 s, `orgmedia/adapters` 3.9 s. Coverage with PostGIS:
  `orgaudit/adapters` 90.8% (`AccessLog.RecordAccess` 100%), orgaudit total
  95.8%, `orgmedia/adapters` 80.5% (`AccessAudit.Record` 100%; the S3 adapter
  is the uncovered part). Log
  `/data/dev/worktrees/f3-logs/priv005-go-green-integration.jsonl`, profiles
  `priv005-go-integration-{orgaudit,orgmedia}-cover.out`.
- REFACTOR: the database-guard probes were rewritten from formatted SQL to
  parameterized statements and gained the target-type and other-target
  probes; the suite was re-run green (the GREEN integration log above is the
  post-refactor run).

What the PostGIS tests prove (BUC-EMP-106/107):

- `TestDocumentContentReadsAreRecordedThroughTheSensitiveAccessLog`: one
  issuance and one read by a `FLEET_MANAGER` write exactly one `ISSUED` and
  one `SERVED` row each with organization, actor account, membership,
  category, target type/UUID, grant link (served grant equals the issuance's
  request, its own request differs) and reason; nothing in organization B;
  the joined rows contain no token, content marker, storage key, `/doc/`,
  content digest, MIME, file name, URL, `X-Amz` or coordinate-shaped value.
- `TestDeniedDocumentReadsServeNothingAndLeaveNoRecord`: foreign document,
  foreign organization and foreign member are `NOT_FOUND`; a `SUPERVISOR`
  (no `doc.document.view_content` in the real catalog) is `FORBIDDEN`; an
  expired session is refused without leaking detail; after issuance a role
  change to `SUPERVISOR` is `FORBIDDEN`, and an expired session, another
  member's or another organization's use of the capability, a revoked grant
  and the five-minute expiry serve nothing — each leaves no new row in either
  organization and storage is never read.
- `TestRefusedAccessRecordServesNothing`: with `INSERT` on the detail table
  revoked, issuance mints no capability and a read with an earlier capability
  releases no bytes, and the generic event rolls back too; restoring the
  grant restores access with exactly one new record.
- `TestSensitiveAccessRecordsScopedReferencesForEveryCategory`: all seven
  category/stage combinations store the exact derived event and detail; the
  detail schema has only UUID and closed-code columns.
- `TestSensitiveAccessReplayConflictAndConcurrentDuplicates`: an identical
  retry is `ErrReplayed`, a changed binding under the same request is
  `ErrReused`, a second issuance of an existing grant is refused, and twelve
  concurrent identical reads record once with eleven replays.
- `TestFailedDetailWriteLeavesNoAuditEvent`: a refused detail insert leaves
  neither row; the retry after recovery records once.
- `TestSensitiveAccessIsolationAppendOnlyAndDatabaseGuards`: identical
  request/grant UUIDs coexist in two organizations; a scoped runtime read sees
  only its own rows and an unscoped one sees none; the database refuses a
  foreign-organization insert (42501), a detail without event or without its
  member-bound grant of the same category, target and actor (23503), a
  sensitive event without detail at commit, a category/stage/target type
  that does not match the action, an operator reason, a capability read that
  is its own grant and a live read with a grant (23514); UPDATE, DELETE and
  TRUNCATE fail (55000) even after an accidental broad grant.
- `TestAuditReaderListsAccessRecordsOnlyWithScopedAuthority`: the existing
  `Service.List` returns only the authorized organization's access records
  with actor and reason, and refuses another organization and a missing
  authorizer.
- `TestSensitiveAccessMigrationUpgradeFailureRecoveryAndPreviousCompatibility`:
  from the chain before `000202` with a pre-upgrade EMP-PLAT-006 issuance
  event, a broken `000202` leaves no table, function, trigger or ledger row;
  the retry applies exactly `000202`; the previous and current chains check
  compatible; a re-apply is a no-op; the floor stays `000132`; the
  pre-upgrade grant cannot be redeemed (no member binding) while fresh
  accesses record.

Unit tests prove the closed categories and their catalog permissions
(including `FINANCE` reading trips without location, `SUPERVISOR` without
document content or export, `AUDITOR` with recorded tracks only), the seven
registered actions with only the member reason, the canonical digest and its
sensitivity to every fact, refusal of coordinate, file-name, storage-key,
URL, token and malformed values, the recorder's fail-closed outcomes, and in
orgmedia that the record precedes minting and that a missing content
permission is `FORBIDDEN` with nothing recorded, minted or read. Existing
orgaudit and orgmedia tests pass unchanged except the S3 fixture's grant line.

Wider sweep (the migration adds a trigger on the shared `org_audit_events`
table, so every enterprise package that writes audit events was re-run on
PostGIS): `go test -race -count=1 -tags=integration -p 4 -json` over
`orgaudit/...`, `membership/adapters/...`, `organization/adapters/...`,
`orgaccess/adapters/...`, `orgcapability/adapters/...`,
`orgevent/adapters/...`, `orgsync/adapters/...`, `fleet/adapters/...`,
`platform/{migrate,database,orgapi,orgquota}/...` and
`enterpriseexport/...`: 22 packages pass (407 top-level and 347 subtests
outside `organization/adapters`, 0 failures, 3 pre-existing real-data
skips). `organization/adapters` reached Go's default 10-minute package timeout
in that contended run with all 282 completed tests passing, so it was re-run
alone with `-timeout 45m`: 340/340 top-level and 2369/2369 subtests pass in
619.8 s. Logs `/data/dev/worktrees/f3-logs/priv005-go-integration-sweep.jsonl`
and `/data/dev/worktrees/f3-logs/priv005-go-integration-organization.jsonl`.
The sweep compiled the pre-refactor guard test; the refactor touched only that
test file, whose final version passed in the GREEN integration run.

Gates:

```sh
export PATH=/data/dev/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.4.linux-amd64/bin:$PATH GOCACHE=/data/dev/cache/f3-go-build
export GOTMPDIR=/data/dev/cache/f3-tmp TMPDIR=/data/dev/cache/f3-tmp STATICCHECK_CACHE=/data/dev/cache/f3-staticcheck   # final static checks
gofmt -l ./internal/modules/orgaudit ./internal/modules/orgmedia ./db/queries/orgaudit    # empty
go build ./... && go vet ./...                                                             # clean
go vet -tags=integration ./internal/modules/orgaudit/... ./internal/modules/orgmedia/...  # clean
go test -count=1 ./...        # 192 packages ok, 72 without tests; 2416 top-level + 5723 subtests pass, 0 fail, 4 pre-existing skips
GOTOOLCHAIN=go1.27.1 /home/iiii/go/bin/staticcheck [-tags=integration] ./internal/modules/orgaudit/... ./internal/modules/orgmedia/...   # no findings
/home/iiii/go/bin/sqlc generate && /home/iiii/go/bin/sqlc vet && /home/iiii/go/bin/sqlc diff   # clean
python3 docs/enterprise/tools/check_plan.py      # (repo root) tasks 348; rules 545; use cases 167; screens 145; problems 0
git diff --check                                  # clean
git diff | grep -nEi 'password|secret|token|BEGIN .*KEY|@gmail|cpf'
git diff --binary | gitleaks stdin --redact --no-banner   # no leaks found
```

The untagged backend log is
`/data/dev/worktrees/f3-logs/priv005-go-backend-untagged.jsonl`; the four
skips are pre-existing data/vector generators (`identity/profile`,
`enterpriseexport/portalgen` ×2, `quality/domain`). The keyword scan matches
only the word "token" (capability-token vocabulary, the synthetic
`v1.synthetic-capability-token` refusal input and the existing synthetic HMAC
fixture key); there is no credential, key or personal data. All identifiers
are synthetic `7b0e2a54-…` UUIDs or generated UUIDv4s. Staticcheck log
`/data/dev/worktrees/f3-logs/priv005-go-staticcheck.log` (empty). Source
manifest `/data/dev/worktrees/f3-logs/priv005-source.sha256` (17 files);
migration `000202` SHA-256
`70f6b94967a33b3e165ed2268bbc2abb4e4ac71a9228bf56234696378acfa758`.
No dependency, OpenAPI, route, job, producer, Kotlin, app or panel change.

## Retained obligations

- Live authority binding: no production `orgmedia.Authority` exists. The
  tests' role fixture uses the real `SystemCatalog` and
  `DOCUMENT_CONTENT.Permission()`, but resolving the live session,
  membership, scoped assignment and `doc.document.view_content` decision
  belongs to EMP-DOC-002 with EMP-AUTH-003, as do the document routes and the
  HTTP mapping of `FORBIDDEN` with the missing key.
- Export producer: EMP-RPT-001 records `EXPORT_FILE` `ISSUED`/`SERVED`
  through `AccessRecorder` and owns BR-RPT-005 job creation audit, filter
  digest and row count (as its binding or its own job event).
- Location producers: EMP-GPS-012 (F4) records `LOCATION_LIVE` and
  `LOCATION_TRACK` and owns BR-GPS-033 (driver own trips only).
- Support access: no support/operator reason is registered and the database
  guard refuses any reason other than `member.authorized`, so operator access
  to content stays impossible, not allowed. EMP-BACK-003 must add approved
  support sessions, a reviewed reason and the read-side support authorizer
  (BR-AUD-006, BR-PRIV-021, BR-BACK-003).
- Reader side: there is no organization audit read route. Reads go through
  the existing `orgaudit` `Service.List`, gated by an injected
  `AuthorizeAuditRead` (organization-wide `audit.event.read`); it lists the
  generic event columns. A concrete authorizer, the audit viewer (UC-RPT-05)
  and exposing category, membership and grant to readers remain with the
  owning API tasks.
- Runtime grants and backups: production runtime logins need `SELECT,
  INSERT` on `org_sensitive_access_events`; backup roles need the same
  RLS-bypass property recorded for `org_audit_events`. Retention follows the
  separately reviewed audit retention procedure.
- The S3-backed orgmedia suites (`storage_integration_test.go`) were updated
  for the new table and compile under `-tags=integration`, but were not run:
  no disposable S3 bucket was provided to this task. The PostGIS suites
  replace S3 with an in-memory object port and run the real tokens,
  inspector, audit store and migration.
- Integration: LOCAL_DONE only; no push, PR, CI, staging or release claim.

## Proposed shared-doc changes

1. `docs/product/ABASTEVO_EMPRESAS_CONTRACT.md`: append a section titled
   "Enterprise sensitive-access log (EMP-PRIV-005)" holding the CONTRACT
   block above.
2. `docs/enterprise/16-decisions.md`: append the EDR block above (EDR-146,
   EDR-147) after the last existing decision row.
3. `docs/enterprise/backlog/ep01-platform.md`: set EMP-PRIV-005 `state
   LOCAL_DONE` and add the BACKLOG block below under its task block.
4. `docs/enterprise/09-architecture.md`, organization audit section, after
   the paragraph on migration 000165, add: "Sensitive reads (EMP-PRIV-005)
   record through `orgaudit` `AccessRecorder`/`AccessLog`. Migration 000202
   adds `org_sensitive_access_events`, one append-only detail row per
   sensitive audit event (category, stage, membership, grant), written in the
   same runner transaction, with forced RLS, a guard that matches the event's
   action and member reason and links a served capability to its
   member-bound issuance, and a deferred check that no sensitive event
   commits without its detail. The runtime role receives SELECT and INSERT
   only. Owners record before minting a capability or releasing content and
   serve nothing on any error." followed by a link labelled "Evidence" to
   `evidence/f3-priv005-sensitive-access-log.md` (relative to `09-architecture.md`).
5. No `scripts/check-manifest.txt`, OpenAPI or test-inventory change is
   needed: the touched packages are already quick packages and no route was
   added.
