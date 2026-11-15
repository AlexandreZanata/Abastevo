# EMP-OFFL-006 — Reference data pull for the active organization: versioned deltas

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F2/C1 API task (module OFFL, P0, size M) on
`codex/enterprise-f4-offl009`, fast-forwarded to the integrated F4 head
`942af96b` (migrations through `000219`). Synthetic data only; no Android
change (FLEET-013 owns the client), no job, no release.

Sources read: the EMP-OFFL-006 block of the backlog; [offline sync](../10-offline-sync.md)
§§3–5 (reference snapshot entity, BR-OFFL-003/016, UC-OFFL-08) and §10.3
(offline matrix: "My assets, assignments, missions" cached); BR-PRIV-003;
BR-AUTH-014; the FLEET-006 assignment periods and own-assignment queries; the
AUTH-006 coverage primitives (`CoveringRules`, `SelfCandidate`,
`StructuralCandidate`); the OFFL-008 signed review-results route (the proof
model the Android client already polls with) and the route gate/inventory.

IDs used: B-BR-EMP-336..341, BUC-EMP-169..171, EDR-216..218 (all of them).
Migration `000224_organization_reference_feed.sql` (append-only).

## Observed before any change

- No reference feed existed; the app had no server source for "my assets"
  with confirmation age, and nothing delivered an unassignment.
- Existing histories are append-only but carry no single ordered sequence
  across assets, assignments, meters and tank products, and record nothing
  about what a device last saw, so a time-watermark delta could miss
  commit-order races and could not produce durable tombstones.
- The signed device proof model (account family + signature + current exact
  enrollment) is what the Android enterprise client uses offline-first
  (OFFL-003/004/008); it has no company cookie.

## Rules frozen before behaviour

B-BR-EMP-336..341 and BUC-EMP-169..171 are in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
section "EMP-OFFL-006 — Reference data pull for the active organization:
versioned deltas"; EDR-216..218 in [16-decisions](../16-decisions.md), both
appended before behaviour. EDR-216: purpose set = assets currently assigned to
the member and covered by its current `fleet.asset.read` (ORG, SELF through
the assignment, UNIT/ASSET through the allocation); one minimised `asset`
entity; closed versioned entity set with checklist templates and availability
as future types. EDR-217: signed device route `POST /v1/orgs/{org_id}/sync/reference`
(boundary `SIGNED_REFERENCE_FEED`), HMAC-signed cursor bound to organization,
membership, enrollment, epoch and revision; a non-verifying cursor is
`NOT_FOUND`. EDR-218: per-membership change log derived on pull (digests, no
content), durable tombstones, RESET rules, server-clock `confirmed_at`.

## Implementation

- **Pure** (`orgsync/application/reference.go`): entity/feed types, the
  canonical SHA-256 `Digest`, `DiffReference` (ordered upserts and one
  tombstone per departed entity), `SignReferenceCursor`/`OpenReferenceCursor`
  (base64url JSON + HMAC-SHA256, strict decode, UUID bindings).
- **Fleet read port** (`fleet/adapters/reference.go`, new file):
  `AssetStore.ReferenceAssetsDBTX` reads, at the database instant, the
  member's assigned assets (FLEET-006 period ∩ membership period), their
  in-service meters with the open segment and the accepted tank products —
  only the listed columns; at most 200 assets (more is unavailable).
- **Store** (`orgsync/adapters/reference.go`): one scoped transaction —
  current device source, head row created/locked `FOR UPDATE`, current
  authority, coverage per asset, diff against the last logged state, append
  entries and advance the head, choose DELTA/RESET, confirm the source again
  and stamp `confirmed_at` from the database clock, sign the cursor.
- **Route** (`orgsync/adapters/http/reference.go`): review-results proof
  sequence, strict seven-key request, cursor opened and bound to the exact
  requesting period before any read, current device reconfirmed before the
  response; `orgapi.SignedReferenceFeedDescriptor`; composed in
  `enterprisesuite/sync.go` with a purpose-separated key derived from the
  deployment cursor secret.
