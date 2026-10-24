# MEX-15 — Conflict resolution, backup controls and restore preview

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `52c994b3` (maintained dev, MEX-14). Date: 2026-10-23.
Depends MEX-14; owns PRM09-T03/T04/T08 and reuses PRM04.
Rules B-BR-MEX-02/05/06; BUC-MEX-04. Risk: auth/jobs/privacy/data loss.
Task tracking: local MEX-15; issue/PR unchanged, remote checks NOT_RUN.
User scope: one local atomic commit; no push, PR creation or merge.

Before behavior: owner-scoped conflicts require an explicit keep-mine,
take-server or cancel choice. Connectivity failures never imply a server
record was deleted. Device and dataset participation can be paused without
removing local work. Only actual non-delete receipts update the last-ack
status; pending local edits are never advertised as backed up. Restore
previews are read-only, owner/origin-bound, bounded and expire after five
minutes. Confirmation rechecks source revisions/payloads, account, local
changes, participation, held commands and erasures. Existing local work is
retained; replacement uses explicit conflict resolution. Process death or
cancel loses staged authority. Local apply runs in a Room transaction;
corrupt items are reported individually and persistence failures roll back.

## Delivered

- Fixed the records gateway implementation/binding and migration placement.
  Private records POST uses live proof, no-store, bounded pages and a 1 MiB
  response budget; canonical typed payloads, UUIDs, revisions, dataset,
  vehicle and cursors are validated. Denials/outages/cancellation do not
  become successful empty pages. No backend, OpenAPI or contract changes.
- Additive Room 24→25 `ledger_erased` table with first-erasure-wins inserts,
  owner/origin partitioning and explicit owner cleanup. Local deletion and
  remote delete acknowledgement retain erasure markers before removing work.
- Explicit conflict keep/take/cancel, including held deletes without a
  draft. Lookup traverses bounded pages; incomplete/offline reads preserve
  work. Missing keep-mine content remains held as NoContent. Resolution
  checks local/account movement and refuses active worker claims; Hilt
  binds local draft/outbox changes to one Room transaction.
- Restore imports only absent records, preserving existing local drafts
  and all queued/failed/conflicted/in-flight intents, including legacy
  deletes created before the erasure table existed. Up to five 100-row
  pages per enabled dataset; truncated previews cannot be confirmed.
  Fresh source bodies/revisions and local/account/scope state are checked
  before transaction writes. Erased/voided rows never become live imports;
  identical confirmation is idempotent, corrupt items are reported, disk
  failure rolls back. No preview persistence or automatic restore.
- Dataset/device controls gate dispatch, including requested command IDs
  and account/participation changes while a claim suspends. A paused claimed
  intent retains its bounded lease for ordinary expiry/recovery.
  Status counts pending commands by exact dataset and shows up to 50 held
  conflict records. Atomic preference edits store receipt revision/time
  together; deletes never advertise a backup acknowledgement.
- Account card, `ledger-sync` route, ViewModel and lazy Compose screen:
  explicit participation, device/dataset pause, last-ack/pending/conflict
  status, keep/take/cancel, restore diff and confirm/cancel, empty/offline/
  stale/partial/error states. Late owner/generation results cannot render
  private previews. Revocation cancels existing work; this screen schedules
  no uploads. 27 strings in all eight existing locales. No new dependency.

## Actual checks

Tested source: parent `52c994b3` plus this task; code/test/script fingerprint
(SHA-256 over sorted task paths + file content hashes, excluding docs and
exported schemas): `ce20b000acdac84103f10dcab7ecbb3bedbb2bcf2677594fc1ef4847649c5a1e`.

- RED: offline take-server deleted local work, cross-account preview wrote
  into the next account, and requested-ID dispatch bypassed dataset revoke.
  All three regressions became GREEN. Legacy-delete guard first failed
  compilation against the missing explicit command-store port, then passed.
- Application coverage includes keep/take/cancel, no-content/corrupt refusal,
  paginated lookup, delete conflicts, account switch during I/O, read-only
  preview, expiry/truncation/revoke, edits/deletes queued after preview,
  erase-after-preview, idempotency, per-item decode and transaction failure.
- ViewModel coverage includes opt-in/dataset/device gates, all conflict
  choices, preview/confirm/double tap, stale abort, cancel/re-created model
  losing stage, late owner response isolation, offline/empty/partial states.
- MockWebServer covers private wire shape, opaque cursor, malformed/foreign/
  missing/negative revisions and payloads, 4xx/5xx, over-budget and actual
  response-body cancellation. Throttling is attached by a dispatcher after
  request parsing, so cancellation tests the response rather than a stalled
  request body. DAO/preference units cover migration SQL, partitioning,
  first epoch, defaults, persistence, exact counts and active-claim refusal.
- `./gradlew :domain:test :application:test --offline --rerun-tasks`: PASS.
- `./gradlew :data:testDebugUnitTest :app:testDebugUnitTest --offline --rerun-tasks`: PASS.
- Post-claim dispatch fences were also reproduced RED without the guard:
  dataset revoke/account switch during a suspended claim cannot submit an
  old command/proof. Final full checks above include the restored guard.
- Final XML census: domain 672 + application 448 + data 530 +
  app 443 = **2093 tests, 0 failures, 0 errors, 2 pre-existing skips**
  (2091 executed). The skips are the explicitly opt-in live ANP listing
  and actual training-consent Go/PostgreSQL interop tests; no new skips.
- `python3 scripts/tests/test_mex15_local_sql.py`: **6 tests PASS**, actual
  SQLite migration upgrade/replay, private-row preservation, first erasure
  epoch, exact owner/dataset SQL, failure rollback, legacy held deletes and
  two-connection worker/resolution serialization. Requires schema 25 exported
  by the preceding rerun build; generated JSON is omitted from Git.
- `./gradlew :app:lintDebug :data:lintDebug --offline`: PASS, zero errors;
  no lint findings in the new sync files. Existing warnings remain.
- `./gradlew :app:assembleDebug --offline`: PASS, actual Hilt/Room graph and
  debug APK built. Native Room upgrade execution is separately deferred.
- Eight-locale key/XML check: 27 matching keys each, no duplicates/missing.
  Python compilation, `git diff --check` and `bash scripts/scan-secrets.sh`:
  PASS. New source/tests/docs reviewed for secrets/PII/precise GPS; only
  synthetic identifiers/proof placeholders. Generated Room 25.json removed;
  the two unrelated backend formatting changes were restored, not committed.

## Limits and deliberate adjustments

- Safer restore behavior replaces the unfinished implementation that could
  overwrite local drafts: existing local records are retained, and explicit
  conflict resolution owns replacement. Five-minute preview expiry,
  transaction guards and legacy pending-delete checks close demonstrated or
  identified data-loss windows without changing the backend contract.
- SQLite and re-created ViewModel tests are local source evidence, not a
  physical Android process-death/Room-upgrade execution. Native/device runs
  remain OWED under the standing deferral.
- Two-device/reconnect, live signed staging proof and private-object deletion
  receipts remain integration obligations inherited from MEX-14/the plan.
  A last receipt for one command is not a complete backup certificate.
  No runtime source-epoch/distributed-race or production certification is
  claimed by local previews. Backend and private storage were not mutated.
- LOCAL_DONE/PASS is this bounded Android source checkpoint only;
  integration/release/deployment/wiki status is unchanged. No push, PR,
  merge or provider/sales activation occurred.

Next: MEX-16 Premium status/acquisition boundaries from this local checkpoint,
within its separately bounded scope; native/staging and final protected batch
integration remain pending. No remote wait is needed for local construction.
