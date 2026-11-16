# EMP-OFFL-011 — Cleanup of revoked sessions and devices on next contact

Date: 2026-11-16.
Status: LOCAL_DONE
Validation: PARTIAL — host RED/GREEN, Kotlin suites, lint, builds and the enterprise validation build PASS; native acceptance on `emulator-5588` OWED (see below)
Integration: INTEGRATION_PENDING

Bounded F4 Android task (module OFFL, phase F4, P1, type APP, size S) on
`codex/enterprise-f4-offl011` (worktree `/data/dev/worktrees/f4-offl011`),
opened from the F4 integration head `78de91be`. Synthetic data only. No Go,
SQL, migration, route, OpenAPI, permission or wire-format change: both
revocation classes were already observable by the device (EMP-PRIV-003 signed
contact verdicts, EMP-OFFL-009 late-intake codes). No Android device or
emulator was addressed: `emulator-5588` is `RUNNING_LOCKED` behind a synthetic
credential (`f4-logs/EMULATOR_5588_LOCKED_AFTER_REBOOT.md`) and the physical
phone is never used. Logs: `/data/dev/worktrees/f4-logs/offl011-*`.

Sources read: the EMP-OFFL-011 block (EP04) and the OFFL-003, OFFL-009,
OFFL-010 and OFFL-012 blocks; [offline sync](../10-offline-sync.md) §§3–5,
8, 10.2–10.4 and 11 (BR-OFFL-011..014, UC-OFFL-06/07, SCR-OFFL-01/04);
[identity](../modules/identity.md) §§4–5 (BR-IDN-029, BR-IDN-037..039,
UC-IDN-08/09/16); [privacy and security](../11-privacy-security.md) §§4–6
(BR-PRIV-016..018, UC-PRIV-03, the local partition states); EDR-068..070,
EDR-122, EDR-197..201, EDR-268 and the latest rows; the evidence of
[PRIV-003 V1](f1-priv003-protected-device-store.md) and its
[F2 completion](f2-priv003-protection-completion.md),
[IDN-015](f1-idn015-device-registry.md),
[OFFL-003](f2-offl003-protected-operation-queue.md),
[OFFL-009](f4-offl009-post-revocation-window.md),
[OFFL-012](f4-offl012-org-state-queued-operations.md),
[OFFL-005](f4-offl005-attachment-upload.md) and the
[sync-context fix](f4-fix-sync-context-intermittent.md); the server contact
source (`membership/adapters/device_contact.go`) and the signed intake route's
late path (`orgsync/adapters/http/http.go`).

IDs used: B-BR-EMP-536..539, BUC-EMP-275..276, EDR-348..350 (all of them; none
left unused). Migration `000260`: **unused** (no persistent server state
changes).

## Observed before any change

- The device learned a revocation only when the person selected the company:
  `ProtectedEnterprisePersistence.confirm` applied ERASE (erase) and LOCK
  (`requireContactLocked`, the PRIV-003 reconfirmation wait). The operation
  worker never asked the contact: a delivery refused as a whole (`403
  DEVICE_REVOKED` for a lost key) became `EnterpriseQueueDeliveryFailure` and
  was retried with backoff forever — a lost phone kept sending its queue.
- After a LOCK the `lock_` reconfirmation marker refused every access,
  including delivery, and an ended member can no longer select the company to
  reconfirm: eligible pre-end work (EMP-OFFL-009 window) stayed undelivered
  until the 30-day offline validity erased it with no message — the "eligible
  intake discarded silently" negative.
- The erasure itself (EMP-PRIV-003) already persists its intent before removing
  keys and files, refuses every read once the intent exists, reports
  `cleanupRequired` until verified and is retried by maintenance.
- The server already distinguishes the classes on the exact contact (`ERASE`
  for `LOST_OR_COMPROMISED`, `LOCK` for `ACCESS_ENDED`), takes the late path for
  an `ACCESS_ENDED` sealed period, and answers a lost key with a whole-response
  `403`.

## Rules frozen before behaviour

