package com.anpfuel.domain.enterprise.sync

import java.time.Instant

data class ReviewResolutionSnapshot(val operationId: String, val payloadDigest: String, val reviewItemId: String,
    val resolutionId: String, val reviewRevision: Long, val outcome: String, val reason: String,
    val resolvedAt: String, val entityRevision: Long?) {
    override fun toString(): String = "ReviewResolutionSnapshot(redacted)"
}

/** Resolution evidence never replaces the original final REVIEW acknowledgement. */
class ReviewResolution private constructor(private val value: ReviewResolutionSnapshot, raw: ByteArray) {
    private val encoded = raw.copyOf()
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    fun snapshot(): ReviewResolutionSnapshot = value.copy()
    override fun toString(): String = "ReviewResolution(redacted)"
    companion object {
        const val ACCEPTED = "ACCEPTED"
        const val REFUSED = "REFUSED"
        const val MAX_BYTES = 4096
        fun create(value: ReviewResolutionSnapshot): ReviewResolution {
            uuid(value.operationId); digest(value.payloadDigest); uuid(value.reviewItemId); uuid(value.resolutionId)
            if (value.reviewRevision != 2L || canonicalTime(value.resolvedAt) == Instant.parse("0001-01-01T00:00:00Z")) throw invalid()
            requireUnicode(value.reason)
            if (value.reason.codePointCount(0, value.reason.length) > 280 || value.reason.codePoints().anyMatch(Character::isISOControl) ||
                (value.reason.isNotEmpty() || value.outcome == REFUSED) && value.reason.isBlank()) throw invalid()
            when (value.outcome) {
                ACCEPTED -> if (value.entityRevision == null || value.entityRevision < 1) throw invalid()
                REFUSED -> if (value.entityRevision != null) throw invalid()
                else -> throw invalid()
            }
            val root = SyncJson.Object(mapOf("schema_version" to SyncJson.Integer("1"), "operation_id" to SyncJson.Text(value.operationId),
                "payload_digest" to SyncJson.Text(value.payloadDigest), "review_item_id" to SyncJson.Text(value.reviewItemId),
                "resolution_id" to SyncJson.Text(value.resolutionId), "review_revision" to SyncJson.Integer("2"),
                "outcome" to SyncJson.Text(value.outcome), "reason" to SyncJson.Text(value.reason), "resolved_at" to SyncJson.Text(value.resolvedAt),
                "entity_revision" to (value.entityRevision?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null)))
            val bytes = CanonicalJson.encode(root)
            if (bytes.size > MAX_BYTES) throw invalid(SyncFailure.LIMIT)
            return ReviewResolution(value.copy(), bytes)
        }
        fun parse(raw: ByteArray): ReviewResolution = decode(objectValue(CanonicalJson.parse(raw, MAX_BYTES)))
        internal fun decode(root: SyncJson.Object): ReviewResolution {
            exactKeys(root, setOf("schema_version", "operation_id", "payload_digest", "review_item_id", "resolution_id", "review_revision", "outcome", "reason", "resolved_at", "entity_revision")); version(root)
            return create(ReviewResolutionSnapshot(text(root, "operation_id"), text(root, "payload_digest"), text(root, "review_item_id"),
                text(root, "resolution_id"), positive(root["review_revision"]), text(root, "outcome"), text(root, "reason"),
                text(root, "resolved_at"), nullablePositive(root["entity_revision"])))
        }
    }
}
