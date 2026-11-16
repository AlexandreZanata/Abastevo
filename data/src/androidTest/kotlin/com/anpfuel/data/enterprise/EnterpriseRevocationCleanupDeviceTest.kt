package com.anpfuel.data.enterprise

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.application.enterprise.EnterpriseAccessEndedLocked
import com.anpfuel.application.enterprise.EnterpriseContactAuthority
import com.anpfuel.application.enterprise.EnterpriseContactReply
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.application.enterprise.queue.EnterpriseOperationDelivery
import com.anpfuel.application.enterprise.queue.EnterpriseOperationTransport
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducer
import com.anpfuel.application.enterprise.queue.EnterpriseQueueProducerDescriptor
import com.anpfuel.application.enterprise.queue.EnterpriseQueueRegistry
import com.anpfuel.application.enterprise.queue.EnterpriseQueueService
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import com.anpfuel.application.enterprise.queue.EnterpriseQueueTransaction
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.domain.enterprise.EnterpriseBoundOfflineLease
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterpriseOfflineLease
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.io.Closeable
import java.io.File
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/**
 * EMP-OFFL-011 (B-BR-EMP-536..539) on the actual AndroidKeyStore provider and atomic files: the ACCESS_ENDED marker,
 * intake-only delivery, erasure of exactly one partition and an interrupted cleanup finished by maintenance. The signed
 * contact and the transport are explicit synthetic ports; the real signed contact is EMP-PRIV-003's native test.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseRevocationCleanupDeviceTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    private class Fixture(context: Context) : Closeable {
        val root = File(context.noBackupFilesDir, "enterprise-revocation-test-" + UUID.randomUUID())
        private val realKeys = AndroidEnterpriseVaultKeys()
        val files = AtomicEnterpriseVaultFiles(root)
        val account: String = UUID.randomUUID().toString()
        val other: String = UUID.randomUUID().toString()
        val tags = listOf(account, other).map { EnterpriseVaultCodec.accountTag(it, ORIGIN) }
        var failing: Set<String> = emptySet()
        val keys = object : EnterpriseVaultKeys by realKeys {
            override fun aliases(prefix: String): List<String> = realKeys.aliases(prefix).filter { alias -> tags.any { alias.contains(it) } }
            override fun delete(alias: String) { if (alias in failing) error("synthetic provider delete fault"); realKeys.delete(alias) }
        }
        val fence = EnterpriseSessionFence()
        val erasure = EnterpriseVaultErasure(files, keys, fence)
        var now = EnterpriseDeviceClock("synthetic-revocation-boot", 1_000, 10_000)
        var identity = EnterpriseLocalIdentity(account, ORIGIN, UUID.randomUUID().toString())
        val member: String = UUID.randomUUID().toString()
        val enrollment: String = UUID.randomUUID().toString()
        var verdict: (com.anpfuel.application.enterprise.EnterpriseContactRequest) -> EnterpriseContactReply = { error("unexpected contact") }
        var contacts = 0
        val store = ProtectedEnterprisePersistence(files, keys, fence, erasure, { identity }, { now },
            EnterpriseContactAuthority { request -> contacts++; verdict(request) })
        val queue = ProtectedEnterpriseQueue(store)
        val sent = mutableListOf<String>()
        var answer: (EnterprisePartition, List<EnterpriseOperation>) -> List<OperationAcknowledgement> = { _, ops -> ops.map { ack(it, "ACCEPTED", null) } }

        fun seed(owner: String = account, family: String = identity.family): EnterprisePartition {
            val partition = EnterprisePartition.of(owner, ORIGIN, UUID.randomUUID().toString())
            val time = EnterpriseOfflineLease(family, 10_000, now, EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS)
            val bound = EnterpriseBoundOfflineLease(member, enrollment, 1, time)
            files.locked(EnterpriseVaultCodec.accountTag(owner, ORIGIN)) {
                EnterpriseVaultGenerations(files, keys).commit(VaultSnapshot(partition, time, bound = bound))
            }
            return partition
        }

        fun lease(): EnterpriseQueueAccountLease {
            val epoch = fence.epoch; val background = fence.backgroundEpoch; val captured = identity
            return EnterpriseQueueAccountLease.capture(captured, epoch, background, UUID.randomUUID().toString()) {
                identity == captured && fence.epoch == epoch && fence.backgroundEpoch == background
            }
        }
        val source = object : EnterpriseQueueSource {
            override suspend fun account(): EnterpriseQueueAccountLease = lease()
            override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt =
                account.forEnrollment(store.protectedEnrollment(account, partition))
        }
        private val producer = object : EnterpriseQueueProducer {
            override fun descriptor() = EnterpriseQueueProducerDescriptor("validation.record")
            override fun save(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation) = Unit
            override fun acknowledge(tx: EnterpriseQueueTransaction, attempt: EnterpriseQueueAttempt, operation: EnterpriseOperation,
                ack: OperationAcknowledgement) = Unit
        }
        val service = EnterpriseQueueService(source, queue, EnterpriseOperationTransport { attempt, operations ->
            sent += operations.map { it.operationId }
            EnterpriseOperationDelivery(answer(attempt.partition, operations))
        }, EnterpriseQueueRegistry(producer), { now }, { 0L })
        val runner = EnterpriseOperationWorkRunner(source, store::discover, { account, p -> service.dispatch(account, p) },
            cleanup = { account, p, hinted -> store.revocationCleanup(account, p, hinted) })

        fun operation(p: EnterprisePartition): EnterpriseOperation = EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"${UUID.randomUUID()}","owner_account_id":"${p.ownerAccount}","api_origin":"${p.origin}","org_id":"${p.organizationId}","membership_id":"$member","enrollment_id":"$enrollment","type":"validation.record","operation_schema":1,"entity":{"type":"record","id":"${UUID.randomUUID()}","expected_revision":null},"occurred_at":"2026-11-16T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"SYNTHETIC-REVOCATION"}}""".toByteArray(),
            SyncBinding.of(p.ownerAccount, p.origin, p.organizationId))
        fun ack(operation: EnterpriseOperation, result: String, code: String?): OperationAcknowledgement {
            val final = result != "RETRY"
            val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${UUID.randomUUID()}\"" else "null"
            return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${if (result == "ACCEPTED") "1" else "null"},"received_at":${if (final) "\"2026-11-16T12:00:01Z\"" else "null"},"review_item_id":$review,"warnings":[]}""".toByteArray(), operation)
        }
        fun aliasesOf(p: EnterprisePartition): List<String> =
            realKeys.aliases(EnterpriseVaultGenerationNames.PREFIX).filter { it.contains(EnterpriseVaultCodec.token(p)) && !it.contains("_denied_") }
        fun later() { now = now.copy(elapsedMillis = now.elapsedMillis + 7 * 3_600_000L, wallMillis = now.wallMillis + 7 * 3_600_000L) }
        fun runAll() = runBlocking { repeat(4) { runner.runOnce() } }

        override fun close() {
            try {
                realKeys.aliases(EnterpriseVaultGenerationNames.PREFIX).filter { alias -> tags.any { alias.contains(it) } }.forEach(realKeys::delete)
                realKeys.aliases("abastevo_enterprise_v1_").filter { alias -> tags.any { alias.contains(it) } }.forEach(realKeys::delete)
            } finally { root.deleteRecursively() }
        }
    }

    @Test
    fun accessEndedDeliversThenErasesOnlyItsPartitionOnTheRealProvider(): Unit = runBlocking {
        Fixture(context).use { f ->
            val revoked = f.seed(); val kept = f.seed()
            val foreign = f.seed(owner = f.other, family = UUID.randomUUID().toString())
            f.store.put(EnterpriseLocalRecord(kept, f.member, byteArrayOf(2)))
            val held = f.operation(revoked); val waiting = f.operation(revoked)
            assertTrue(f.service.save(revoked, held)); assertTrue(f.service.save(revoked, waiting))
            f.verdict = { EnterpriseContactReply.Lock(it.existingEnrollment!!, f.identity.family, 10_500) }
            f.answer = { p, ops -> ops.map {
                when {
                    p != revoked -> f.ack(it, "ACCEPTED", null)
                    it.operationId == held.operationId -> f.ack(it, "ACCEPTED_FOR_REVIEW", "ACCESS_ENDED_BEFORE_RECEIPT")
                    else -> f.ack(it, "RETRY", "UNAVAILABLE")
                }
            } }
            f.runAll()
            assertEquals(1, f.contacts)
            assertNotNull("The real provider refused the ACCESS_ENDED marker",
                f.keys.existing(EnterpriseVaultGenerationNames.ended(EnterpriseVaultCodec.token(revoked))))
            try { f.store.record(revoked, f.member); fail("A locked partition was readable") } catch (_: EnterpriseAccessEndedLocked) { }
            assertTrue("Eligible work was erased before its final answer", f.aliasesOf(revoked).isNotEmpty())
            f.answer = { p, ops -> ops.map { if (p == revoked) f.ack(it, "REJECTED", "INTAKE_WINDOW_CLOSED") else f.ack(it, "ACCEPTED", null) } }
            f.later(); f.runAll()
            assertEquals(listOf(held.operationId, waiting.operationId, waiting.operationId).sorted(), f.sent.sorted())
            assertTrue("Revoked key material survived on the real provider", f.aliasesOf(revoked).isEmpty())
            try { f.store.record(revoked, f.member); fail("An erased partition was readable") } catch (_: EnterpriseLocalDenied) { }
            assertArrayEquals(byteArrayOf(2), f.store.record(kept, f.member)!!.envelope)
            assertTrue(f.aliasesOf(kept).isNotEmpty()); assertTrue(f.aliasesOf(foreign).isNotEmpty())
            assertFalse(f.erasure.cleanupRequired)
        }
    }

    @Test
    fun anInterruptedErasureStaysDeniedAndMaintenanceFinishesItOnTheRealProvider(): Unit = runBlocking {
        Fixture(context).use { f ->
            val revoked = f.seed(); val kept = f.seed()
            f.store.put(EnterpriseLocalRecord(revoked, f.member, byteArrayOf(1)))
            f.service.save(revoked, f.operation(revoked))
            f.verdict = { EnterpriseContactReply.Erase(it.existingEnrollment!!, f.identity.family, 10_500) }
            f.answer = { p, ops -> if (p == revoked) throw com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure(revocationHint = true)
                else ops.map { f.ack(it, "ACCEPTED", null) } }
            val token = EnterpriseVaultCodec.token(revoked)
            val generation = EnterpriseVaultGenerations(f.files, f.keys).head(token)!!
            f.failing = setOf(EnterpriseVaultGenerationNames.data(token, generation))
            f.runAll()
            assertEquals(1, f.contacts)
            assertTrue(f.erasure.cleanupRequired)
            try { f.store.record(revoked, f.member); fail("An interrupted erasure exposed data") } catch (_: EnterpriseLocalDenied) { }
            val sent = f.sent.size
            f.later(); f.runAll()
            assertEquals("A lost partition sent work again", sent, f.sent.size)
            f.failing = emptySet()
            assertTrue(EnterpriseVaultJanitor(f.files, f.keys, f.fence, f.erasure, { f.now }).runOnce().complete)
            assertFalse(f.erasure.cleanupRequired)
            assertTrue(f.aliasesOf(revoked).isEmpty()); assertTrue(f.aliasesOf(kept).isNotEmpty())
        }
    }

    private companion object { const val ORIGIN = "https://revocation.example.invalid" }
}
