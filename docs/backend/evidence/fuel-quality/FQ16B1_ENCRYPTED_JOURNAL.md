# FQ-16B1 — Durable encrypted owner intent journal

Status: LOCAL_DONE
Validation: PASS

Parent c973ae93, maintained dev. B-BR-FQ-07/14/16; BUC-FQ-05/08.
Use existing Room with additive 13→14 migration, no new dependency. One frozen
intent per origin/account/device/login-family scope. Store only encrypted typed
command and hash; never credentials, proofs, signed nonce or media. Family digest
is an authority-generation fence, not an account identifier shown to users.

A durable owner gate blocks new freeze/claim and stale acknowledgement after
logout/erasure; later login has a new family scope. Gates and queued rows have
seven-day operational retention, 64 gates / 25 pending intents globally, one
intent per owner gate. No deletion of another owner's live data to admit work.
Expired gate cleanup only follows expired pending rows; blocked gates outlive
all bounded 120s dispatch leases. Attempts are charged before dispatch and capped
at existing PortableOutbox.MAX_DISPATCH_ATTEMPTS=25. Backoff uses existing policy.
Clock rollback refuses mutation/dispatch; erase remains possible under rollback.

Acceptance: real SQLite previous-schema upgrade, replay/frozen conflict, atomic
admission/claim, restart/expired lease recovery, stale completion, failure/expiry/
rollback, owner gate/erasure race and unaffected foreign rows. AES/GCM wrong key/
tamper and no plaintext credential surface. Native Room cases compiled only;
actual native execution stays deferred. Runtime key custody/worker/UI restoration
and erasure wiring follow in FQ-16B2; do not claim full FQ-16 from this prerequisite.

## Acceptance, 2026-10-18

Room schema14 adds only quality_owner_gate and quality_report_outbox plus owned
indexes; schema13 catalog/other modules remain intact. Freeze preserves existing
ciphertext/deadline for identical command/hash and refuses a competing intent.
Activation/freeze/claim/completion/failure/erasure use Room transactions. Claim
charges attempts before I/O. Exact local claim IDs and lease deadlines fence
stale acknowledgements. Completion/failure also advance the owner clock fence.
Erasure blocks the current epoch even when no earlier gate existed, then removes
only this local owner’s intent rows; live foreign data cannot be evicted.

`python3 scripts/tests/test-quality-outbox-sql.py`: PASS, five cases using actual
production @Query SQL and the actual migration against Python SQLite, including
schema13→14 versus exported fresh schema14 (all columns/indexes), prior-row
preservation, migration replay, 12 independent concurrent connections (one
claim), restart/lease recovery, stale ack/failure, 25-attempt cap, clock rollback,
expiry, retry backoff and erased/foreign owner isolation. This verifies SQLite
SQL semantics; it does not impersonate Android Room transaction execution.

`./gradlew --offline :data:testDebugUnitTest --tests '*QualityReportSealTest'
--tests '*QualityReportJournalTest' :data:assembleDebugAndroidTest
:app:assembleDebug :app:lintDebug`: PASS. Six JVM cases: encrypted exact intent,
wrong key/owner/stamp/hash/tamper/clock/expiry, immutable replay/conflict,
admission/clock refusal, absent-gate erasure barrier and completion clock fence.
Native migration and two real Room concurrency/admission cases COMPILED ONLY,
execution OWED under deferred native acceptance. No new dependency. Three native
quality data cases and four app UI cases are compiled, not device-certified.

Final affected run /tmp/fq16b1-final.log PASS (39s); Gradle reported metaspace
pressure and completed successfully, without changing build settings. Scoped
whitespace/secret/schema/link review PASS. No queue is wired to runtime dispatch
yet; key custody/crypto-erasure/worker/UI recovery in FQ-16B2 are mandatory before
claiming usable offline private submission. D remains held. No deployment/merge.
