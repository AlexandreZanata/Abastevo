package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.attachment.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.sync.*
import java.security.MessageDigest
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun aid(n: Int) = "7a000000-0000-4000-8000-" + n.toString().padStart(12, '0')
private const val ATTACHMENT_ORIGIN = "https://attachments.example.invalid"

/** In-memory stand-in for the protected attachment store; the data module proves the sealed one. */
internal class AttachmentDisk {
    class Partition { var book: AttachmentBook? = null; val bytes = linkedMapOf<String, ByteArray>(); val owners = linkedMapOf<String, EnterpriseQueueEntry>() }
    val partitions = mutableMapOf<EnterprisePartition, Partition>()
    fun of(p: EnterprisePartition) = partitions.getOrPut(p) { Partition() }
    var refuseTransitions = false
}

internal class AttachmentServerFake {
    var session: (EnterpriseAttachment) -> AttachmentSessionResult = { error("no session answer") }
    var transfer: (String, ByteArray) -> AttachmentTransferResult = { _, _ -> error("no transfer answer") }
    val opened = mutableListOf<String>()
    val uploads = mutableListOf<Pair<String, ByteArray>>()
    val grants = mutableListOf<String>()
    private var grant = 0
    fun response(item: EnterpriseAttachment, storage: AttachmentStorage, revision: Long, member: String, enrollment: String): AttachmentSessionResult {
        val request = AttachmentSessionRequest.create(AttachmentRequestSnapshot(aid(7000 + opened.size), aid(7500), "synthetic-attachment-access", member, enrollment,
            item.declarationId, item.documentId, AttachmentPolicy.KIND, item.sha256, item.bytes, AttachmentPolicy.MEDIA_TYPE, item.linkId, item.target.kind.name, item.target.id))
        val upload = if (storage == AttachmentStorage.UPLOADING) """{"capability":"v1.grant-${++grant}","expires_at":"2026-11-15T12:05:00Z"}""" else "null"
        val raw = """{"schema_version":1,"request_id":"${request.snapshot().requestId}","membership_id":"$member","enrollment_id":"$enrollment","document_id":"${item.documentId}","storage":"${storage.name}","storage_revision":$revision,"upload":$upload}"""
        return AttachmentSessionResult.Opened(AttachmentSessionResponse.parse(raw.toByteArray(), request))
    }
}