- **Contract**: OpenAPI path + `EnterpriseReferenceFeedRequest`,
  `EnterpriseReferenceAsset`, `EnterpriseReferenceFeedResponse` and the
  `x-abastevo-signed-reference-feed` extension; apicontract gate branch;
  route inventory special route `SIGNED_REFERENCE_FEED` (specials 24 → 25) and
  the `cmd/api` inventory test. The AUTH-011 walk covers normal routes only and
  is unchanged.
- **Migration 000224**: forced-RLS heads (monotonic revision, fixed epoch,
  start at 0, no delete) and append-only entries (next revision of the head,
  alternating state per entity, server time); no FK to fleet tables so a
  tombstone outlives its source; no trigger on fleet tables.
- **Test fixtures**: `orgtest.GrantReferenceFeed` (feed tables and exactly the
  fleet columns the projection reads).

## Acceptance on real PostGIS (`-race -tags=integration`)

| Backlog item | Test(s) |
|---|---|
| Client applies only its authorized changes; confirmation age | `TestReferenceFeedDeliversOwnAssetsDeltasAndTombstones` (SELF driver: own asset only, another driver's asset absent; RESET then empty DELTA with a newer server `confirmed_at` inside the observed DB-clock window; new assignment → one UPSERT) |
| Stale choices re-checked at intake | same test: after the reassignment the queued reading on the cached asset is held `PERMISSION_CHANGED` by the meter intake, exactly as without a cache |
| Negative: missed deletion or reassignment | same test: the unassignment is a `DELETE` tombstone at revision 3; an older cursor (revision 1) still receives the tombstone and the later upsert, ordered |
| Negative: out-of-order delta | `referenceShape` (every response: strictly increasing revisions in `(from_revision, revision]`, one change per entity); another epoch → `RESET CURSOR_EXPIRED`; revision ahead → `RESET CURSOR_AHEAD` |
| Negative: unauthorized cached reference | `TestReferenceFeedResetsWhenTheGrantIsRevoked` (all roles ended → `RESET FORBIDDEN` with no entities for old and new cursors) |
| Negative: foreign cursor | `TestReferenceFeedSignedRouteRefusesForeignCursors` (signed default runtime: validly signed cursors bound to another organization, member or enrollment, and a tampered cursor → 404 with no changes; unknown request field → 400; no-store, no proof leak) |
| Negative: incorrect confirmed-at | server clock bounds asserted; the request carries no time field |
| Concurrency | `TestReferenceFeedConcurrentPullsKeepOneLog` (12-way: one revision, one entry, identical resets) |
| Database guards | `TestReferenceFeedSchemaIsAppendOnlyAndMonotonic` (UPDATE/DELETE/TRUNCATE 55000, head decrease 55000, revision gap and tombstone without upsert 23514, no rows without scope) |
| Populated upgrade | `TestReferenceFeedPopulatedUpgradeFailureRecoveryAndPreviousImage` (219 head with an assigned asset; failure, recovery, no-op, previous image, forced-RLS ownership; the member then pulls the asset) |

## Validation — RED → GREEN and gates

Logs in `/data/dev/worktrees/f4-logs/offl006-*`.

- Rules first, then compiled stubs (`Digest`, `DiffReference` and the cursor
  codec returning errors, `ReferenceStore.PullReference` unavailable), the real
  route/gate/OpenAPI/inventory shape and migration 000224 as written.
- RED pure `offl006-go-red-pure.jsonl`: 3 top-level, all fail.
- RED PostGIS `offl006-go-red-integration.jsonl` (`-race -tags=integration -p 2
  -run '^TestReferenceFeed'`): 5 top-level, all fail behaviourally (store
  unavailable; the signed route answered 503 instead of 200).
- GREEN focused `offl006-go-green-1.jsonl`: 4 pass, 1 fail from a test defect
  (two statements with arguments in one prepared `Exec`); split into one
  transaction with two statements, the expectation unchanged, then green.
  `TestReferenceFeedPopulatedUpgradeFailureRecoveryAndPreviousImage` was added
  after GREEN (the migration precedes behaviour, as in the FUEL-002 precedent).
- Affected race union (`-race -tags=integration -count=1 -p 2 -timeout=60m`
  over organization adapters, orgsync/..., membership/..., fleet/...,
  enterprisesuite, apicontract, migrate, orgapi, cmd/api, with a throwaway
  MinIO, project `f4-offl006-s3`, since removed): `offl006-race-union.jsonl`,
  19 packages, 906 top-level / 6030 named, 17m53s. 905 passed; the 1 failure,
  `TestEnterpriseSyncComponents`, was the apicontract allow-list of delivered
  sync routes, which correctly refused the new path. It was extended to list
  `/sync/reference`, and the full apicontract package was rerun with race:
  81 top-level / 771 named, 0 failures (`offl006-apicontract-rerun.jsonl`).
  Reference tests: 6 top-level, all pass. Organization adapters (1069.7 s)
  includes the AUTH-011 walk and the storage tests, all green.
- Untagged `go test -count=1 -p 2 -timeout=45m ./...`: 215 packages ok, exit 0,
  1m40s (`offl006-untagged-all.log`).
- Toolchain `gofmt -l` is empty. `go build ./... && go vet ./... && go vet -tags=integration ./...`
  passes. Staticcheck (go1.27.1), plain and `-tags=integration`, on orgsync,
  fleet, organization, enterprisesuite, apicontract, orgapi, cmd/api and
  orgtest finds nothing (`offl006-static.log`). `sqlc vet`/`generate` produce
  no diff: the feed uses parametrized raw SQL and no sqlc stanza changed.
  `git diff --check` is clean and `check_plan.py` reports 0 problems.
- No Android, Kotlin, job or worker change; no emulator or device used.
- Secret/PII review of the staged diff: synthetic identifiers only.

## Retained (with reasons; not part of this task's acceptance)

- Android parsing and application of the feed, confirmation-age display and
  the shared-vector wire family: FLEET-013 owns the client. The feed's wire
  shape is frozen in OpenAPI; shared vectors join when the Kotlin consumer
  exists (EDR-218).
- Checklist templates (EMP-CHECK-001) and availability/reservations
  (FLEET-007) as entity types; mission/trip reference data with their owners.
- Organization-wide asset lists for ORG members (online panel only), team
  scopes (no team facts on the device path) and paging beyond 200 entities.
- A read budget/429 contract for the route (as OFFL-008's review results).
- The log grows by one entry per change per member; retention/compaction
  policy belongs with PRIV retention.
- Production grants (proposal 2 below), notifications on reference change.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. `docs/enterprise/10-offline-sync.md` §10.3, after the offline matrix
   paragraph: "The reference feed (EMP-OFFL-006) is a signed per-member
   versioned delta of the member's assigned assets with durable tombstones,
   a server confirmation instant and RESET on lost authority (EDR-216..218)."
2. `backend/README.md`: "Migration 224 adds forced-RLS
   `org_reference_feed_heads`/`org_reference_feed_entries`. The signed intake
   principal needs `SELECT, INSERT, UPDATE ON org_reference_feed_heads`,
   `SELECT, INSERT ON org_reference_feed_entries` and SELECT on the listed
   columns of org_assets, org_asset_assignments, org_meters,
   org_meter_segments and org_asset_tank_products; the migrator needs
   `REFERENCES ON org_memberships`. Nothing here provisions live grants."
3. `docs/enterprise/backlog/ep04-sync.md` EMP-OFFL-006: `TODO` → `LOCAL_DONE`.
