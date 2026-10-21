# Vehicle registration policy v2 — staged MEX-01 contract

Date: 2026-10-21. State: HOSTED_SOURCE_IMPLEMENTED / LOCAL_RUNTIME_PENDING.
Rules: B-BR-MEX-01/02/06; BUC-MEX-01/04; amends B-BR-PR01/02/04/05/06.
[Machine limits](../../contracts/premium/policy-v1.json) contain an additive v2
amendment; [worked examples](../../contracts/testdata/premium/vehicle-registration-v2.json)
are synthetic contract acceptance, not persistence or billing acceptance.

## Registration and authority

One new local vehicle is free, including guest/offline operation. Creating a
second or third requires the planned `vehicles.multiple` capability in the one
existing Premium bundle. The MEX-07 hosted implementation grants this capability through the existing
verified subscription evaluator; local admission remains MEX-08/EE-19 pending. Unknown/missing
capability versions cannot grant extra registrations. The technical bound stays
three; it is not an unlimited entitlement or a promise to sell three slots.
No new product, price, trial, provider enrollment or manual grant path is added.

Count distinct non-deleted vehicle IDs in the owning local/account partition.
An archived but retained registration still counts, matching the existing cloud
count contract; hiding/reordering/archiving is not a quota reset. A PHEV uses
one ID/slot for both carriers. A vehicle correction or identical-ID replay is not
a new registration. Do not merge unrelated guest/account/origin partitions or
derive ownership from user-submitted account IDs. Identity, conflicts, erasure
fences and version checks apply before treating an ID as an existing replay.

Hosted creation additionally requires existing `cloud.sync`, current owner and
signed proof, device binding, deletion epoch and opted-in consent. A free local
first vehicle does not imply free hosted uploads. All hosted checks stay live;
neither a purchase UI flag nor a local offline lease authorizes a server write.
Retained owner exit operations continue under PRM policy, never a quota bypass.

Local paid creation can use only a verified, versioned owner/origin capability
lease, bounded by min(verification time + 24 hours, verified paid-through time).
The end is exclusive; rollback/future verification, wrong owner/origin, revoked
authority, unknown policy or elapsed lease refuse extra creation. Server proof
freshness remains 900 seconds, exclusive. Outages cannot renew either deadline.
Free operation and held-data rights do not depend on renewing a paid lease.

## Existing data, lapse and upgrade

Existing two/three-vehicle users retain all IDs, settings, histories, navigation,
basic alerts, read/export/correction/delete access. Even unexpected imported
over-ceiling legacy data is retained for correction/exit; no migration truncates,
hides or automatically archives it. Extra creation is blocked until eligible
and within the technical ceiling. Deleting a vehicle follows existing explicit
history/archive/cascade/erasure rules; quota never authorizes silent data removal.

Renewal cancellation does not end access before verified paid-through time.
Expiry/revocation stops new paid work; pending local changes stay durable in the
correct owner partition. Correction of already-held local records is free and
does not upload automatically. Hosted replay/resume still needs current PRM
authority, consent, revisions and erasure checks. No lapse extends cloud exit
retention or converts old snapshots into current access.

The existing `professional.write` / `professional.report`, cloud/backup/restore
and advanced-alert capability matrix is unchanged. Configured estimates and
basic tools stay free; this amendment does not newly make ledger/report
generation free. Account/community/public exploration never gain a paid gate,
and payment never affects evidence, trust, verification priority or ranking.

## Import, restore and atomic admission

Preview counts only genuinely new distinct IDs and reports retained/conflicting/
erased IDs separately. Same owner/ID restoration is not a registration, but
cannot resurrect erased data or overwrite a newer revision. Restoring bytes is
not restoring a paid lease. New distinct imported IDs consume slots exactly as
manual creation does; apply the full selected batch under the existing atomic
command contract. An over-limit batch is refused without partial registration;
the user may explicitly choose a smaller preview. No silent truncation.

Preview is advisory. Commit rechecks current count, paid proof, consent, owner,
epoch, ID/version and erased fences inside the owning transaction. Two callers
starting from empty free state cannot both win: one registers, the second sees
count1 and requires Premium. Concurrent Premium callers starting at2 cannot
both pass ceiling3. Hosted/local admissions must use each existing persistence
boundary, not an unlocked count-then-save or a UI-only restriction. Failure or
cancellation leaves counts, records, receipts and histories unchanged.

## Reviewed examples and deferred implementation

Twenty-three examples freeze first/second, Premium third/fourth, retained2/3/4,
expired correction/new work, same-ID replay/restore, two sequential winners'
observations for the free concurrent case, atomic import refusal, live cloud
authority versus a local lease, expiry/clock rollback/stale/unknown proof.
They specify outcomes; they do not prove actual locking, SQLite or billing.

MEX-07 implements hosted capability/wire/admission and real PostGIS concurrency,
failure/rollback and unauthorized/expired tests; MEX-08 owns local atomic quota,
actual SQLite upgrade/reopen/concurrency/rollback and retained-data compatibility.
Both must consume this version before activation. Applied SQL/Room migrations
and current limits remain unchanged by MEX-01. Future EE contracts extend
vehicle capabilities without resetting identities or counting a hybrid twice.