internal class AttachmentFixture(val enabled: Boolean = true) {
    val disk = AttachmentDisk()
    val server = AttachmentServerFake()
    val fence = EnterpriseSessionFence()
    val a = EnterprisePartition.of(aid(1), ATTACHMENT_ORIGIN, aid(2))
    val b = EnterprisePartition.of(aid(1), ATTACHMENT_ORIGIN, aid(20))
    val member = aid(3)
    val enrollment = aid(4)
    var identity = EnterpriseLocalIdentity(aid(1), ATTACHMENT_ORIGIN, aid(9))
    var elapsed = 5_000L
    private var nonce = 1000
    val source = object : EnterpriseQueueSource {
        override suspend fun account(): EnterpriseQueueAccountLease {
            val captured = identity; val epoch = fence.epoch; val background = fence.backgroundEpoch
            return EnterpriseQueueAccountLease.capture(captured, epoch, background, aid(nonce++)) {
                identity == captured && fence.epoch == epoch && fence.backgroundEpoch == background
            }
        }
        override suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
            account.assertCurrent()
            return account.forEnrollment(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(partition, member), enrollment, 1))
        }
    }
    val store = object : EnterpriseAttachmentStore {
        override suspend fun capture(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment, bytes: ByteArray) {
            attempt.assertCurrent()
            val p = disk.of(attempt.partition)
            if (attachment.ownerOperationId !in p.owners) throw AttachmentLocalException(AttachmentLocalFailure.UNKNOWN_OPERATION)
            p.book = (p.book ?: AttachmentBook.empty(member, enrollment)).withCapture(attachment)
            p.bytes[attachment.documentId] = bytes.copyOf()
        }
        override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseAttachmentSnapshot {
            attempt.assertCurrent()
            val p = disk.of(attempt.partition)
            return EnterpriseAttachmentSnapshot(p.book, p.owners.filterKeys { id -> p.book?.items?.any { it.ownerOperationId == id } == true })
        }
        override suspend fun bytes(attempt: EnterpriseQueueAttempt, documentId: String): ByteArray? = disk.of(attempt.partition).bytes[documentId]?.copyOf()
        override suspend fun transition(attempt: EnterpriseQueueAttempt, expected: EnterpriseAttachment, next: EnterpriseAttachment): Boolean {
            attempt.assertCurrent()
            if (disk.refuseTransitions) return false
            val p = disk.of(attempt.partition)
            val replaced = p.book?.replace(expected, next) ?: return false
            p.book = replaced
            if (next.state == AttachmentState.STORED) p.bytes.remove(next.documentId)
            return true
        }
    }
    val transport = object : EnterpriseAttachmentTransport {
        override suspend fun open(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment): AttachmentSessionResult {
            attempt.assertCurrent(); server.opened += attachment.documentId
            return server.session(attachment)
        }
        override suspend fun upload(attempt: EnterpriseQueueAttempt, documentId: String, grant: AttachmentUploadGrant, bytes: ByteArray): AttachmentTransferResult {
            attempt.assertCurrent(); server.uploads += documentId to bytes.copyOf(); server.grants += grant.capability
            return server.transfer(documentId, bytes)
        }
    }
    private var ids = 100
    val service = EnterpriseAttachmentService(source, store, transport, { EnterpriseDeviceClock("boot-1", elapsed, 1_763_208_000_000L + elapsed) },
        { Instant.parse("2026-11-15T12:00:00Z") }, entropy = { 0L }, newId = { aid(ids++) }, enabled = enabled)

    fun owner(p: EnterprisePartition = a, n: Int = 900): EnterpriseOperation {
        val operation = EnterpriseOperation.seal(("""{"schema_version":1,"operation_id":"${aid(n)}","owner_account_id":"${p.ownerAccount}","api_origin":"${p.origin}","org_id":"${p.organizationId}","membership_id":"$member","enrollment_id":"$enrollment","type":"validation.record","operation_schema":1,"entity":{"type":"record","id":"${aid(n + 1)}","expected_revision":null},"occurred_at":"2026-11-15T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"SYNTHETIC"}}""").toByteArray(),
            SyncBinding.of(p.ownerAccount, p.origin, p.organizationId))
        disk.of(p).owners[operation.operationId] = EnterpriseQueueEntry(operation)
        return operation
    }
    fun receipt(operation: EnterpriseOperation, result: String, p: EnterprisePartition = a) {
        val fields = when (result) {
            "ACCEPTED" -> """"code":null,"entity_revision":1,"received_at":"2026-11-15T12:00:01Z","review_item_id":null"""
            "ACCEPTED_FOR_REVIEW" -> """"code":"STALE_REVISION","entity_revision":null,"received_at":"2026-11-15T12:00:01Z","review_item_id":"${aid(950)}""""
            else -> """"code":"MALFORMED","entity_revision":null,"received_at":"2026-11-15T12:00:01Z","review_item_id":null"""
        }
        val ack = OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"$result",$fields,"warnings":[]}""").toByteArray(), operation)
        disk.of(p).owners[operation.operationId] = EnterpriseQueueEntry(operation, acknowledgement = ack)
    }
    fun jpeg(size: Int = 96, fill: Byte = 5): ByteArray = ByteArray(size) { fill }.also {
        it[0] = 0xff.toByte(); it[1] = 0xd8.toByte(); it[2] = 0xff.toByte(); it[size - 2] = 0xff.toByte(); it[size - 1] = 0xd9.toByte()
    }
    fun target() = AttachmentTarget(DocumentTargetKind.METER_READING, aid(901))
    suspend fun deliver(p: EnterprisePartition = a) = service.deliver(source.account(), p)
    fun item(id: String, p: EnterprisePartition = a) = requireNotNull(disk.of(p).book?.item(id))
    suspend fun label(id: String) = service.attachments(a).single { it.attachment.documentId == id }.label
}

class EnterpriseAttachmentServiceTest {
    private fun sha(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) }

    @Test fun `capture seals a JPEG for a queued operation with fixed identifiers and refuses everything else`() = runTest {
        val f = AttachmentFixture()
        val owner = f.owner()
        val bytes = f.jpeg()
        val captured = f.service.capture(f.a, owner.operationId, f.target(), bytes)
        assertEquals(AttachmentState.WAITING_FOR_RECORD, captured.state)
        assertEquals(sha(bytes), captured.sha256); assertEquals(bytes.size.toLong(), captured.bytes)
        assertEquals(3, setOf(captured.documentId, captured.declarationId, captured.linkId).size)
        assertArrayEquals(bytes, f.disk.of(f.a).bytes[captured.documentId])
        assertEquals(AttachmentLabel.SAVED_ON_DEVICE, f.label(captured.documentId))
        for ((input, failure) in listOf(f.jpeg().also { it[0] = 0 } to AttachmentLocalFailure.NOT_JPEG, ByteArray(0) to AttachmentLocalFailure.SIZE))
            assertEquals(failure, assertThrows(AttachmentLocalException::class.java) { kotlinx.coroutines.runBlocking { f.service.capture(f.a, owner.operationId, f.target(), input) } }.failure)
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, assertThrows(AttachmentLocalException::class.java) {
            kotlinx.coroutines.runBlocking { f.service.capture(f.a, aid(999), f.target(), f.jpeg()) } }.failure)
        // BR-OFFL-003: another organization of the same account sees nothing of A.
        assertTrue(f.service.attachments(f.b).isEmpty())
        assertEquals(1, f.disk.of(f.a).book!!.items.size)
        val disabled = AttachmentFixture(enabled = false)
        val queued = disabled.owner()
        assertEquals(AttachmentLocalFailure.DISABLED, assertThrows(AttachmentLocalException::class.java) {
            kotlinx.coroutines.runBlocking { disabled.service.capture(disabled.a, queued.operationId, disabled.target(), disabled.jpeg()) } }.failure)
        assertEquals(0, disabled.deliver().attempted)
        assertNull(disabled.disk.of(disabled.a).book)
    }

    @Test fun `nothing is declared before the owning operation has a final receipt`() = runTest {
        val f = AttachmentFixture()
        val owner = f.owner()
        val item = f.service.capture(f.a, owner.operationId, f.target(), f.jpeg())
        val result = f.deliver()
        assertTrue(f.server.opened.isEmpty() && f.server.uploads.isEmpty())
        assertEquals(1, result.pending); assertEquals(AttachmentState.WAITING_FOR_RECORD, f.item(item.documentId).state)
        assertEquals(AttachmentLabel.SAVED_ON_DEVICE, f.label(item.documentId))
    }

    @Test fun `an accepted operation's attachment is declared, sent once and verified only by STORED`() = runTest {
        val f = AttachmentFixture()
        val owner = f.owner()
        val bytes = f.jpeg()
        val item = f.service.capture(f.a, owner.operationId, f.target(), bytes)
        f.receipt(owner, "ACCEPTED")
        f.server.session = { f.server.response(it, AttachmentStorage.UPLOADING, 1, f.member, f.enrollment) }
        f.server.transfer = { _, _ -> AttachmentTransferResult.Settled(AttachmentStorage.STORED, 3) }
        val result = f.deliver()
        assertEquals(1, result.stored)
        assertEquals(listOf(item.documentId), f.server.uploads.map { it.first })
        assertArrayEquals(bytes, f.server.uploads.single().second, "the exact sealed bytes are sent")
        assertEquals(listOf("v1.grant-1"), f.server.grants)
        val stored = f.item(item.documentId)
        assertEquals(AttachmentState.STORED, stored.state); assertEquals(3L, stored.storageRevision)
        assertNull(f.disk.of(f.a).bytes[item.documentId], "verified bytes leave the device")
        assertEquals(AttachmentLabel.VERIFIED, f.label(item.documentId))
        // Duplicate completion: a verified attachment is never sent again.
        assertEquals(0, f.deliver().attempted)
        assertEquals(1, f.server.opened.size); assertEquals(1, f.server.uploads.size)
    }

    @Test fun `operation and attachment outcomes are independent and a failed transfer is retried from the sealed bytes`() = runTest {
        val f = AttachmentFixture()
        val owner = f.owner()
        val bytes = f.jpeg()
        val item = f.service.capture(f.a, owner.operationId, f.target(), bytes)
        f.receipt(owner, "ACCEPTED")
        val receipt = f.disk.of(f.a).owners.getValue(owner.operationId).acknowledgement!!.canonicalBytes
        f.server.session = { f.server.response(it, AttachmentStorage.UPLOADING, 1, f.member, f.enrollment) }
        // A truncated or mismatched body (422) or an expired capability (401) is a refused transfer.
        f.server.transfer = { _, _ -> AttachmentTransferResult.Rejected() }
        val first = f.deliver()
        assertEquals(1, first.pending); assertNotNull(first.nextAttemptMillis)
        val declared = f.item(item.documentId)
        assertEquals(AttachmentState.DECLARED, declared.state); assertEquals(1L, declared.retry.attempts)
        assertEquals(AttachmentLabel.SENDING, f.label(item.documentId), "the record is accepted, its attachment is not")
        assertArrayEquals(receipt, f.disk.of(f.a).owners.getValue(owner.operationId).acknowledgement!!.canonicalBytes, "the operation receipt never changes")
        // Not yet due: nothing is sent.
        assertEquals(0, f.deliver().attempted); assertEquals(1, f.server.uploads.size)
        // Due again: a fresh session (a new capability) and the same sealed bytes.
        f.elapsed += 60_000
        f.server.transfer = { _, _ -> AttachmentTransferResult.Settled(AttachmentStorage.STORED, 3) }
        assertEquals(1, f.deliver().stored)
        assertEquals(2, f.server.opened.size); assertEquals(listOf("v1.grant-1", "v1.grant-2"), f.server.grants)
        assertTrue(f.server.uploads.all { it.second.contentEquals(bytes) })
        assertEquals(AttachmentState.STORED, f.item(item.documentId).state)
    }

    @Test fun `a transport failure keeps the attachment pending and honours the server floor`() = runTest {
        val f = AttachmentFixture()
        val owner = f.owner(); f.receipt(owner, "ACCEPTED_FOR_REVIEW")
        val item = f.service.capture(f.a, owner.operationId, f.target(), f.jpeg())
        f.server.session = { throw EnterpriseQueueDeliveryFailure(3_600_000) }
        val result = f.deliver()
        assertTrue(f.server.uploads.isEmpty())
        val pending = f.item(item.documentId)
        assertEquals(AttachmentState.WAITING_FOR_RECORD, pending.state); assertEquals(1L, pending.retry.attempts)
        assertTrue(pending.retry.delayMillis >= 3_600_000); assertTrue(result.nextAttemptMillis!! >= 3_600_000)
        // A lost session answer or process death retries the same identifiers.
        f.elapsed += 3_600_000
        f.server.session = { f.server.response(it, AttachmentStorage.STORED, 3, f.member, f.enrollment) }
        assertEquals(1, f.deliver().stored)
        assertEquals(listOf(item.documentId, item.documentId), f.server.opened)
        assertTrue(f.server.uploads.isEmpty(), "a document the server already verified is not sent again")
    }

    @Test fun `RECEIVED is being verified and becomes verified only when the server says STORED`() = runTest {
        val f = AttachmentFixture()
        val owner = f.owner(); f.receipt(owner, "ACCEPTED")
        val item = f.service.capture(f.a, owner.operationId, f.target(), f.jpeg())
        f.server.session = { f.server.response(it, AttachmentStorage.UPLOADING, 1, f.member, f.enrollment) }
        f.server.transfer = { _, _ -> AttachmentTransferResult.Settled(AttachmentStorage.RECEIVED, 2) }
        f.deliver()
        assertEquals(AttachmentState.RECEIVED, f.item(item.documentId).state)
        assertEquals(AttachmentLabel.VERIFYING, f.label(item.documentId))
        assertNotNull(f.disk.of(f.a).bytes[item.documentId], "bytes stay until verification")
        f.elapsed += 60_000
        f.server.session = { f.server.response(it, AttachmentStorage.STORED, 3, f.member, f.enrollment) }
        f.deliver()
        assertEquals(AttachmentState.STORED, f.item(item.documentId).state)
        assertEquals(1, f.server.uploads.size)
    }

    @Test fun `refused records, refused content and lost files are explicit and keep unsent bytes`() = runTest {
        val f = AttachmentFixture()
        val rejected = f.owner(n = 900); val forbidden = f.owner(n = 910); val unsafe = f.owner(n = 920); val lost = f.owner(n = 930); val changed = f.owner(n = 940)
        val r = f.service.capture(f.a, rejected.operationId, f.target(), f.jpeg(fill = 1))
        val fb = f.service.capture(f.a, forbidden.operationId, f.target(), f.jpeg(fill = 2))
        val q = f.service.capture(f.a, unsafe.operationId, f.target(), f.jpeg(fill = 3))
        val l = f.service.capture(f.a, lost.operationId, f.target(), f.jpeg(fill = 4))
        val c = f.service.capture(f.a, changed.operationId, f.target(), f.jpeg(fill = 6))
        f.receipt(rejected, "REJECTED"); f.receipt(forbidden, "ACCEPTED"); f.receipt(unsafe, "ACCEPTED"); f.receipt(lost, "ACCEPTED"); f.receipt(changed, "ACCEPTED")
        f.disk.of(f.a).bytes.remove(l.documentId)
        f.disk.of(f.a).bytes[c.documentId] = f.jpeg(fill = 9)
        f.server.session = {
            when (it.documentId) {
                fb.documentId -> AttachmentSessionResult.Refused(AttachmentRefusal.FORBIDDEN)
                q.documentId -> f.server.response(it, AttachmentStorage.QUARANTINED, 3, f.member, f.enrollment)
                else -> f.server.response(it, AttachmentStorage.UPLOADING, 1, f.member, f.enrollment)
            }
        }
        f.server.transfer = { _, _ -> error("nothing may be sent") }
        val result = f.deliver()
        assertTrue(f.server.uploads.isEmpty())
        assertEquals(AttachmentRefusal.OWNER_REJECTED, f.item(r.documentId).refusal)
        assertEquals(AttachmentRefusal.FORBIDDEN, f.item(fb.documentId).refusal)
        assertEquals(AttachmentRefusal.QUARANTINED, f.item(q.documentId).refusal)
        assertEquals(AttachmentState.FILE_MISSING, f.item(l.documentId).state)
        assertEquals(AttachmentState.FILE_MISSING, f.item(c.documentId).state, "bytes that no longer match the declaration are never sent")
        assertEquals(3, result.refused); assertEquals(2, result.missing)
        assertFalse(r.documentId in f.server.opened, "a refused record's attachment is never declared")
        for (kept in listOf(r, fb, q)) assertNotNull(f.disk.of(f.a).bytes[kept.documentId], "refused bytes are kept (BR-OFFL-013)")
        assertEquals(AttachmentLabel.REFUSED, f.label(fb.documentId)); assertEquals(AttachmentLabel.FILE_MISSING, f.label(l.documentId))
        // Settled attachments are not attempted again.
        f.elapsed += 60_000
        assertEquals(0, f.deliver().attempted)
    }

    @Test fun `a lost compare-and-set records nothing and a session for another document is never used`() = runTest {
        val f = AttachmentFixture()
        val owner = f.owner(); f.receipt(owner, "ACCEPTED")
        val item = f.service.capture(f.a, owner.operationId, f.target(), f.jpeg())
        f.server.session = { f.server.response(it, AttachmentStorage.UPLOADING, 1, f.member, f.enrollment) }
        f.server.transfer = { _, _ -> AttachmentTransferResult.Settled(AttachmentStorage.STORED, 3) }
        f.disk.refuseTransitions = true
        f.deliver()
        assertEquals(AttachmentState.WAITING_FOR_RECORD, f.item(item.documentId).state)
        assertTrue(f.server.uploads.isEmpty(), "a concurrently changed attachment is re-read before any transfer")
        f.disk.refuseTransitions = false
        f.server.session = { f.server.response(EnterpriseAttachment(aid(5000), aid(5001), aid(5002), owner.operationId, f.target(), it.sha256, it.bytes, it.capturedAt),
            AttachmentStorage.UPLOADING, 1, f.member, f.enrollment) }
        f.deliver()
        assertTrue(f.server.uploads.isEmpty(), "an answer for another document is not used")
        assertEquals(AttachmentState.WAITING_FOR_RECORD, f.item(item.documentId).state)
    }
}
