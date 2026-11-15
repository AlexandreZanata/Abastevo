# EMP-FUEL-002 — Fueling persistence, sync handler and arithmetic consistency

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F3 DB task (module FUEL, cut C1, P0, size L) on
`codex/enterprise-f3-fuel002`, opened from the integrated F3 batch head
(migrations through `000212`; `000211`/`000213`/`000214` unused). It persists
one organization fueling and its reading evidence through the signed intake.
Synthetic C1 data only; no Android, panel, job, expense or release change.

Sources read: the EMP-FUEL-002 block and the FUEL-003/005/006/011/012/015 and
EXP-002 blocks of the backlog; [fuelings](../modules/fuelings.md) §§3–12
(BR-FUEL-001..024, UC-FUEL-01, §10.1/10.2 fields and arithmetic);
BR-MONEY-004/005 and the worked example of
[04-business-rules](../04-business-rules.md); BR-OFFL-004 and §§4–8 of
[10-offline-sync](../10-offline-sync.md) (operation catalog: `fueling.submit`);
[07-permissions](../07-permissions.md) BR-AUTH-005 and the matrix row
`fuel.fueling.record S S S S - - S S -`; the FUEL-001 pure domain
([evidence](f2-fuel001-supply-domain.md), EDR-091..094); the OFFL-002 ledger
([evidence](f2-offl002-intake-ledger.md)); the METER-002 producer and its full
closure ([evidence](f2-meter002-persistence-intake.md), EDR-173/174); the
FLEET-005 snapshot port (B-BR-EMP-226); APPR-001 review lifecycle; APPR-006
period gate; DOC-003 link port (EDR-162).

IDs used: B-BR-EMP-263..267, BUC-EMP-134..135, EDR-175..177 (all of them).
Migration `000215_organization_fuelings.sql` (append-only).

## Rules frozen before behaviour

Observed before any change: no fueling table, store, handler or route exists;
the only signed intake producer is `meter.reading.record`; the ACK warning
vocabulary is exactly `CLOCK_SKEW` (OFFL-001 frozen protocol);
`orgvalue.Reconcile` admits only the financial-v1 `L`/`M3` units; the
FLEET-005 snapshot port has only a `pgx.Tx` form while the signed path only
borrows a DBTX (EDR-106); the DOC-003 `LinkTx` port needs a `pgx.Tx` and a
company web actor; the asset, unit and cost-centre usage sources refuse any
foreign key they do not list (latest bodies: 000210 asset, 000208 unit and
cost centre); `fuel.fueling.record` is an `S` cell for every role holding it.

Rules B-BR-EMP-262, B-BR-EMP-263, B-BR-EMP-264, B-BR-EMP-265, B-BR-EMP-266, B-BR-EMP-267, BUC-EMP-134, BUC-EMP-135 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-FUEL-002 — Fueling persistence, sync handler and arithmetic consistency" (frozen before behaviour; text moved verbatim at integration).

Decisions EDR-175, EDR-176, EDR-177 are recorded in [16-decisions](../16-decisions.md).


## Implementation

- Migration `000215_organization_fuelings.sql` (append-only): forced-RLS,
  insert-only `org_fuelings` (original revision-1 fact, composite
  same-organization foreign keys to the sync operation, asset, responsible and
  author employee records, author membership, enrollment, `fuel-products-v1`
  product/unit pair, the FUELING reading, unit and cost centre), with CHECKs
  re-proving the exact arithmetic (gross, expected = (q×p+5000)/10000,
  difference, derived-only-without-price), reading-or-reason, `NOT_FOUND`
  place and microsecond floor + remainder; `org_fueling_warnings`
  (`ARITHMETIC_MISMATCH`, `PRODUCT_NOT_ACCEPTED`, revision 1) and
  `org_fueling_review_entries` (APPR-001 `SUBMIT`/`SUBMITTED` sequence 1;
  FUEL-003 relaxes the CHECK when it appends decisions). A BEFORE trigger
  proves the source (operation type/schema/entity/digest/membership/enrollment
  and enrolled device of the author), refuses `DRAFT` assets and non-active
  or merged responsible employees, and re-proves the FLEET-005 snapshot at
  the declared instant; a deferred constraint trigger publishes a fueling
  only with its ACCEPTED receipt and effect at revision 1, its first review
  entry, the exact arithmetic warning and a FUELING-origin reading of the
  same operation, author, enrollment, instant and asset. The 000210 asset and
  000208 unit/cost-centre usage sources are redefined from their latest
  bodies with every existing allow-list line kept; fuelings count as asset
  operations (they fix the kind and block a draft discard), unit and cost
  centre snapshots are associations, not current use.
