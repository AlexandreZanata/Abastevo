package com.anpfuel.domain.enterprise.attachment

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.QueueRetryState
import com.anpfuel.domain.enterprise.sync.SyncBinding
import java.security.MessageDigest
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

internal object AttachmentFixture {
    fun id(value: Int) = "7b0e2a54-0000-4000-8000-" + value.toString().padStart(12, '0')
    const val MEMBER = "7b0e2a54-0000-4000-8000-000000041503"
    const val ENROLLMENT = "7b0e2a54-0000-4000-8000-000000041504"
    val captured: Instant = Instant.parse("2026-11-15T12:00:00Z")
    fun jpeg(size: Int = 64, fill: Byte = 7): ByteArray = ByteArray(size) { fill }.also {
        it[0] = 0xff.toByte(); it[1] = 0xd8.toByte(); it[2] = 0xff.toByte(); it[size - 2] = 0xff.toByte(); it[size - 1] = 0xd9.toByte()
    }
    fun sha(bytes: ByteArray) = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) }
    fun item(n: Int, bytes: Long = 64, owner: Int = 900, state: AttachmentState = AttachmentState.WAITING_FOR_RECORD,
        refusal: AttachmentRefusal? = null, revision: Long? = null, retry: QueueRetryState = QueueRetryState()) =
        EnterpriseAttachment(id(n), id(n + 1), id(n + 2), id(owner), AttachmentTarget(DocumentTargetKind.METER_READING, id(n + 3)),
            "ab".repeat(32), bytes, captured, state, refusal, revision, retry)
    private const val ORIGIN = "https://api.anpfuel.example.invalid"
    private const val ACCOUNT = "7b0e2a54-0000-4000-8000-000000041500"
    private const val ORG = "7b0e2a54-0000-4000-8000-000000041509"
    fun owner(operationId: String = id(900)): EnterpriseOperation = EnterpriseOperation.seal(("""{"schema_version":1,"operation_id":"$operationId","owner_account_id":"$ACCOUNT","api_origin":"$ORIGIN","org_id":"$ORG","membership_id":"$MEMBER","enrollment_id":"$ENROLLMENT","type":"validation.record","operation_schema":1,"entity":{"type":"record","id":"${id(901)}","expected_revision":null},"occurred_at":"2026-11-15T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"SYNTHETIC"}}""").toByteArray(),
        SyncBinding.of(ACCOUNT, ORIGIN, ORG))
    fun receipt(result: String, operation: EnterpriseOperation = owner()): OperationAcknowledgement {
        val fields = when (result) {
            "ACCEPTED" -> """"code":null,"entity_revision":1,"received_at":"2026-11-15T12:00:01Z","review_item_id":null"""
            "ACCEPTED_FOR_REVIEW" -> """"code":"STALE_REVISION","entity_revision":null,"received_at":"2026-11-15T12:00:01Z","review_item_id":"${id(950)}""""
            else -> """"code":"MALFORMED","entity_revision":null,"received_at":"2026-11-15T12:00:01Z","review_item_id":null"""
        }
        return OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"$result",$fields,"warnings":[]}""").toByteArray(), operation)
    }
}

class AttachmentBookTest {
    private val f = AttachmentFixture

    @Test fun `only a JPEG within the C1 budget is captured and its digest is the exact bytes`() {
        val bytes = f.jpeg()
        assertEquals(f.sha(bytes), AttachmentPolicy.checkJpeg(bytes))
        val cases = listOf(ByteArray(0) to AttachmentLocalFailure.SIZE, f.jpeg().also { it[0] = 0x89.toByte() } to AttachmentLocalFailure.NOT_JPEG,
            f.jpeg().also { it[2] = 0 } to AttachmentLocalFailure.NOT_JPEG, f.jpeg().also { it[it.size - 1] = 0 } to AttachmentLocalFailure.NOT_JPEG,
            byteArrayOf(0xff.toByte(), 0xd8.toByte(), 0xff.toByte()) to AttachmentLocalFailure.NOT_JPEG,
            f.jpeg((AttachmentPolicy.MAX_BYTES + 1).toInt()) to AttachmentLocalFailure.SIZE)
        for ((input, failure) in cases) assertEquals(failure, assertThrows(AttachmentLocalException::class.java) { AttachmentPolicy.checkJpeg(input) }.failure)
        val exact = f.jpeg(AttachmentPolicy.MAX_BYTES.toInt())
        assertEquals(f.sha(exact), AttachmentPolicy.checkJpeg(exact))
    }

    @Test fun `capture is refused for duplicates, a full index and the unsettled byte budget, and nothing is evicted`() {
        var book = AttachmentBook.empty(f.MEMBER, f.ENROLLMENT)
        val five = AttachmentPolicy.MAX_BYTES
        book = book.withCapture(f.item(100, five)).withCapture(f.item(110, five)).withCapture(f.item(120, five))
        assertEquals(3 * five, book.pendingBytes)
        assertEquals(AttachmentLocalFailure.BUDGET, assertThrows(AttachmentLocalException::class.java) { book.withCapture(f.item(130, five)) }.failure)
        assertEquals(AttachmentLocalFailure.DUPLICATE, assertThrows(AttachmentLocalException::class.java) { book.withCapture(f.item(100)) }.failure)
        // Any identifier of a new attachment colliding with an existing one is a duplicate.
        val collides = EnterpriseAttachment(f.id(200), f.id(101), f.id(202), f.id(900), AttachmentTarget(DocumentTargetKind.INCIDENT, f.id(203)),
            "ab".repeat(32), 10, f.captured)
        assertEquals(AttachmentLocalFailure.DUPLICATE, assertThrows(AttachmentLocalException::class.java) { book.withCapture(collides) }.failure)
        // Verified bytes leave the device, so the budget frees; refused ones are kept (BR-OFFL-013).
        val first = book.item(f.id(100))!!
        book = book.replace(first, first.copy(state = AttachmentState.STORED, storageRevision = 3))!!
        assertEquals(2 * five, book.pendingBytes)
        val second = book.item(f.id(110))!!
        book = book.replace(second, second.copy(state = AttachmentState.REFUSED, refusal = AttachmentRefusal.QUARANTINED))!!
        assertEquals(2 * five, book.pendingBytes)
        book = book.withCapture(f.item(130, five))
        assertEquals(4, book.items.size)
        var full = AttachmentBook.empty(f.MEMBER, f.ENROLLMENT)
        for (n in 0 until AttachmentBook.MAX_ITEMS) full = full.withCapture(f.item(10_000 + n * 4, 1))
        assertEquals(AttachmentLocalFailure.FULL, assertThrows(AttachmentLocalException::class.java) { full.withCapture(f.item(100)) }.failure)
    }

    @Test fun `transitions are compare-and-set and never change the declaration`() {
        val book = AttachmentBook.empty(f.MEMBER, f.ENROLLMENT).withCapture(f.item(100))
        val stored = book.item(f.id(100))!!
        val retried = stored.copy(retry = QueueRetryState(1, EnterpriseDeviceClock("boot-1", 10, 20), 30_000))
        val next = book.replace(stored, retried)!!
        assertNull(next.replace(stored, stored.copy(state = AttachmentState.DECLARED, storageRevision = 1)), "a stale expectation writes nothing")
        assertNull(book.replace(f.item(500), f.item(500)), "an unknown attachment writes nothing")
        val other = EnterpriseAttachment(stored.documentId, stored.declarationId, stored.linkId, stored.ownerOperationId, stored.target,
            "cd".repeat(32), stored.bytes, stored.capturedAt)
        assertThrows(IllegalArgumentException::class.java) { book.replace(stored, other) }
        assertTrue(next.item(f.id(100))!!.sameAs(retried))
    }

    @Test fun `the index has one strict canonical spelling bound to its membership and enrollment`() {
        var book = AttachmentBook.empty(f.MEMBER, f.ENROLLMENT)
        val states = listOf(f.item(100), f.item(110, state = AttachmentState.DECLARED, revision = 1, retry = QueueRetryState(2, EnterpriseDeviceClock("boot-3", 7, 9), 60_000)),
            f.item(120, state = AttachmentState.RECEIVED, revision = 2), f.item(130, state = AttachmentState.STORED, revision = 3),
            f.item(140, state = AttachmentState.REFUSED, refusal = AttachmentRefusal.OWNER_REJECTED), f.item(150, state = AttachmentState.FILE_MISSING))
        for (item in states) book = book.withCapture(EnterpriseAttachment(item.documentId, item.declarationId, item.linkId, item.ownerOperationId,
            item.target, item.sha256, item.bytes, item.capturedAt)).let { b -> b.replace(b.item(item.documentId)!!, item)!! }
        val raw = book.canonicalBytes
        val decoded = AttachmentBook.decode(raw)
        assertArrayEquals(raw, decoded.canonicalBytes)
        for (item in states) assertTrue(decoded.item(item.documentId)!!.sameAs(item))
        assertEquals(AttachmentLocalFailure.FOREIGN, assertThrows(AttachmentLocalException::class.java) { decoded.requireBinding(f.id(1), f.ENROLLMENT) }.failure)
        assertEquals(AttachmentLocalFailure.FOREIGN, assertThrows(AttachmentLocalException::class.java) { decoded.requireBinding(f.MEMBER, f.id(1)) }.failure)
        val text = String(raw)
        for (bad in listOf(text.replace("\"version\":1", "\"version\":2"), text.replace("attachment-book", "trip-book"), text.replaceFirst("{", "{ "),
            text.replace("\"WAITING_FOR_RECORD\"", "\"SENT\""), text.replace("\"OWNER_REJECTED\"", "\"NOT_FOUND\""), "{}", text + " ")) {
            assertNotEquals(text, bad, "every mutation must change the stored spelling")
            assertThrows(IllegalArgumentException::class.java) { AttachmentBook.decode(bad.toByteArray()) }
        }
    }

    @Test fun `the owner receipt gates the declaration and server answers alone settle an attachment`() {
        assertEquals(AttachmentOwnerGate.WAIT, AttachmentDelivery.ownerGate(null))
        assertEquals(AttachmentOwnerGate.DECLARE, AttachmentDelivery.ownerGate(f.receipt("ACCEPTED")))
        assertEquals(AttachmentOwnerGate.DECLARE, AttachmentDelivery.ownerGate(f.receipt("ACCEPTED_FOR_REVIEW")))
        assertEquals(AttachmentOwnerGate.REFUSE, AttachmentDelivery.ownerGate(f.receipt("REJECTED")))
        val waiting = f.item(100)
        assertEquals(AttachmentState.DECLARED, AttachmentDelivery.settle(waiting, AttachmentStorage.UPLOADING, 1).state)
        assertEquals(AttachmentState.RECEIVED, AttachmentDelivery.settle(waiting, AttachmentStorage.RECEIVED, 2).state)
        val stored = AttachmentDelivery.settle(waiting, AttachmentStorage.STORED, 3)
        assertEquals(AttachmentState.STORED, stored.state); assertEquals(3L, stored.storageRevision)
        assertEquals(AttachmentRefusal.QUARANTINED, AttachmentDelivery.settle(waiting, AttachmentStorage.QUARANTINED, 3).refusal)
        for (terminal in listOf(AttachmentStorage.FAILED, AttachmentStorage.REPLACED, AttachmentStorage.DELETED))
            assertEquals(AttachmentRefusal.FAILED, AttachmentDelivery.settle(waiting, terminal, 2).refusal)
        // A settled attachment never moves again.
        assertSame(stored, AttachmentDelivery.settle(stored, AttachmentStorage.UPLOADING, 1))
    }

    @Test fun `the label of an attachment is its own and verified only after STORED`() {
        fun label(item: EnterpriseAttachment, receipt: OperationAcknowledgement? = null) = AttachmentPresentation.view(item, receipt).label
        assertEquals(AttachmentLabel.SAVED_ON_DEVICE, label(f.item(100)))
        // The record accepted by the company does not make its attachment verified (B-BR-EMP-416).
        assertEquals(AttachmentLabel.SENDING, label(f.item(100), f.receipt("ACCEPTED")))
        assertEquals(AttachmentLabel.SENDING, label(f.item(100, state = AttachmentState.DECLARED, revision = 1), f.receipt("ACCEPTED")))
        assertEquals(AttachmentLabel.VERIFYING, label(f.item(100, state = AttachmentState.RECEIVED, revision = 2), f.receipt("ACCEPTED")))
        assertEquals(AttachmentLabel.VERIFIED, label(f.item(100, state = AttachmentState.STORED, revision = 3), f.receipt("ACCEPTED")))
        val refused = AttachmentPresentation.view(f.item(100, state = AttachmentState.REFUSED, refusal = AttachmentRefusal.FORBIDDEN), f.receipt("ACCEPTED"))
        assertEquals(AttachmentLabel.REFUSED, refused.label); assertEquals(AttachmentRefusal.FORBIDDEN, refused.refusal)
        assertEquals(AttachmentLabel.FILE_MISSING, label(f.item(100, state = AttachmentState.FILE_MISSING)))
        val tired = f.item(100, state = AttachmentState.DECLARED, revision = 1, retry = QueueRetryState(3, EnterpriseDeviceClock("boot-1", 1, 1), 120_000))
        assertEquals(AttachmentLabel.NEEDS_ATTENTION, label(tired, f.receipt("ACCEPTED")))
        assertEquals("AttachmentView(redacted)", refused.toString())
        assertFalse(f.item(100).toString().contains(f.id(100)))
    }
}
