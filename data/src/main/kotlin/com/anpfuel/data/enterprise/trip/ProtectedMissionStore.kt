package com.anpfuel.data.enterprise.trip

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.trip.EnterpriseMissionProducers
import com.anpfuel.application.enterprise.trip.EnterpriseMissionSnapshot
import com.anpfuel.application.enterprise.trip.EnterpriseMissionStore
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import com.anpfuel.data.enterprise.VaultSnapshot
import com.anpfuel.data.enterprise.protectedqueue.ProtectedQueueCodec
import com.anpfuel.domain.enterprise.trip.MissionInbox
import com.anpfuel.domain.enterprise.trip.MissionLocalException

/**
 * The EMP-TRIP-021 missions inbox storage (EDR-342): the sealed `mission-inbox` RECORD of the exact partition's protected
 * generation and the retained queue entries of its saved answers, read and written through the attempt's custody,
 * enrollment and lease checks. A server page is written without an operation in one locked read-modify-write; an answer is
 * written by the `trip.mission.respond` producer in the queue's own write. No plaintext table exists.
 */
class ProtectedMissionStore(private val persistence: ProtectedEnterprisePersistence) : EnterpriseMissionStore {
    /** The copy is business evidence: an unreadable or foreign one is unavailable and never replaced (EDR-342). */
    private fun inbox(snapshot: VaultSnapshot, attempt: EnterpriseQueueAttempt): MissionInbox? {
        val stored = snapshot.entries[KEY] ?: return null
        return try {
            MissionInbox.decode(stored.bytes).requireBinding(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)
        } catch (_: Exception) { throw EnterprisePersistenceUnavailable() }
    }

    override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseMissionSnapshot = persistence.queueAccess(attempt) { snapshot ->
        val inbox = inbox(snapshot, attempt) ?: return@queueAccess VaultQueueChange(EnterpriseMissionSnapshot(null, emptyMap()))
        val entries = inbox.responses.map { it.operationId }.associateWith { id ->
            val entry = snapshot.entries["QUEUE:$id"] ?: throw EnterprisePersistenceUnavailable()
            ProtectedQueueCodec.decode(entry.bytes, snapshot, entry.id)
        }
        VaultQueueChange(EnterpriseMissionSnapshot(inbox, entries))
    }

    override suspend fun update(attempt: EnterpriseQueueAttempt, change: (MissionInbox) -> MissionInbox): MissionInbox {
        val member = attempt.enrollment.target.membershipId
        val enrollment = attempt.enrollment.enrollmentId
        var refusal: MissionLocalException? = null
        val result = persistence.queueAccess(attempt) { snapshot ->
            val current = inbox(snapshot, attempt) ?: MissionInbox.empty(member, enrollment)
            // A local refusal is the caller's answer, not an outage: keep it and write nothing.
            val next = try { change(current) } catch (refused: MissionLocalException) {
                refusal = refused
                return@queueAccess VaultQueueChange<MissionInbox?>(null)
            }
            next.requireBinding(member, enrollment)
            val bytes = next.canonicalBytes
            if (snapshot.entries[KEY]?.bytes?.contentEquals(bytes) == true) VaultQueueChange(next)
            else {
                snapshot.entries[KEY] = VaultEntry(VaultKind.RECORD, EnterpriseMissionProducers.INBOX_RECORD_ID, "", bytes)
                VaultQueueChange(next, changed = true)
            }
        }
        refusal?.let { throw it }
        return result ?: throw EnterprisePersistenceUnavailable()
    }

    override fun toString(): String = "ProtectedMissionStore(redacted)"

    private companion object { const val KEY = "RECORD:" + EnterpriseMissionProducers.INBOX_RECORD_ID }
}
