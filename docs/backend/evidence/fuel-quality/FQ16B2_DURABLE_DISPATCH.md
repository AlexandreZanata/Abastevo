# FQ-16B2 — Durable report dispatch and local privacy lifecycle

Status: LOCAL_DONE
Validation: PASS

Parent 22730930, maintained dev. B-BR-FQ-07/14/16; BUC-FQ-05/08.
Wire the encrypted journal to signed reports, restore a frozen pending intent
in the private screen and dispatch through existing Hilt WorkManager with fresh
session/proof on every attempt. Include login-family generation in the opaque
owner scope. Explicit confirmation is the only enqueue trigger; queued is never
received. Limits/backoff remain the existing 25-attempt/hour-capped policy.

Key custody uses a separate owner-derived AndroidKeyStore AES alias. A 14→15
append-only key-erasure receipt prevents gate cleanup from forgetting unconfirmed
key deletion. Logout blocks/erases only this owner's queue before authority clears,
then deletes its keys; failures remain durably repairable. Expired commands are
ineligible after seven days; periodic bounded repair removes cipher keys and
metadata, with no extension of operational media retention or public promotion.
No private backup, raw credentials/proof, document or portal.

Acceptance: fresh family/session/proof, restart/reconnect same intent, quota/
backoff/attempt exhaustion, claimed/erased/expired/foreign owner, uncertain ack,
key failure/recovery/restore and actual SQLite migration/lease guards; affected
JVM, contracts, lint and APK compilation. Native execution remains OWED. No real
production certification or D scope follows.

## Delivered and verified, 2026-10-19

The production Hilt graph uses DurableQualityReports, AndroidKeyStore owner keys,
file-backed noBackupFilesDir erasure markers and existing WorkManager. Exact
confirmed commands survive restart encrypted; neither WorkManager input nor
SavedState contains the private text, credential, proof or claim. Current
account/device/origin/login-family is checked before and after signed I/O. A
retained offline session permits local queuing but never substitutes for the
fresh server session and exact-body proof required for dispatch.

Connected dispatch claims one current-owner command with a 120s nonce-fenced
lease. Attempts are precharged, capped at25; existing exponential/jitter policy
and bounded Retry-After are retained. Uncertain responses preserve the exact
command ID for backend replay. Terminal refusals remove the claimed intent;
auth/family changes park it, without dispatching another owner. Disabled intake
still permits deliberate withdrawal/appeal and private history through Profile.

Logout records an independent filesystem marker before Room cleanup; even if
Room fails, owner-prefix crypto-erasure is attempted. Markers survive restart,
including interrupted atomic rename, and prevent dispatch until repair. A new
Room14→15 key-erasure receipt keeps blocked/expired gates until key deletion is
confirmed. Periodic bounded repair runs offline and schedules connected dispatch;
it cannot erase foreign-owner keys. Backup/device-transfer exclusions cover the
existing database, and markers use Android no-backup storage. No dependencies
were added. Expired ciphertext is never eligible, even before physical cleanup.

The negative feature-flag variant exposed a public ViewModel bypass: manual fuel
selection and revision loading could read despite qualityUiEnabled=false. Both
entry methods now enforce the flag; private owner recovery stays accessible.

## Validation

- Six actual-SQLite cases PASS via
  `python3 scripts/tests/test-quality-outbox-sql.py`: production queries and
  schema13→14→15/fresh15 equality, unchanged prior ciphertext, receipt/prune
  fencing, twelve-connection exclusive claim, replay/expiry/clock/foreign owner.
- 29 data JVM cases PASS: DurableQualityReports7, filesystem erasure markers3,
  signed HTTP8, journal4, seal2 and OwnerLocalErasure5. Covers Room outage during
  logout, key deletion failure/restart repair, logout during response, family
  refresh replacement, retained offline authority, quotas/budget and exact replay.
- 39 app JVM cases PASS: private reports9, public quality4, AuthViewModel26.
  Explicit confirmations, restored frozen intent, late responses, account switch
  and own-history route without fabricated station are covered.
- Feature-off variant PASS (one combined case):
  `./gradlew --offline -PqualityUiEnabled=false -PqualityOutboxEnabled=false
  -PqualityCacheEnabled=false :app:testDebugUnitTest --tests '*QualityFlagsTest'`.
  No public source I/O; own history/withdrawal remain reachable and confirmation
  is still mandatory.
- Default configuration data/app Android-test APK, app APK and lint PASS.
  Four quality data native cases and four app quality UI native cases are
  COMPILED ONLY; device/Room/KeyStore execution remains OWED under the existing
  native-test deferral. Gradle deprecation notices are not acceptance failures.
- Current affected validation logs: `/tmp/fq16b2-20261019-validation.log`,
  `/tmp/fq16b2-20261019-flags.log`, `/tmp/fq16b2-20261019-defaults.log`.
  The first confirms matching cached successful tests from2026-10-18; the
  flags/default configuration inputs were rebuilt on2026-10-19. No source change
  after those test inputs. Whitespace, scoped secret/backup and document links
  reviewed. No device, deployment, import activation, remote merge or D work.

## Remaining scope and recovery

Disable qualityOutboxEnabled to pause automatic dispatch/intake; retain repair,
private owner reads and explicit recovery actions. Disable the public cache/UI
separately. Never reset the database or delete foreign live material. Attempt
exhaustion remains parked until expiry; no automatic budget reset is introduced.
Only one pending intent per owner/family is admitted; queued is never presented
as an acknowledged report. Native transaction/key custody behavior still needs
its deferred device acceptance.

Next: verify actual enforcement/fine resource contracts and complete their live
source streams, linked FQ-14 journeys and FQ-17 scoped acceptance/final protected
delivery. Adapter-only synthetic fixtures do not establish full source coverage.
D and real-production certification remain explicitly held.