- Domain: `fueling/domain.Reconcile` (EDR-176) on the FUEL-001 candidate
  through `orgvalue.Reconcile`.
- Application (`fueling/application`): the `fueling.submit` codec (exact
  17-key payload, FUEL-001 candidate validation, MALFORMED versus retained
  unavailable forms), the embedded `meter.Fueling` command, ports
  (`Authority`, `Readings` = fleet embedded port + lookups + lifecycle,
  `Store`) and policies (`fuel.fueling.record` / `fuel.fueling.read`).
- Adapters (`fueling/adapters`): sqlc store (`db/queries/fueling`) and the
  intake handler: authority/target before and after the reading lookup, the
  fleet root/asset locks, final MALFORMED checks, holds (no fact, retry),
  arithmetic, `RecordEmbedded`, insert, final re-classification; replay
  re-proves the stored fact with current `fuel.fueling.read`.
- Authority (`organization/adapters/fueling_authority.go`): `FuelingAuthority`
  (same real sources as the meter authority) with `CheckFuelingTx` (current
  device, target, CoveringRules over the stored own record and its snapshot;
  ORG-only root selector) and `ClassifyFuelingTx` (EDR-118 continuity through
  the generalised `historyDisposition`; the meter keeps `meterHistoryRule`).
- Fleet: `AssetStore.UnitAndCostCentreAtDBTX` (borrowed form of the FLEET-005
  port; the `pgx.Tx` form delegates, unchanged semantics).
- Runtime: `enterprisesuite` registers the fueling owner beside the meter with
  `NO_ASYNC_FOLLOWUP` for ACCEPTED/REJECTED; OpenAPI declares the owner and
  `EnterpriseFuelingSubmitPayload`; `scripts/check-manifest.txt` lists the two
  new packages; `orgtest` grants the asset-usage column of `org_fuelings`.
- No route, authorization-matrix, inventory, Android or shared-vector change
  (route inventory counts unchanged: routed 21, signed 1, not routed 63,
  normal 110, special 24; matrix 210 cells; AUTH-011 walk unchanged).

## Acceptance on real PostGIS (signed intake through the real handler)

