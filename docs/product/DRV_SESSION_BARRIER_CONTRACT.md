# Proposed driver session and GPS invalidation barrier

Status: PROPOSED / NOT IMPLEMENTED. This contract describes a candidate for
the open DRV-18 and DRV-21 owner/lifecycle races. It does not change existing
authentication, authorize an operational rollout or certify the driver suite.
It applies B-BR-DRV-07, 09, 10 and 18 and BUC-DRV-03 from the
[driver suite contract](DRIVER_SUITE_CONTRACT.md).

## Behavior boundary

- Each admitted driver operation binds to an explicit origin, owner and local
  session generation. Guest data retains its existing separate ownership and
  adoption rules; an authenticated account must not inherit it implicitly.
  A second account cannot read, write or receive the first account’s data.
- Logout, account/key/origin replacement and any existing authorization loss
  that requires GPS to stop invalidate the affected generation. A later login
  to the same account obtains a new generation; an in-flight operation from
  the earlier session must not become valid again. A routine credential refresh
  for the same authorized identity must preserve existing auth behavior. The
  exact identity/key and entitlement events that require invalidation must be
  reconciled with the existing auth and Premium contracts before implementation;
  this document adds no new entitlement or expiry policy.
- A driver write may commit before invalidation wins the database transaction.
  Once invalidation commits, no write carrying the old generation may commit.
  This is a defined ordering boundary, not a claim that already committed writes
  can be revoked or that a concurrent operation is retroactively cancelled.
- GPS collection uses the initial owner and generation. After the in-memory
  invalidation signal, callbacks must reject fixes for that generation, even if
  Android has not yet delivered listener removal. The service also receives an
  explicit stop signal without waiting for a location callback. Listener removal
  runs on the owning Android thread; its completion is asynchronous, so no
  instantaneous physical shutdown is claimed. No GPS collection restarts on
  a later login or process restart without a separate explicit user action.
- The last GPS checkpoint stays incomplete when its generation is invalidated
  or its final write cannot be admitted. A new owner never receives an earlier
  owner’s aggregate. Raw coordinates are excluded from persistence and logs.

## Candidate design and ordering

The existing `AuthSessionStore` is synchronous and backed by encrypted
SharedPreferences and Android Keystore, while driver data lives in Room.
Those stores cannot participate in one atomic transaction. A local Room marker
may hold only the origin, owner, generation and admission state; it must never
contain a credential, token or location. Generation values are opaque and must
not be reused. An unknown, inconsistent or interrupted state is closed to
driver writes until secure session rehydration establishes an authorized state.

1. Driver operations capture an admitted generation; each mutation checks the
   exact marker and applies its changes in the same Room transaction. This must
   cover all driver datasets and associated commands, not just shift updates.
   A stale generation cannot publish either the parent or its dependent rows.
2. A session transition first fences the old generation and publishes the
   in-memory GPS invalidation signal, then persists its Room revocation. The
   order and failure behavior of the auth operation itself must preserve the
   existing key custody, rollback and offline logout semantics. No network call
   or secure-store I/O is performed inside a Room transaction.
3. Only after the resulting secure session has been durably written and read
   back may a matching new marker become admitted. If the transition or
   read-back fails, no unverified generation is opened. Restoration of a prior
   session after an auth failure must follow the existing auth contract and, if
   permitted, use a freshly verified generation rather than resurrect a stale
   one. Explicit logout must retain the current offline logout behavior.
4. At process start, admission stays closed until the persisted marker and the
   secure session are reconciled. A crash after fencing, after secure-store
   mutation or before admission cannot silently authorize a mismatched owner.
   Any retry or recovery must be idempotent and leave existing driver records
   intact. It must not perform an implicit guest adoption.

Room must serialize the marker check and each data mutation against marker
revocation. A bounded non-suspending transaction body can run on an IO executor;
its SQLite lock ends at commit or rollback. Do not block the main thread, hold
an application mutex across suspension, wait for network inside a transaction,
or acquire locks in conflicting order with `SerializedAuthOperations`. The
current synchronous auth port and existing call sites require an explicit
coordination design before any conversion to asynchronous operations; a
`runBlocking` bridge on the main thread is not acceptable. This proposal does
not assert that current code already routes all writes or session changes
through such a coordinator. Process boundaries and any direct access to secure
preferences must be audited; a process-local signal alone cannot fence another
writer.

## Failure and recovery requirements

| Failure point | Required observable result |
| --- | --- |
| Marker revocation fails | No new identity may be admitted on the assumption that fencing succeeded; report/retry according to the existing auth contract. GPS must stop on the local signal. |
| Secure write, clear or read-back fails after fencing | Keep driver admission closed. Preserve the existing recovery-key and auth rollback obligations; never expose data as a different owner. |
| Process dies before, during or after secure-store mutation | Rehydration reconciles secure session and marker before admitting writes. An incomplete GPS checkpoint remains incomplete and GPS does not restart. |
| Driver transaction races a transition | Exactly one database ordering is observed: an old-generation transaction commits before revocation, or it is rejected/rolled back after revocation. No partial dependent rows. |
| GPS receives no further location callbacks | The session signal still requests listener removal. Until removal completes, callbacks reject the revoked generation. |
| GPS final checkpoint write fails or is fenced | The initial incomplete checkpoint remains available if it was durably created; no complete distance or successful final save is claimed. |
| Storage is unavailable during recovery | Keep admission closed and preserve existing data. Surface failure through existing error paths; do not invent a successful login, logout or deletion receipt. |

## Mandatory verification before implementation acceptance

- Real Room tests with separate connections and controlled barriers immediately
  before the driver transaction and marker revocation: both serial orderings,
  stale writes after commit, rollback of linked rows, and multiple simultaneous
  writers. Cover every dataset and any direct DAO entry point capable of
  bypassing admission.
- A→B, A→logout→A, guest→account, origin/key change, same-identity refresh,
  login failure and logout while offline. Assert owner isolation, no reuse of a
  revoked generation, no unintended guest adoption, and unchanged key custody
  and existing auth failure behavior.
- Fault injection at each Room and secure-store step, including write failure,
  failed read-back and rollback, followed by database reopen and process
  rehydration. Assert closed admission for ambiguous states, preserved data and
  correct recovery; never claim cross-store atomicity.
- GPS tests with actual service and Room plus a deterministic listener/clock
  seam: owner change with no location callback, callback queued during fencing,
  stale start, repeated stop, failed final write, process death and no automatic
  restart. Verify listener release, no accepted fix after the in-memory fence,
  unchanged owner and incomplete checkpoint. Existing MAP coexistence and
  physical battery/field proofs remain separate requirements.
- Auth regression and negative tests must cover every caller of session
  save/clear, identity refresh and key recovery; prove no main-thread blocking
  or lock-order deadlock. Run applicable migration, static, secret and privacy
  checks. This proposal by itself is not test evidence or release acceptance.
