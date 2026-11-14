package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Process
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.di.EnterpriseModule
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseQueue
import com.anpfuel.data.enterprise.protectedqueue.ProtectedEnterpriseSyncStatusStore
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.*
import java.io.ByteArrayOutputStream
import java.io.DataOutputStream
import java.nio.ByteBuffer
import java.security.MessageDigest
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Local format/crypto acceptance only: named synthetic custody and receipt values,
 * actual Android Keystore/files, and two processes. This is not signed HTTP,
 * actual30-day elapsed acceptance, business permission or a production producer.
 * Run the two methods separately and force-stop only the owned target between
 * them, retaining app data and Keystore handles.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseSyncStatusVisibilityDeviceTest {
    private val context: Context get() = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun seedDismissedV2AndUntouchedPending(): Unit = runBlocking {
        val graph = Graph(context)
        assertEquals("Previous synthetic visibility stage was not cleaned", 0, graph.preferences.getInt(STAGE_PID, 0))
        graph.assertNoRetainedPayload()
        graph.establishSyntheticCustody()
        val refused = graph.operation(ORG_A, REFUSED, attachment = true)
        val pending = graph.operation(ORG_A, PENDING)
        val other = graph.operation(ORG_B, OTHER)
        val ack = graph.refusal(refused)
        graph.save(refused); graph.save(pending); graph.save(other)
        graph.queue.acknowledge(graph.attempt(ORG_A), refused, ack) { }
        graph.persistence.put(EnterpriseLocalRecord(partition(ORG_A), RECEIPT_CHECKPOINT, digest(ack.canonicalBytes).toByteArray()))
        graph.persistence.putPrivateFile(partition(ORG_A), FILE, ATTACHMENT)
        graph.installHistoricalV1(refused, ack)
        val beforeRead = graph.head(ORG_A)
        val visible = graph.status.detail(graph.attempt(ORG_A), REFUSED, refused.payloadDigest)
        assertFalse("Historical V1 refusal was silently hidden", visible.item.dismissedRejected)
        assertEquals(EnterpriseSyncStatus.REJECTED, visible.item.status)
        assertEquals(beforeRead, graph.head(ORG_A))
        assertEquals(1, graph.frameVersion(REFUSED))
        val before = graph.status.page(graph.attempt(ORG_A), EnterpriseSyncMode.PENDING)
        assertEquals(2, before.totals.pendingCount)
        assertEquals(1L, before.totals.unconfirmedAttachmentCount)
        assertTrue(graph.status.dismissRejected(graph.attempt(ORG_A), REFUSED, refused.payloadDigest, digest(ack.canonicalBytes)))
        val published = graph.head(ORG_A)
        assertFalse(graph.status.dismissRejected(graph.attempt(ORG_A), REFUSED, refused.payloadDigest, digest(ack.canonicalBytes)))
        assertEquals(published, graph.head(ORG_A))
        assertEquals(2, graph.frameVersion(REFUSED))
        graph.assertState(refused, ack, pending, other)
        graph.assertKeyCustody()
        assertTrue("Visibility seed marker was not durable", graph.preferences.edit().putInt(STAGE_PID, Process.myPid()).commit())
        // Keep ciphertext and exact handles for the independent process below.
    }

    @Test
    fun recoverVisibilityAfterProcessDeath(): Unit = runBlocking {
        val graph = Graph(context)
        try {
            val previous = graph.preferences.getInt(STAGE_PID, 0)
            assertTrue("Visibility recovery requires an earlier process", previous > 0 && previous != Process.myPid())
            val aliases = graph.assertKeyCustody()
            // No source confirmation or key creation is performed in recovery.
            val refused = graph.entry(ORG_A, REFUSED).operation
            val ack = graph.entry(ORG_A, REFUSED).acknowledgement ?: throw AssertionError("Retained refusal receipt is missing")
            val pending = graph.entry(ORG_A, PENDING).operation
            val other = graph.entry(ORG_B, OTHER).operation
            graph.assertState(refused, ack, pending, other)
            assertEquals(2, graph.frameVersion(REFUSED))
            assertEquals(aliases, graph.assertKeyCustody())
            assertFalse(graph.status.dismissRejected(graph.attempt(ORG_A), REFUSED, refused.payloadDigest, digest(ack.canonicalBytes)))
            assertNull(graph.entry(ORG_A, PENDING).acknowledgement)
            assertNull(graph.entry(ORG_B, OTHER).acknowledgement)
        } finally {
            graph.cleanup()
        }
    }

    private class Graph(context: Context) {
        val preferences = context.getSharedPreferences("enterprise-native-status-visibility", Context.MODE_PRIVATE)
        private val fence = EnterpriseSessionFence()
        private val files = EnterpriseModule.vaultFiles(context)
        private val keys = EnterpriseModule.vaultKeys()
        private val erasure = EnterpriseVaultErasure(files, keys, fence)
        private val clock = AndroidEnterpriseDeviceClock(context)
        private val identity = EnterpriseLocalIdentity(ACCOUNT, ApiEnvironment.PREVIEW.origin, FAMILY)
        /** Named local fixture authority; no server or runtime authorization is claimed. */
        private val syntheticCustody = EnterpriseContactAuthority { request ->
            EnterpriseContactReply.Active(EnterpriseContactGrant(EnterpriseEnrollmentBinding(request.target,
                if (request.target.partition.organizationId == ORG_A) ENROLL_A else ENROLL_B, 1), FAMILY, System.currentTimeMillis()))
        }
        val persistence = ProtectedEnterprisePersistence(files, keys, fence, erasure, { identity }, clock::read, syntheticCustody)
        val queue = ProtectedEnterpriseQueue(persistence)
        val status = ProtectedEnterpriseSyncStatusStore(persistence)
        private val generations = EnterpriseVaultGenerations(files, keys)

        fun assertNoRetainedPayload() {
            for (organization in listOf(ORG_A, ORG_B)) {
                val token = EnterpriseVaultCodec.token(partition(organization))
                assertTrue("Previous synthetic payload remains", files.generations(token).isEmpty())
                assertTrue("Previous synthetic payload keys remain", keys.aliases(EnterpriseVaultGenerationNames.prefix(token)).isEmpty())
                assertNull("Previous synthetic legacy payload remains", files.read(token))
                assertNull("Previous synthetic legacy key remains", keys.existing(EnterpriseVaultCodec.alias(token)))
            }
        }

        suspend fun establishSyntheticCustody() {
            val local = EnterpriseLocalContext({ ACCOUNT }, { ApiEnvironment.PREVIEW.origin }, persistence, { fence.epoch })
            fun current() = EnterpriseAccessBinding(ACCOUNT, ApiEnvironment.PREVIEW.origin, FAMILY, fence.epoch)
            for (organization in listOf(ORG_A, ORG_B)) {
                val binding = current(); val owner = local.reserveContact(local.acquireOwner(), binding, fence, ::current)
                // A prior completed cleanup leaves only its denial marker. The
                // exact test partitions have no payload (asserted above), so
                // this is an explicit reviewed empty-fixture reset.
                val handoff = persistence.confirm(EnterpriseDeviceTarget(partition(organization), member(organization)),
                    binding, owner, acknowledgeErasedWork = true)
                local.select(local.consumeContact(owner, handoff, current()), partition(organization))
            }
        }

        suspend fun attempt(organization: String): EnterpriseQueueAttempt {
            val epoch = fence.epoch; val background = fence.backgroundEpoch
            val account = EnterpriseQueueAccountLease.capture(identity, epoch, background, UUID.randomUUID().toString()) {
                fence.epoch == epoch && fence.backgroundEpoch == background
            }
            return account.forEnrollment(persistence.protectedEnrollment(account, partition(organization)))
        }

        suspend fun operation(organization: String, operationId: String, attachment: Boolean = false): EnterpriseOperation {
            val current = attempt(organization)
            val attachments = JSONArray()
            if (attachment) attachments.put(JSONObject().put("id", FILE).put("sha256", digest(ATTACHMENT).removePrefix("sha256:"))
                .put("bytes", ATTACHMENT.size).put("mime", "image/jpeg"))
            val raw = JSONObject().put("schema_version", 1).put("operation_id", operationId).put("owner_account_id", ACCOUNT)
                .put("api_origin", ApiEnvironment.PREVIEW.origin).put("org_id", organization).put("membership_id", member(organization))
                .put("enrollment_id", current.enrollment.enrollmentId).put("type", "validation.local_visibility").put("operation_schema", 1)
                .put("entity", JSONObject().put("type", "record").put("id", operationId).put("expected_revision", JSONObject.NULL))
                .put("occurred_at", EnterpriseTime.formatInstant(Instant.now())).put("entry_zone", "UTC").put("device_offset_seconds", 0)
                .put("depends_on", JSONArray()).put("soft_references", JSONArray()).put("attachments", attachments)
                .put("payload", JSONObject().put("value", "SYNTHETIC-LOCAL-VISIBILITY"))
            return EnterpriseOperation.seal(raw.toString().toByteArray(), SyncBinding.of(ACCOUNT, ApiEnvironment.PREVIEW.origin, organization))
        }

        fun refusal(operation: EnterpriseOperation): OperationAcknowledgement {
            val raw = JSONObject().put("schema_version", 1).put("operation_id", operation.operationId).put("payload_digest", operation.payloadDigest)
                .put("result", "REJECTED").put("code", "MALFORMED").put("entity_revision", JSONObject.NULL)
                .put("received_at", EnterpriseTime.formatInstant(Instant.now())).put("review_item_id", JSONObject.NULL).put("warnings", JSONArray())
            return OperationAcknowledgement.parse(raw.toString().toByteArray(), operation)
        }

        suspend fun save(operation: EnterpriseOperation) {
            val organization = operation.binding.organizationId
            assertTrue(queue.save(attempt(organization), operation) {
                it.put(EnterpriseLocalRecord(partition(organization), operation.operationId, projection(operation)))
            })
        }

        fun installHistoricalV1(operation: EnterpriseOperation, acknowledgement: OperationAcknowledgement) {
            val old = ByteArrayOutputStream().also { bytes -> DataOutputStream(bytes).use { out ->
                out.writeInt(0x51554555); out.writeInt(1)
                val op = operation.canonicalBytes; out.writeInt(op.size); out.write(op)
                val ack = acknowledgement.canonicalBytes; out.writeInt(ack.size); out.write(ack)
                out.writeLong(0); out.writeBoolean(false); out.writeLong(0)
            } }.toByteArray()
            val snapshot = generations.load(partition(ORG_A)) ?: throw AssertionError("Synthetic V1 source snapshot is missing")
            snapshot.entries["QUEUE:" + operation.operationId] = VaultEntry(VaultKind.QUEUE, operation.operationId, "", old)
            generations.commit(snapshot)
        }

        suspend fun entry(organization: String, operationId: String): EnterpriseQueueEntry = queue.entry(attempt(organization), operationId)
            ?: throw AssertionError("Original synthetic queue entry is missing")
        fun head(organization: String) = generations.head(EnterpriseVaultCodec.token(partition(organization)))
        fun frameVersion(operationId: String) = ByteBuffer.wrap(generations.load(partition(ORG_A))!!
            .entries.getValue("QUEUE:$operationId").bytes).getInt(4)

        suspend fun assertState(refused: EnterpriseOperation, ack: OperationAcknowledgement, pending: EnterpriseOperation, other: EnterpriseOperation) {
            val a = attempt(ORG_A); val retained = entry(ORG_A, REFUSED)
            assertTrue("Original intent was changed by visibility", refused.canonicalBytes.contentEquals(retained.operation.canonicalBytes))
            assertTrue("Original refusal receipt was changed", ack.canonicalBytes.contentEquals(retained.acknowledgement!!.canonicalBytes))
            val receiptCheckpoint = persistence.record(partition(ORG_A), RECEIPT_CHECKPOINT)
                ?: throw AssertionError("Original synthetic receipt checkpoint is missing")
            assertEquals("Original receipt bytes did not survive independently", String(receiptCheckpoint.envelope, Charsets.UTF_8), digest(retained.acknowledgement!!.canonicalBytes))
            assertTrue(retained.dismissedRejected)
            val page = status.page(a, EnterpriseSyncMode.PENDING)
            assertEquals(listOf(PENDING), page.items.map { it.identity.operationId })
            assertEquals(1, page.totals.pendingCount); assertEquals(1, page.totals.hiddenRejectedCount)
            assertEquals(0L, page.totals.unconfirmedAttachmentCount)
            val history = status.page(a, EnterpriseSyncMode.HISTORY)
            assertEquals(2, history.items.size)
            assertTrue(history.items.single { it.identity.operationId == REFUSED }.dismissedRejected)
            val detail = status.detail(a, REFUSED, refused.payloadDigest)
            assertEquals(EnterpriseAttachmentEvidence.MATCHING, detail.attachments.single().localEvidence)
            assertFalse(detail.attachments.single().uploadConfirmed)
            assertArrayEquals(ATTACHMENT, persistence.privateFile(partition(ORG_A), FILE))
            assertArrayEquals(projection(refused), persistence.record(partition(ORG_A), REFUSED)!!.envelope)
            assertArrayEquals(projection(pending), persistence.record(partition(ORG_A), PENDING)!!.envelope)
            assertArrayEquals(projection(other), persistence.record(partition(ORG_B), OTHER)!!.envelope)
            assertNull(entry(ORG_A, PENDING).acknowledgement); assertFalse(entry(ORG_A, PENDING).dismissedRejected)
            assertEquals(listOf(OTHER), status.page(attempt(ORG_B), EnterpriseSyncMode.PENDING).items.map { it.identity.operationId })
            assertFalse(entry(ORG_B, OTHER).dismissedRejected)
        }

        fun assertKeyCustody(): Set<String> {
            val result = mutableSetOf<String>()
            for (organization in listOf(ORG_A, ORG_B)) {
                val aliases = keys.aliases(EnterpriseVaultGenerationNames.prefix(EnterpriseVaultCodec.token(partition(organization))))
                assertTrue("Original synthetic Android handles are missing", aliases.isNotEmpty())
                aliases.forEach { alias -> assertNull("Android vault material was exportable", keys.existing(alias)!!.secret.encoded) }
                result += aliases
            }
            return result
        }

        fun cleanup() {
            for (organization in listOf(ORG_A, ORG_B)) assertTrue("Synthetic visibility cleanup failed", erasure.partitionRevoked(partition(organization)))
            assertTrue("Synthetic visibility marker cleanup failed", preferences.edit().clear().commit())
        }
    }

    companion object {
        private const val ACCOUNT = "7b0e2a54-0000-4000-8000-000000004001"
        private const val FAMILY = "7b0e2a54-0000-4000-8000-000000004002"
        private const val ORG_A = "7b0e2a54-0000-4000-8000-000000004011"
        private const val ORG_B = "7b0e2a54-0000-4000-8000-000000004012"
        private const val MEMBER_A = "7b0e2a54-0000-4000-8000-000000004021"
        private const val MEMBER_B = "7b0e2a54-0000-4000-8000-000000004022"
        private const val ENROLL_A = "7b0e2a54-0000-4000-8000-000000004031"
        private const val ENROLL_B = "7b0e2a54-0000-4000-8000-000000004032"
        private const val REFUSED = "7b0e2a54-0000-4000-8000-000000004041"
        private const val PENDING = "7b0e2a54-0000-4000-8000-000000004042"
        private const val OTHER = "7b0e2a54-0000-4000-8000-000000004043"
        private const val FILE = "7b0e2a54-0000-4000-8000-000000004051"
        private const val RECEIPT_CHECKPOINT = "7b0e2a54-0000-4000-8000-000000004052"
        private const val STAGE_PID = "seed_process"
        private val ATTACHMENT = "SYNTHETIC-LOCAL-STATUS-ATTACHMENT".toByteArray()
        private fun partition(organization: String) = EnterprisePartition.of(ACCOUNT, ApiEnvironment.PREVIEW.origin, organization)
        private fun member(organization: String) = if (organization == ORG_A) MEMBER_A else MEMBER_B
        private fun digest(bytes: ByteArray) = "sha256:" + MessageDigest.getInstance("SHA-256").digest(bytes)
            .joinToString("") { "%02x".format(it.toInt() and 255) }
        private fun projection(operation: EnterpriseOperation) = operation.canonicalBytes
    }
}
