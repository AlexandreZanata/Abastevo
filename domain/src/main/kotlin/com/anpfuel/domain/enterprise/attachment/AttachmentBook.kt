package com.anpfuel.domain.enterprise.attachment

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.QueueRetryState
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.sync.SyncResult
import java.security.MessageDigest
import java.time.Instant

/*
 * The device's attachment index (EMP-OFFL-005, B-BR-EMP-415..419, EDR-267..269): one sealed record per partition
 * naming each captured attachment, its owning operation, its link target, its local state and its own retry state.
 * It decides nothing on the server; only server answers move an attachment forward.
 */

/** Local refusals at capture; never a server outcome. */
enum class AttachmentLocalFailure {
    /** Not a C1 synthetic JPEG (start-of-image and end-of-image markers). */
    NOT_JPEG,
    /** Empty or above the 5 MiB EMP-DOC-002 budget. */
    SIZE,
    /** The owning operation is not saved in this partition's queue. */
    UNKNOWN_OPERATION,
    /** The identifier already exists in the index. */
    DUPLICATE,
    /** The index reached its entry bound. */
    FULL,
    /** Unsettled attachments already hold the partition's byte budget. */
    BUDGET,
    /** The index belongs to another membership or enrollment. */
    FOREIGN,
    /** Attachment capture and delivery are switched off (`ENTERPRISE_ATTACHMENT_UPLOAD_ENABLED`). */
    DISABLED,
}

class AttachmentLocalException(val failure: AttachmentLocalFailure) : IllegalStateException("attachment local: ${failure.name.lowercase()}")

/** Local states (EDR-268). Only `STORED` means the server verified the bytes. */
enum class AttachmentState { WAITING_FOR_RECORD, DECLARED, RECEIVED, STORED, REFUSED, FILE_MISSING }

/** Closed refusal codes of a `REFUSED` attachment. */
enum class AttachmentRefusal { OWNER_REJECTED, FORBIDDEN, VALIDATION_FAILED, CONFLICT, OPERATION_ID_REUSED, QUARANTINED, FAILED }

/** One EMP-DOC-003 link target, fixed at capture (EDR-269). */
class AttachmentTarget(val kind: DocumentTargetKind, val id: String) {
    init { EnterprisePartition.requireId(id) }
    override fun equals(other: Any?): Boolean = other is AttachmentTarget && kind == other.kind && id == other.id
    override fun hashCode(): Int = 31 * kind.hashCode() + id.hashCode()
    override fun toString(): String = "AttachmentTarget(redacted)"
}

/** One captured attachment. The three identifiers and the declaration never change. */
class EnterpriseAttachment(
    val documentId: String,
    val declarationId: String,
    val linkId: String,
    val ownerOperationId: String,
    val target: AttachmentTarget,
    val sha256: String,
    val bytes: Long,
    val capturedAt: Instant,
    val state: AttachmentState = AttachmentState.WAITING_FOR_RECORD,
    val refusal: AttachmentRefusal? = null,
    val storageRevision: Long? = null,
    val retry: QueueRetryState = QueueRetryState(),
) {
    init {
        for (id in listOf(documentId, declarationId, linkId, ownerOperationId)) EnterprisePartition.requireId(id)
        require(listOf(documentId, declarationId, linkId).distinct().size == 3)
        require(sha256.length == 64 && sha256.all { it in '0'..'9' || it in 'a'..'f' } && bytes in 1..AttachmentPolicy.MAX_BYTES)
        require((refusal != null) == (state == AttachmentState.REFUSED) && (storageRevision == null || storageRevision >= 1))
    }
    /** Holds sealed bytes on the device: everything not verified and not missing. */
    val holdsBytes: Boolean get() = state != AttachmentState.STORED && state != AttachmentState.FILE_MISSING
    /** Settled attachments are never delivered again. */
    val settled: Boolean get() = state == AttachmentState.STORED || state == AttachmentState.REFUSED || state == AttachmentState.FILE_MISSING
    fun copy(state: AttachmentState = this.state, refusal: AttachmentRefusal? = this.refusal, storageRevision: Long? = this.storageRevision,
        retry: QueueRetryState = this.retry): EnterpriseAttachment = EnterpriseAttachment(documentId, declarationId, linkId, ownerOperationId,
        target, sha256, bytes, capturedAt, state, refusal, storageRevision, retry)
    /** Structural equality for compare-and-set; it grants no authority. */
    fun sameAs(other: EnterpriseAttachment): Boolean = sameDeclaration(other) && state == other.state && refusal == other.refusal &&
        storageRevision == other.storageRevision && retry.sameAs(other.retry)
    /** The immutable part fixed at capture. */
    fun sameDeclaration(other: EnterpriseAttachment): Boolean = documentId == other.documentId && declarationId == other.declarationId &&
        linkId == other.linkId && ownerOperationId == other.ownerOperationId && target == other.target && sha256 == other.sha256 &&
        bytes == other.bytes && capturedAt == other.capturedAt
    internal val identifiers: List<String> get() = listOf(documentId, declarationId, linkId)
    override fun toString(): String = "EnterpriseAttachment(redacted)"
}