B-BR-EMP-536..539 and BUC-EMP-275/276 are the
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section
"EMP-OFFL-011 — Cleanup of revoked sessions and devices on next contact";
EDR-348..350 are in [16-decisions](../16-decisions.md). Both were appended
before any behaviour. In short: the class comes only from the exact signed
contact; delivery refusals and late codes are hints for one contact; cleanup
touches only the named partition; ACCESS_ENDED locks every use but intake-only
delivery and erases once nothing is unsent or the window closed; LOST erases at
once and never sends again; cleanup is resumable and `CLEANED` is said only
after verified disposal.

## Shape

- **Domain** `domain/.../enterprise/sync/RevocationCleanup.kt`:
  `EnterpriseRevocationClass`, `RevocationQueueFacts`, `RevocationCleanupAction`,
  `RevocationCleanupState` and the pure `RevocationCleanup` (hint codes and
  statuses, queue facts, decision, reportable state).
- **Application**: `EnterpriseAccessEndedLocked` (a subclass of the now open
  `EnterpriseLocalDenied`, so every existing denial path still applies);
  `EnterpriseContextState.accessEndedLocked`; `EnterpriseQueueDeliveryFailure.revocationHint`;
  `EnterpriseQueueDispatchResult.revocationHint`; `EnterpriseRevocationOutcome`.
- **Data**: the `ended_` Keystore marker name; `ProtectedEnterprisePersistence.revocationCleanup`
  and `QueueUse`; the runner's `cleanup` hook and `revocation*` counts.
- Stubs were `TODO()` (domain, `revocationCleanup`) or inert (runner hook not
  called, no hint, LOCK unchanged).

## RED

`offl011-kotlin-red.log`, XML in `offl011-kotlin-red-xml/`, summary
`offl011-kotlin-red-summary.txt`. 15 new tests, 15 failing, every failure
behavioural:

| Class | Tests | Failed | Why |
|---|---|---|---|
| domain `RevocationCleanupTest` | 6 | 6 | `NotImplementedError` from the stubs |
| application `EnterpriseOperationQueueTest` | 16 | 1 (new) | dispatch reported no hint for a late code |
| application `EnterpriseProtectedContextFlowTest` | 6 | 1 (new) | LOCK not presented (`accessEndedLocked` false) |
| data `EnterpriseOperationHttpClientTest` | 11 | 1 (new) | 403 carried no hint |
| data `EnterpriseRevocationCleanupTest` | 6 | 6 | no contact, no lock, no erasure; LOCK not typed |

The 25 pre-existing tests of those classes passed against the stubs.

## GREEN and REFACTOR

The first GREEN run (`offl011-kotlin-green-1.log`) passed the 15 new tests and
the unchanged `EnterpriseVaultContactBindingTest` (6, including
`sourceAccessEndedLocksAndPreservesUntilSameEnrollmentActiveProof`) and
compiled the app. No expectation changed between RED and GREEN.

- **Pure rule** (`RevocationCleanup`, EDR-350): LOST and offline expiry erase;
  ACCESS_ENDED delivers while anything is unsent and the window is open, and
  refuses to decide without the queue's own facts; `CLEANED` only with a
  verified erasure; a delivering partition can never be reported erased.
- **Hints** (EDR-348): `EnterpriseOperationHttpClient` marks a whole-response
  403/404 (after its no-store/no-cookie/JSON checks) as a hint;
  `EnterpriseQueueService.dispatch` reports a hint for a transport hint or a
  late-intake code; queue state is unchanged by either.
- **Lock** (EDR-349): `endAccessLocked` writes the opaque Keystore marker
  `abastevo_enterprise_v2_ended_<token>` (accepted by the actual provider's
  alias pattern, discovered by token like the other markers, erased by the
  existing erasure with the partition). `ProtectedEnterprisePersistence` adds
  `QueueUse`: only queue `pending`, `entry`, `acknowledge` and `retry` are
  delivery; records, feature reads/writes, saves, status, review lookups and
  attachments throw `EnterpriseAccessEndedLocked`. The selection's LOCK now
  writes this marker and throws the typed denial instead of the PRIV-003
  reconfirmation wait; an exact ACTIVE still removes it.