| Backlog item | Test(s) |
|---|---|
| AC: matching resend returns the stored result | `TestFuelingIntakeStoresOneFactWithItsReadingAndReplaysTheOriginalAck` (3 sends, byte-identical ACK, 1 fueling/reading/warning/entry/effect/audit); `TestFuelingIntakeDefaultRuntimeSignedTLSAcceptsAndReplays` (default runtime, signed TLS); SELF replay in `TestFuelingIntakeAuthorityAtReceipt` |
| AC: paid total survives a mismatch, stored as warning | same first test (paid 97000 vs expected 97350: ACK ACCEPTED, paid/gross 97000, difference 350, `ARITHMETIC_MISMATCH` row); domain `TestReconcileFollowsTheWorkedExamplesAndKeepsThePaidTotal` (13 cases incl. §10.2, KWH, M3, derived, widened gross) |
| Foreign asset (other organization) | `…AuthorityAtReceipt/foreign-asset-not-found` (real second organization's asset: whole-response `NOT_FOUND`, no claim/receipt/audit/fact) |
| Mismatched digest under the same operation | first test: `OPERATION_ID_REUSED`, stored fueling unchanged, original ACK still replayed |
| Without reading evidence / invalid reason | `TestFuelingIntakeRefusesMalformedFormsWithoutAFact` (neither, both, reading before the meter's installation: final MALFORMED receipt, no fueling, no reading); unknown meter / meter of another asset: `DEPENDENCY_PENDING`, nothing stored; parser `TestParseOperationRefusesMalformedAndKeepsUnsupportedForms` (blank reason, 29 malformed + 8 retained forms) |
| Embedded reading fails → no partial row (both directions) | `TestFuelingIntakeRollsBackTheReadingAndTheFuelingTogether` (failure after the real reading insert and after the real fueling insert: nothing stored, no claim; the same operation then records both) |
| Warning never rejects; paid never recomputed | `TestFuelingIntakeKeepsWarningsAsFactsAndTheMissingReasonPath` (KWH on a DIESEL tank → `PRODUCT_NOT_ACCEPTED` + ACCEPTED; derived price stays derived; discount reconciliation keeps entered amounts; another responsible named by an ORG member) |
| Concurrent duplicate operations → one row | `TestFuelingIntakeConcurrentDuplicatesLeaveOneFueling` (12-way on a 12-connection pool: identical ACCEPTED ACKs or RETRY, exactly one fueling/reading/effect, replay afterwards) |
| SELF on unassigned asset → FORBIDDEN; changed assignment → held | `…AuthorityAtReceipt/{self-never-assigned-forbidden,other-asset-assigned-forbidden,self-assigned-own-fueling-and-replay/another-responsible-forbidden}`; `…/declared-before-assignment-held-unavailable` (classifier returns `HOLD_PERMISSION_CHANGED`; nothing stored, RETRY/UNAVAILABLE — EDR-177); pure `TestFuelingHistoryRuleCoversOwnRecordsAndKeepsTheMeterRule` (10 cases + meter regression) |
| Post-disposal | `TestFuelingIntakeDisposalStoresNothingAtOrAfterTheInstant` (D and D+1ns store nothing; D−1ms accepted) |
| Incompatible product for the tank | warn, never refuse (BR-FUEL-016/017): second test and the default-runtime test |
| RLS without org setting; isolation; insert-only | `TestFuelingSchemaIsForcedRLSAndInsertOnly` (no rows and refused insert without setting, none for another organization, UPDATE/DELETE/TRUNCATE → 55000 even with grants) |
| Populated upgrade 000212 → 000215 | `TestFuelingPopulatedUpgradeFailureRecoveryAndPreviousImage` (populated org/unit/cost centre/active tanked vehicle/meter/readings; injected failure leaves schema, functions and data unchanged; recovery applies once; no-op replay; previous image compatible; migrator-owned forced RLS; usage answers unchanged and every allow-list line kept); `TestMeterPopulatedOrdinaryUpgradeFailureRecoveryAndPreviousImage` still passes on the full chain |
| Contract | `TestFuelingSubmitContractIsAnExplicitAtomicOwnerWithExactValues`, updated `TestMeterStandaloneContractHasOneExplicitAtomicOwner` (two owners) |

## Retained (with reasons; not part of this task's acceptance)

- Held fuelings (`PERMISSION_CHANGED` after changed rights, `ASSET_UNAVAILABLE`
  at or after disposal): classified exactly like the meter, but nothing is
  stored and the operation stays `RETRY`/`UNAVAILABLE` (EDR-177). A held
  fueling needs an owner that can resolve it (`fuel.fueling.review`,
  FUEL-003) plus an OFFL-008 review-source extension
  (`org_sync_review_source_valid`, resolution trigger) for `fueling.submit`;
  a review item without a resolution path would be a fake held fact.
- Receipt link (`LinkTx`) and receipt bytes: `LinkTx` is the web-session
  `pgx.Tx` port with a company actor and its own link operation; the signed
  path never exposes `pgx.Tx` (EDR-106) and the receipt bytes follow the
  operation through DOC-004. Attachments stay unavailable; the `FUELING`
  link target (column, foreign key, resolver, EDR-162) belongs with that
  producer.
- Panel/API reads (list/detail): FUEL-012 (with FUEL-003); tests read the
  database directly, so no route, OpenAPI path, inventory or matrix change.
- Expense projection (one cost per fueling): EXP-002; the owner declares
  `NO_ASYNC_FOLLOWUP` until then.
- Capacity tolerance (`OVER_CAPACITY`), late entry, duplicate suspicion,
  price reference snapshot, revisions/decisions/amendments: FUEL-005,
  FUEL-006, FUEL-003/015 and the intelligence tasks. Period locks are not
  consulted at intake (B-BR-EMP-266).
- Trip, contracted supplier, catalog-station, organization-supplier and
  internal-tank references, tank/tank-group destination (F6), structural
  (`UNIT`/`COST_CENTRE`/`ASSET`/`TEAM`) grants in the signed intake (as
  B-BR-EMP-262) and a SELF-only member's malformed operation (batch-level
  `FORBIDDEN`, as the meter).
- Android producer (FUEL-011) and its Kotlin arithmetic mirror: no shared
  vector changed, so no Kotlin/Android code was touched and no emulator or
  device was used. `orgvalue.Reconcile` keeps its L/M3 vocabulary; KWH uses
  the same formula in the fueling domain (EDR-176) — a shared exact-values
  extension is optional for FUEL-011.
- Production grants (proposal 4 below), panel screens and release remain
  separate. No real receipt, legal gate, deployment or money movement.

## Proposed shared-doc changes

Not edited here; exact text for the coordinator.

1. Product contract: move the bullets between `CONTRACT:BEGIN`/`CONTRACT:END`
   verbatim under a new section after "EMP-METER-002 closure — SELF field
   readings on assigned assets", headed
   `## EMP-FUEL-002 — Fueling persistence and signed intake (2026-11-14)`
   with the usual evidence link to this file.
2. `docs/enterprise/16-decisions.md`: insert the rows between
   `EDR:BEGIN`/`EDR:END` in number order.
3. `docs/enterprise/backlog/ep08-fuelings.md` EMP-FUEL-002: state `TODO` →
   `LOCAL_DONE` with the note between `BACKLOG:BEGIN`/`BACKLOG:END`
   (only if the coordinator accepts the validation below).
4. `backend/README.md`, after the meter intake grants paragraph, new
   subsection `### Fueling intake (EMP-FUEL-002)`:
   "Migration215 adds forced-RLS, insert-only `org_fuelings`,
   `org_fueling_warnings` and `org_fueling_review_entries`. The signed intake
   principal needs `SELECT, INSERT ON org_fuelings, org_fueling_warnings,
   org_fueling_review_entries` and `SELECT ON org_employee_identities,
   org_asset_tank_products, org_asset_unit_periods,
   org_asset_cost_centre_periods` besides its meter and sync grants; every
   principal that can reach the asset usage source (asset discard or kind
   change) needs `SELECT (org_id, asset_id) ON org_fuelings`. The migrator
   needs `REFERENCES` on organizations, org_sync_operations, org_assets,
   org_employee_identities, org_memberships, org_device_enrollments,
   org_fuel_products_v1, org_meter_readings, org_units and org_cost_centres
   and ownership of `org_asset_current_usage`, `org_unit_current_usage` and
   `org_cost_centre_current_usage`. The runtime registers `fueling.submit`
   beside `meter.reading.record` with `NO_ASYNC_FOLLOWUP` for ACCEPTED and
   REJECTED; changed-right and post-disposal fuelings store nothing and stay
   RETRY/UNAVAILABLE. Nothing here provisions live grants."
5. `docs/enterprise/modules/fuelings.md`: §3 after "Names are proposed; the
   first persistence task freezes them." add "EMP-FUEL-002 froze
   `org_fuelings`, `org_fueling_warnings` and `org_fueling_review_entries`
   (EDR-176)."; §8 Operations, append "Schema 1 of `fueling.submit`
   (EDR-175) carries the exact 17-key payload of
   `EnterpriseFuelingSubmitPayload`; a reading is recorded with origin
   `FUELING` in the same transaction; trip, contracted supplier, place
   references other than a typed `NOT_FOUND` name and attachments stay
   unavailable until their owners exist."
6. `docs/enterprise/10-offline-sync.md` §8 (held outcomes): "A fueling whose
   rights changed between its declared instant and its receipt, or declared
   at or after the asset's disposal, is not stored and stays
   `RETRY`/`UNAVAILABLE` until the held-fueling review owner exists
   (EMP-FUEL-002, EDR-177)."
7. `docs/enterprise/07-permissions.md` §8, after the EMP-METER-002
   interpretation: "EMP-FUEL-002 interpretation (2026-11-14, EDR-177): in the
   signed intake `fuel.fueling.record` (an `S` cell for every holder) is
   covered by an `ORG` grant with a positive rule (the new fueling is the
   member's own record), and by a `SELF` grant only while the asset is
   assigned to the member and the member is the responsible employee, with
   EDR-118 continuity; structural grants stay unsupported in the signed
   intake. A receipt replay needs current `fuel.fueling.read` over the
   stored record and its unit/cost-centre snapshot."
8. `docs/enterprise/09-architecture.md` / INDEX: no change required; the
   coordinator may list the new `fueling/application` and `fueling/adapters`
   packages where module packages are enumerated.
9. `scripts/check-manifest.txt` and `contracts/openapi/v1.yaml` were edited
   in this task as authorized (two quick packages; one owner and one payload
   schema plus a description sentence; no path, status or route change).

## Validation — RED → GREEN and gates (2026-11-14)

Environment: backend cwd, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f3-go-build`
(tmp/staticcheck caches under `/data/dev/cache`), owned PostGIS
`127.0.0.1:32775` (orgtest isolated child databases/roles), coordinator MinIO
for the union. Logs in `/data/dev/worktrees/f3-logs/fuel002-*`.

- Rules frozen first (this file), then compiled stubs (`Reconcile`,
  `ParseOperation`, store, handler and `FuelingAuthority` returning
  unavailable; `fuelingHistoryRule` empty) and the migration.
- RED `fuel002-go-red.jsonl` (`go test -json -count=1 ./internal/modules/fueling/...`;
  classifier unit test; `go test -json -race -tags=integration -count=1
  ./internal/modules/organization/adapters/ -run '^(TestFuelingIntake|TestFuelingSchema|TestFuelingPopulated|TestMeterPopulatedOrdinaryUpgrade)'`):
  29 top-level / 329 named nodes, 12 top-level and 74 named failing, all
  behavioural (stubs: parse/reconcile/classify unavailable, every signed
  fueling `orgsync: unavailable`, no concurrent acceptance). The 17 passing
  top-level tests are the unchanged FUEL-001 domain suite, the policy test and
  the two upgrade tests: the populated 000212 → 000215 upgrade ran against the
  migration as written (schema precedes behaviour; its injected failure is
  inside the test), and the METER-002 populated upgrade stays green on the
  extended chain. A first RED attempt hit a plpgsql syntax error (a `CASE …
  THEN` inside an `IF` condition) — fixed by parenthesizing, not counted as
  RED. Contract RED against the previous spec image:
  `fuel002-contract-red.jsonl`, 2/2 fail (owner missing; meter test expecting
  the second owner).
- GREEN focused `fuel002-go-green.jsonl` (same set, `-race`, plus the default
  runtime TLS test): 34 top-level / 392 named, 0 failures/skips; each
  integration test 1.3–2.2 s. Contract GREEN `fuel002-contract-green.jsonl`:
  5 top / 34 named.
- Affected race union (task command, `-tags=integration -race -count=1`, 60m):
  `fuel002-race-union.jsonl`, 24 packages, 731 top-level / 6659 named nodes,
  6653 pass, 6 fail, 832 s (organization adapters 813.7 s). The 6 failures are
  the cost-centre and operating-unit tests whose fixtures replace
  `org_cost_centre_current_usage`/`org_unit_current_usage` with their own
  copy of the allow-list (`installSyntheticCostCentreSource`, unit schema
  fixture): the new `org_fuelings_*_fkey` keys made those copies fail closed
  (P7804/P7704), exactly as designed. The fixtures now list the fueling keys
  like they list the 208 role-scope keys (no assertion changed); the six
  tests and their suites pass under race (`fuel002-usage-fixtures-rerun.jsonl`,
  25 top / 33 named). Final organization-adapters race rerun:
  `go test -json -race -tags=integration -timeout=60m
  ./internal/modules/organization/adapters -count=1` on the final code →
  `fuel002-race-orgadapters-final.jsonl`, exit 0, 414 top-level / 3998 named
  nodes, 0 failures/skips, 664.4 s. The other 23 union packages had already
  passed with the final production code (the fixture edit touches only
  organization-adapter tests). No `cmd/worker` or directory/quality package
  is in the union, so no Rust-preparer environment exclusion was needed; the
  untagged run below passed everywhere.
- Untagged `go test -count=1 -timeout=45m ./...` after the last code edit:
  214 packages ok, exit 0, 72 s (`fuel002-untagged-all.log`).
- `go build ./... && go vet ./... && go vet -tags=integration ./...` pass and
  toolchain `gofmt -l` is empty (`fuel002-build-vet.log`); staticcheck
  (GOTOOLCHAIN go1.27.1) plain and `-tags=integration` on fueling, fleet,
  organization adapters, enterprisesuite, apicontract and orgtest: no findings
  after removing the now-unused `meterCoverageDisposition` wrapper
  (`fuel002-staticcheck.log`); `sqlc vet` passes and `sqlc generate`
  reproduces every existing package with no diff (`fuel002-sqlc.log`);
  vacuum with the repository rules: 0 errors (warnings/informs unchanged from
  the previous spec, 953/615; `fuel002-vacuum.log`); `git diff --check`
  clean; `check_plan.py` 348 tasks / 545 rules / 167 use cases, 0 problems.
- Coverage: unit `fuel002-cover-unit.out` — fueling/domain 91.3 %
  (`arithmetic.go` 85.7 %), fueling/application 84.1 %; integration
  `fuel002-cover-integration.out` — fueling/adapters 76.3 %,
  `fueling_authority.go` 75.5 %, generalised `historyDisposition` 80.8 %;
  uncovered lines are unavailable/invalid-evidence refusals.
- Secret/PII scan of the staged diff: one hit, the word "tokens" in a code
  comment about unit tokens; only synthetic identifiers and Portuguese
  synthetic labels; no credential, real name, CNPJ or CPF.
- No route, authorization matrix, inventory, Android, Kotlin, shared vector,
  job or worker change; no emulator/device; no Rust preparer test touched.

EMP-FUEL-002 core (persistence, signed handler, embedded reading, exact
replay, arithmetic, authority at receipt, RLS, concurrency and upgrade) is
LOCAL_DONE / INTEGRATION_PENDING on real PostGIS. Held fuelings, receipt
links, panel reads and the expense projection are retained above; no
F3/C1/M3 acceptance, deployment or release is claimed.