/** The versioned `attachment-book` index of one partition (EDR-267). */
class AttachmentBook private constructor(val membershipId: String, val enrollmentId: String, items: List<EnterpriseAttachment>) {
    private val values = items.toList()
    val items: List<EnterpriseAttachment> get() = values.toList()
    fun item(documentId: String): EnterpriseAttachment? = values.firstOrNull { it.documentId == documentId }
    /** Sealed bytes held by unsettled attachments. */
    val pendingBytes: Long get() = values.filter { it.holdsBytes }.sumOf { it.bytes }
    init {
        EnterprisePartition.requireId(membershipId); EnterprisePartition.requireId(enrollmentId)
        require(values.size <= MAX_ITEMS)
        val ids = values.flatMap { it.identifiers }
        require(ids.distinct().size == ids.size)
    }

    /** A new waiting attachment; nothing is ever evicted to make room (BR-OFFL-013). */
    fun withCapture(item: EnterpriseAttachment): AttachmentBook {
        require(item.state == AttachmentState.WAITING_FOR_RECORD && item.retry.initial && item.storageRevision == null)
        val known = values.flatMap { it.identifiers }.toSet()
        if (item.identifiers.any { it in known }) throw AttachmentLocalException(AttachmentLocalFailure.DUPLICATE)
        if (values.size >= MAX_ITEMS) throw AttachmentLocalException(AttachmentLocalFailure.FULL)
        if (pendingBytes + item.bytes > AttachmentPolicy.MAX_PENDING_BYTES) throw AttachmentLocalException(AttachmentLocalFailure.BUDGET)
        return AttachmentBook(membershipId, enrollmentId, values + item)
    }

    /** Compare-and-set of one attachment; null when the stored one differs from [expected]. */
    fun replace(expected: EnterpriseAttachment, next: EnterpriseAttachment): AttachmentBook? {
        require(next.sameDeclaration(expected))
        val stored = item(expected.documentId) ?: return null
        if (!stored.sameAs(expected)) return null
        return AttachmentBook(membershipId, enrollmentId, values.map { if (it.documentId == next.documentId) next else it })
    }

    /** Refuses an index bound to another membership or enrollment. */
    fun requireBinding(membershipId: String, enrollmentId: String): AttachmentBook {
        if (membershipId != this.membershipId || enrollmentId != this.enrollmentId) throw AttachmentLocalException(AttachmentLocalFailure.FOREIGN)
        return this
    }

    val canonicalBytes: ByteArray get() = CanonicalJson.encode(SyncJson.Object(mapOf(
        "format" to SyncJson.Text(FORMAT), "version" to SyncJson.Integer(VERSION.toString()),
        "membership_id" to SyncJson.Text(membershipId), "enrollment_id" to SyncJson.Text(enrollmentId),
        "attachments" to SyncJson.Array(values.map(::itemJson)),
    )))
    override fun toString(): String = "EnterpriseAttachmentBook(redacted)"