- **Worker** (`revocationCleanup` and the runner's `cleanup` hook): after each
  partition's delivery, one probe under the vault lock settles an existing lock
  (lease expiry still erases first; a reconfirmation wait claims nothing) or,
  on a hint, asks the exact contact with the retained enrollment outside every
  lock and applies only an exact ERASE/LOCK for the same enrollment, revision
  and family; a locked partition gets no review lookup or upload; a lock or
  erasure that withdrew the worker's own lease ends the invocation with an
  immediate continuation (no failure, no backoff).
- **App**: the context flow maps `EnterpriseAccessEndedLocked` to
  `accessEndedLocked`; the context screen shows
  `enterprise_protection_access_ended` (pt-BR default and seven locales)
  instead of the generic denial.
- **REFACTOR**: the marker helper takes no fencing flag (only the worker's new
  lock fences, after settling); one `cleanupLocked` path asks the pure rule for
  every class. Rerun: the full suites below.

## Named negatives mapped to tests

| Negative (backlog) | Test(s) |
|---|---|
| Foreign partition erased | data `accessEndedDeliversEligibleIntakeThenErasesOnlyItsOwnPartition` (another organization of the same account and another account's partition keep records and keys; the other organization keeps its review lookups and uploads), `aLostDeviceErasesOnlyThatPartitionAndNeverSendsItsWorkAgain`, `anInterruptedCleanup…` (the other partition survives maintenance); device `accessEndedDeliversThenErasesOnlyItsPartitionOnTheRealProvider` (compiled, OWED) |
| Lost device still sends work | data `aLostDeviceErasesOnlyThatPartitionAndNeverSendsItsWorkAgain` (one refused delivery, one contact, ERASE, then three later invocations send nothing), `aClosedIntakeWindowErasesAndTheRemainingWorkIsNeverSentAgain` (no send after the window closed); device `anInterruptedErasure…` (no send while the erasure is pending) |
| Interrupted cleanup exposes data | data `anInterruptedCleanupStaysDeniedAcrossRestartAndCompletesBeforeClaimingRemoval` (a failing key deletion: `CLEANUP_PENDING`, not `CLEANED`; denied in this and a restarted process; not discovered; maintenance finishes; only then `cleanupRequired` false), `aLockAtSelection…AcrossRestart` (a LOCK survives a restart before its cleanup: still locked, still delivering); device `anInterruptedErasureStaysDeniedAndMaintenanceFinishesItOnTheRealProvider` |
| Offline cleanup overclaim | domain `offlineCleanupOverclaimRemovedIsSaidOnlyAfterVerifiedDisposal`; data `offlineGenericRefusalsActiveAndForeignVerdictsNeverEraseLockOrClaim` (disconnected: no contact, no state; refused, outage, ACTIVE, another enrollment's or family's verdict: no change, no state); application `an access ended verdict is presented as locked without claiming removal…` |
| Eligible intake discarded silently | domain `eligibleIntakeIsNeverDiscardedSilentlyWhileTheWindowIsOpen`; data `accessEndedDeliversEligibleIntakeThenErasesOnlyItsOwnPartition` (a `RETRY UNAVAILABLE` operation stays pending under the lock and is sent again after a restart; erasure only after its final answer) |

Steps of the block: contact (data tests 1–4), offline expiry (`settleEndedLocked`
keeps the PRIV-003 expiry erase; domain `OFFLINE_EXPIRED` row), restart during
cleanup (data tests 5–6), separate classes (LOCK versus ERASE tests).

## Validation

All Gradle runs: JDK 17, offline, no daemon, two workers, under
`flock /data/dev/worktrees/.f4-gradle.lock` (`offl011-scripts/gradle.sh`).

```sh
gradle.sh --continue :domain:test --tests 'com.anpfuel.domain.enterprise.sync.RevocationCleanupTest' \
  :application:test --tests '…EnterpriseOperationQueueTest' --tests '…EnterpriseProtectedContextFlowTest' \
  :data:testDebugUnitTest --tests '…EnterpriseRevocationCleanupTest' --tests '…EnterpriseOperationHttpClientTest'
    # RED 15/15 new failing (offl011-kotlin-red.log); GREEN with EnterpriseVaultContactBindingTest
    # and :app:compileDebugKotlin (offl011-kotlin-green-1.log)
gradle.sh --continue :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test \
  :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :data:lintDebug :app:assembleDebug
    # pass, 6m17s (offl011-kotlin-suites-lint.log): domain enterprise 4585, application 723
    # (enterprise 170), data 980 (enterprise 213; 4 pre-existing live/interop skips), app 827
    # (enterprise 31); 0 failures or errors. Lint 0 errors (app 354 warnings, data 17, as at the
    # head; none in a touched file)
gradle.sh -PenterpriseDeviceValidation=true :app:assembleDebug :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest
    # pass, 2m13s (offl011-validation-build.log); hashes offl011-validation-apks.sha256; the data
    # test APK contains EnterpriseRevocationCleanupDeviceTest and both methods
python3 scripts/tests/test-enterprise-protection.py      # 4 passed
python3 scripts/tests/test-enterprise-local-sql.py       # 5 passed
python3 docs/enterprise/tools/check_plan.py              # 348 tasks / 545 rules / 167 use cases / 145 screens, 0 problems
python3 docs/enterprise/tools/test_check_plan.py         # 3 passed
git diff --check                                          # clean
git diff | gitleaks stdin --redact --no-banner            # no leaks
```

- No Go, SQL, migration, route, OpenAPI, permission or wire change: the Go,
  sqlc, PostGIS, route-inventory, authorization-matrix and AUTH-011 gates do
  not apply and were not run (inventory 165 / walk 165/33/132 / matrix 300 /
  20 signed owners unchanged).
- Secret/PII review of the staged diff: synthetic UUIDs, example origins and
  synthetic payloads only; no credential, real name, CNPJ, CPF, plate,
  location or receipt.

## Native acceptance — OWED

`EnterpriseRevocationCleanupDeviceTest` (data androidTest, 2 methods: the real
AndroidKeyStore accepts and discovers the `ended_` marker; LOCK delivery then
erasure of exactly one partition; an interrupted erasure stays denied and
maintenance finishes it) is compiled into the data test APK. It is not run:
`emulator-5588` is `RUNNING_LOCKED` and this agent must not unlock it. Driver
(one line, under the emulator lock, refuses a locked user):
`/data/dev/worktrees/f4-logs/offl011-scripts/native-driver.sh` after
`gradle.sh :data:assembleDebugAndroidTest`. The real signed contact against
PostGIS stays EMP-PRIV-003's native test; no fixture-server change was needed.

## Retained (not part of this task's acceptance)

- **Reboot or clock rollback before or during intake-only delivery** keeps
  EMP-PRIV-003's person-confirmed reconfirmation (EDR-122): an ended member
  cannot select the company, so that partition stays locked (never readable)
  until its 30-day offline validity expires and erases it. Re-anchoring an
  intake-only partition from a contact's server time without extending its
  validity is a follow-up with its own EDR (EDR-350).
- The offline validity is never extended by a LOCK: with an intake window
  longer than the time left on the device's 30-day validity, undelivered work
  is erased at local expiry (BR-PRIV-018 wins).
- Attachment bytes not yet stored when an ended partition is erased are
  erased with it: the server has no late attachment path (EDR-349).
- A membership **suspension** (enrollment still `ENROLLED`) is not a
  revocation class: the contact refuses generically and nothing changes; its
  device lock (UC-IDN-08) is not decided here (EMP-OFFL-009 retained it too).
- The pending screen of a locked partition keeps its existing "unavailable"
  text; a per-partition "locked, N still sending" view on that screen, a
  notification and the manager-side "unsent records on that phone are lost"
  message (UC-PRIV-03, panel) belong to their owners.
- Native acceptance above; F4/M4 acceptance, production activation and release.
