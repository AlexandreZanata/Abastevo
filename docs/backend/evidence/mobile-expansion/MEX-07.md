# MEX-07 — Atomic hosted vehicle capability admission

Status: LOCAL_DONE
Validation: PASS
Parent: 9f05bdc1. Date: 2026-10-21.

## Before behavior

B-BR-MEX-01/02/06 and BUC-MEX-01/04; registration v2 is authoritative.
Reuse subscription/cloudsync and their owner transaction lock, signed session,
device/consent/epoch, revisions, receipts, erasure and restore boundaries.
Grant `vehicles.multiple` only from the existing verified Premium evaluator;
all existing capabilities keep their meaning and sales/provider gates stay OFF.
The first hosted registration still needs cloud.sync; the second/third also
need current vehicles.multiple, with the same three-ID ceiling (archived IDs
count). Edits/replays of retained IDs do not consume a slot. All writes and
receipts roll back if capability expires/fails before commit. Missing ports
fail closed. Preview cannot grant authority; creation/import/restore recheck
inside their owning transactions. No migration or second billing service.

Acceptance: pure capability boundaries plus actual PostGIS single/12-writer
admission, replay/conflicts, same/foreign owner, fourth slot, whole import and
restore rejection, legacy correction, live proof expiry/revocation, rollback
and restart. Existing full cloud/subscription, HTTP, contract, compilation,
vet/static, formatting, diff and secret checks remain required. Device/local
quota is MEX-08 and depends on EE-19; this task does not bypass it.

Isolation: existing clean codex/mobile-expansion-plan, maintained dev occupied
by the separately reviewed feed delivery. Both histories and other drafts are
preserved. No PR/main merge/deploy/wiki action belongs to these construction
microtasks. Global PR #140 Go vulnerability gate remains a separate unresolved
integration obligation, not a passed scan from this source checkpoint.

## Delivered and actual acceptance

- Reused subscription evaluator, existing cloud.sync gate and owner lock;
  added only the frozen vehicles.multiple capability and a separate server
  transaction port. Unknown/missing extra-slot authority fails closed.
- Central pure 1-free/3-ceiling rule; direct legacy put, Apply/import and
  RestoreCommit share admission and final capability recheck. Identical-ID
  replay/edit remains distinct from creation. Import preview does not count
  repeated identical operations twice; restore preview revalidates after
  external object reads and explicitly reports quota/capability incompatibility.
- Existing schema, IDs, archived rows, paid/free matrix and signed API/epoch/
  consent/revision/erasure checks retained. Foundation PutVehicle remains an
  internal compatibility port (no new HTTP route); public writes still use
  the existing signed/device/epoch/consent Apply or RestoreCommit protocol.
- RED: existing evaluator omitted vehicles.multiple; real SQL admitted an
  additional cloud-only vehicle and a two-ID import without the capability.
  All three failed before implementation; GREEN afterward. An initial compile
  attempt exposed a local variable collision, corrected without weakening tests.
- New pure rule/boundary tests plus seven actual PostGIS cases (with three
  additional path subcases) PASS under race detector: 12 writers admit exactly
  one without extra-slot capability; replay/conflicting operation and foreign
  session checks; archived legacy4 retained/editable; imported batch rollback;
  stale restore preview denial; duplicate preview/replay counts; actual SQL
  proof expiry at the final admission check rolls back direct/batch/restore
  effects and receipts, followed by successful retry.
- Full scoped matrix PASS (cloud adapters 39.334s, subscription adapters
  16.878s, API composition 8.826s), including existing Premium three-slot race,
  free data exit, devices/revocation, consent/epoch, restore/erasure/cancellation:

  ```sh
  ANPFUEL_TEST_DATABASE_URL=<documented loopback test DSN> go test -race -count=1 -tags=integration ./internal/modules/cloudsync/... ./internal/modules/subscription/... ./internal/platform/apicontract ./cmd/api
  ```

  Fresh disposable databases were created by the existing integration helper;
  no persistent application/VPS database was selected or changed.
- Actual OpenAPI valid seven-capability and rejected invented/unlimited
  capability vectors plus final full apicontract race suite PASS. Spec lint
  has zero errors (pre-existing warnings retained). `go vet`, build and pinned
  `GOTOOLCHAIN=go1.27.1 staticcheck` PASS for affected modules/API/contracts.
- MEX-06 review: source/commit and measured-host-only claim checked; 13 location
  tests actually re-executed PASS (no skips). Native latency remains OWED.
- `git diff --check`, gofmt and tracked/changed-surface secret checks PASS;
  no dependency/migration change. Machine amendment remains jointly runtime-
  pending until local MEX-08; source proof does not activate sales/provider.

Recovery: leave the additional-slot port unset to stop new extra registrations,
retain held records and free read/export/erasure. No data reset/down migration.
Integration: INTEGRATION_PENDING. Commercial/provider/live rollout and local
quota/lease/native acceptance are separate. The previously detected global
Go1.27.2 vulnerability gate is still CI_BLOCKED; no new clean scan is claimed.
Next ready MEX unit is MEX-09; MEX-08 remains dependent on EE-19.