    companion object {
        const val FORMAT = "attachment-book"
        const val VERSION = 1L
        const val MAX_ITEMS = 500
        const val MAX_BYTES = 1024 * 1024
        /** Fixed identifier of the sealed index record. */
        const val RECORD_ID = "a7ac4e00-0000-4000-8000-000000000415"
        fun empty(membershipId: String, enrollmentId: String): AttachmentBook = AttachmentBook(membershipId, enrollmentId, emptyList())
        private val ITEM_KEYS = setOf("document_id", "declaration_id", "link_id", "owner_operation_id", "target_kind", "target_id", "sha256",
            "bytes", "captured_at", "state", "refusal", "storage_revision", "retry")
        private val RETRY_KEYS = setOf("attempts", "clock", "delay_millis")
        private val CLOCK_KEYS = setOf("boot", "elapsed_millis", "wall_millis")

        /** Strict decode; an unknown format, version or spelling is refused, never guessed. */
        fun decode(raw: ByteArray): AttachmentBook {
            val root = obj(CanonicalJson.parseLarge(raw, MAX_BYTES), setOf("format", "version", "membership_id", "enrollment_id", "attachments"))
            require(text(root, "format") == FORMAT && (root.fields["version"] as? SyncJson.Integer)?.lexeme == VERSION.toString())
            val items = (root.fields["attachments"] as? SyncJson.Array ?: throw IllegalArgumentException()).values.map { value ->
                val item = obj(value, ITEM_KEYS)
                val retry = obj(item.fields["retry"], RETRY_KEYS)
                val clock = retry.fields["clock"].let { if (it === SyncJson.Null) null else obj(it, CLOCK_KEYS) }?.let {
                    EnterpriseDeviceClock(text(it, "boot"), long(it, "elapsed_millis"), long(it, "wall_millis")) }
                EnterpriseAttachment(text(item, "document_id"), text(item, "declaration_id"), text(item, "link_id"), text(item, "owner_operation_id"),
                    AttachmentTarget(closed(DocumentTargetKind.entries, text(item, "target_kind")) { it.name }, text(item, "target_id")),
                    text(item, "sha256"), long(item, "bytes"), instant(text(item, "captured_at")),
                    closed(AttachmentState.entries, text(item, "state")) { it.name },
                    nullable(item, "refusal")?.let { raw -> closed(AttachmentRefusal.entries, raw) { it.name } },
                    item.fields["storage_revision"].let { if (it === SyncJson.Null) null else long(item, "storage_revision") },
                    QueueRetryState(long(retry, "attempts"), clock, long(retry, "delay_millis")))
            }
            val book = AttachmentBook(text(root, "membership_id"), text(root, "enrollment_id"), items)
            // Only the exact canonical spelling is a stored index.
            require(book.canonicalBytes.contentEquals(raw))
            return book
        }

        private fun <T> closed(values: List<T>, raw: String, name: (T) -> String): T =
            values.firstOrNull { name(it) == raw } ?: throw IllegalArgumentException("Invalid attachment book")
        private fun obj(value: SyncJson?, keys: Set<String>): SyncJson.Object {
            val result = value as? SyncJson.Object ?: throw IllegalArgumentException("Invalid attachment book")
            require(result.fields.keys == keys) { "Invalid attachment book" }
            return result
        }
        private fun text(value: SyncJson.Object, key: String): String = (value.fields[key] as? SyncJson.Text)?.value ?: throw IllegalArgumentException()
        private fun nullable(value: SyncJson.Object, key: String): String? = value.fields[key].let { if (it === SyncJson.Null) null else text(value, key) }
        private fun long(value: SyncJson.Object, key: String): Long = (value.fields[key] as? SyncJson.Integer)?.lexeme?.toLongOrNull() ?: throw IllegalArgumentException()
        private fun instant(raw: String): Instant {
            val at = EnterpriseTime.parseInstant(raw)
            require(EnterpriseTime.formatInstant(at) == raw)
            return at
        }
        private fun itemJson(item: EnterpriseAttachment): SyncJson = SyncJson.Object(mapOf(
            "document_id" to SyncJson.Text(item.documentId), "declaration_id" to SyncJson.Text(item.declarationId),
            "link_id" to SyncJson.Text(item.linkId), "owner_operation_id" to SyncJson.Text(item.ownerOperationId),
            "target_kind" to SyncJson.Text(item.target.kind.name), "target_id" to SyncJson.Text(item.target.id),
            "sha256" to SyncJson.Text(item.sha256), "bytes" to SyncJson.Integer(item.bytes.toString()),
            "captured_at" to SyncJson.Text(EnterpriseTime.formatInstant(item.capturedAt)), "state" to SyncJson.Text(item.state.name),
            "refusal" to (item.refusal?.let { SyncJson.Text(it.name) } ?: SyncJson.Null),
            "storage_revision" to (item.storageRevision?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null),
            "retry" to SyncJson.Object(mapOf("attempts" to SyncJson.Integer(item.retry.attempts.toString()),
                "clock" to (item.retry.clock?.let { SyncJson.Object(mapOf("boot" to SyncJson.Text(it.boot),
                    "elapsed_millis" to SyncJson.Integer(it.elapsedMillis.toString()), "wall_millis" to SyncJson.Integer(it.wallMillis.toString()))) } ?: SyncJson.Null),
                "delay_millis" to SyncJson.Integer(item.retry.delayMillis.toString()))),
        ))
    }
}

