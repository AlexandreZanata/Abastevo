package com.anpfuel.data.repository

import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.data.local.dao.PhotoUploadSessionDao
import com.anpfuel.data.local.entity.PhotoUploadSessionEntity
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.OwnedContribution
import com.anpfuel.domain.repository.PendingContribution
import com.anpfuel.domain.repository.QueuedContribution
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

private class FakeOutbox : ContributionOutboxRepository {
    val purged = mutableListOf<String>()
    override suspend fun purgeOwned(ownerScope: String): Int {
        purged.add(ownerScope)
        return 2
    }
    override suspend fun enqueue(draft: ContributionDraft, payloadJson: String): QueuedContribution =
        throw UnsupportedOperationException()
    override suspend fun findDispatchable(commandId: String, nowMillis: Long): QueuedContribution? = null
    override suspend fun listDispatchablePage(nowMillis: Long, limit: Int): List<QueuedContribution> = emptyList()
    override suspend fun findOwned(commandId: String): OwnedContribution? = null
    override suspend fun listOwnedPage(limit: Int, offset: Int): List<OwnedContribution> = emptyList()
    override suspend fun purgeTerminalHistory(): Int = 0
    override suspend fun loadPayload(commandId: String): String? = null
    override suspend fun markDispatched(commandId: String, nonce: String) = Unit
    override suspend fun markAcknowledged(commandId: String, revision: Int) = Unit
    override suspend fun markFailed(commandId: String, nowMillis: Long) = Unit
    override suspend fun cancel(commandId: String) = Unit
}

private class FakeSessions : PhotoUploadSessionDao {
    val rows = mutableListOf(
        PhotoUploadSessionEntity("cap-1", "sess-1", "photo-1", "scope-a", "https://teste.abastevo.com.br", 1L, 2L, "aa"),
        PhotoUploadSessionEntity("cap-2", "sess-2", "photo-2", "scope-b", "https://teste.abastevo.com.br", 1L, 2L, "bb"),
    )
    override suspend fun find(captureId: String): PhotoUploadSessionEntity? =
        rows.firstOrNull { it.captureId == captureId }
    override suspend fun insert(session: PhotoUploadSessionEntity): Long {
        rows.add(session)
        return 1L
    }
    override suspend fun ready(captureId: String, sessionId: String, evidenceId: String): Int = 0
    override suspend fun listByOwner(ownerScope: String): List<PhotoUploadSessionEntity> =
        rows.filter { it.ownerScope == ownerScope }
    override suspend fun deleteByOwner(ownerScope: String): Int {
        val n = rows.count { it.ownerScope == ownerScope }
        rows.removeAll { it.ownerScope == ownerScope }
        return n
    }
}

private class FakeCache : PhotoCache {
    val deleted = mutableListOf<String>()
    override fun put(id: String, bytes: ByteArray, capturedAtMillis: Long) = Unit
    override fun get(id: String): ByteArray? = null
    override fun delete(id: String) {
        deleted.add(id)
    }
    override fun sweepExpired(nowMillis: Long): Int = 0
}

class OwnerLocalErasureTest {

    private fun erasure(scope: ContributionScope?, outbox: FakeOutbox = FakeOutbox(), sessions: FakeSessions = FakeSessions(), cache: FakeCache = FakeCache()) =
        OwnerLocalErasure(
            outbox = outbox,
            photoSessions = sessions,
            photoCache = cache,
            scopeProvider = ContributionScopeProvider {
                scope ?: throw IllegalStateException("identity-unavailable")
            },
        )

    @Test
    fun `purgeCurrentOwner clears queue sessions and cache copies`() = runTest {
        val outbox = FakeOutbox()
        val sessions = FakeSessions()
        val cache = FakeCache()
        val report = erasure(ContributionScope("scope-a", "https://teste.abastevo.com.br"), outbox, sessions, cache).purgeCurrentOwner()

        assertEquals(2, report.outboxPurged)
        assertEquals(1, report.sessionsPurged)
        assertEquals(listOf("scope-a"), outbox.purged)
        assertEquals(setOf("photo-1", "cap-1"), cache.deleted.toSet())
        assertEquals(listOf("cap-2"), sessions.rows.map { it.captureId })
    }

    @Test
    fun `purge without identity purges nothing`() = runTest {
        val outbox = FakeOutbox()
        val report = erasure(null, outbox).purgeCurrentOwner()
        assertEquals(OwnerLocalErasure.Report(0, 0, 0), report)
        assertEquals(0, outbox.purged.size)
    }

    @Test
    fun `purgeScope is owner isolated`() = runTest {
        val outbox = FakeOutbox()
        val sessions = FakeSessions()
        val report = erasure(null, outbox, sessions).purgeScope("scope-b")
        assertEquals(1, report.sessionsPurged)
        assertEquals(listOf("cap-1"), sessions.rows.map { it.captureId })
    }
}