/** C1 capture policy (B-BR-EMP-415). */
object AttachmentPolicy {
    /** The EMP-DOC-002 C1 JPEG budget. */
    const val MAX_BYTES = 5L * 1024 * 1024
    /** Unsettled sealed bytes per partition (EDR-267). */
    const val MAX_PENDING_BYTES = 16L * 1024 * 1024
    const val KIND = "RECEIPT"
    const val MEDIA_TYPE = "image/jpeg"
    /** Refuses anything but a JPEG within the budget; returns its lowercase SHA-256. */
    fun checkJpeg(bytes: ByteArray): String {
        if (bytes.isEmpty() || bytes.size > MAX_BYTES) throw AttachmentLocalException(AttachmentLocalFailure.SIZE)
        val start = bytes.size >= 4 && bytes[0] == 0xff.toByte() && bytes[1] == 0xd8.toByte() && bytes[2] == 0xff.toByte()
        val end = bytes.size >= 4 && bytes[bytes.size - 2] == 0xff.toByte() && bytes[bytes.size - 1] == 0xd9.toByte()
        if (!start || !end) throw AttachmentLocalException(AttachmentLocalFailure.NOT_JPEG)
        return sha256(bytes)
    }
    fun sha256(bytes: ByteArray): String = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it.toInt() and 255) }
}

/** What the owning operation's receipt allows (B-BR-EMP-416). */
enum class AttachmentOwnerGate { WAIT, DECLARE, REFUSE }

/** Pure delivery decisions (EDR-268); the application service performs the I/O. */
object AttachmentDelivery {
    fun ownerGate(receipt: OperationAcknowledgement?): AttachmentOwnerGate = when {
        receipt == null || !receipt.isFinal -> AttachmentOwnerGate.WAIT
        receipt.result == SyncResult.REJECTED -> AttachmentOwnerGate.REFUSE
        else -> AttachmentOwnerGate.DECLARE
    }
    /** The state a session or ingress answer establishes; never `STORED` without the server saying so. */
    fun settle(item: EnterpriseAttachment, storage: AttachmentStorage, revision: Long): EnterpriseAttachment {
        require(revision >= 1)
        if (item.settled) return item
        return when (storage) {
            AttachmentStorage.UPLOADING -> item.copy(state = AttachmentState.DECLARED, storageRevision = revision)
            AttachmentStorage.RECEIVED -> item.copy(state = AttachmentState.RECEIVED, storageRevision = revision)
            AttachmentStorage.STORED -> item.copy(state = AttachmentState.STORED, storageRevision = revision, retry = QueueRetryState())
            AttachmentStorage.QUARANTINED -> item.copy(state = AttachmentState.REFUSED, refusal = AttachmentRefusal.QUARANTINED, storageRevision = revision)
            AttachmentStorage.FAILED, AttachmentStorage.REPLACED, AttachmentStorage.DELETED ->
                item.copy(state = AttachmentState.REFUSED, refusal = AttachmentRefusal.FAILED, storageRevision = revision)
        }
    }
}

/** What the person sees for one attachment, independent of its operation's sync state. */
enum class AttachmentLabel { SAVED_ON_DEVICE, SENDING, VERIFYING, VERIFIED, REFUSED, FILE_MISSING, NEEDS_ATTENTION }

class AttachmentView(val attachment: EnterpriseAttachment, val label: AttachmentLabel, val refusal: AttachmentRefusal?) {
    override fun toString(): String = "AttachmentView(redacted)"
}

object AttachmentPresentation {
    /** Attempts after which a pending attachment asks for attention. */
    const val ATTENTION_ATTEMPTS = 3L
    fun view(item: EnterpriseAttachment, ownerReceipt: OperationAcknowledgement?): AttachmentView {
        val label = when (item.state) {
            AttachmentState.STORED -> AttachmentLabel.VERIFIED
            AttachmentState.REFUSED -> AttachmentLabel.REFUSED
            AttachmentState.FILE_MISSING -> AttachmentLabel.FILE_MISSING
            else -> when {
                item.retry.attempts >= ATTENTION_ATTEMPTS -> AttachmentLabel.NEEDS_ATTENTION
                item.state == AttachmentState.RECEIVED -> AttachmentLabel.VERIFYING
                item.state == AttachmentState.DECLARED -> AttachmentLabel.SENDING
                AttachmentDelivery.ownerGate(ownerReceipt) == AttachmentOwnerGate.WAIT -> AttachmentLabel.SAVED_ON_DEVICE
                else -> AttachmentLabel.SENDING
            }
        }
        return AttachmentView(item, label, item.refusal)
    }
}
