package com.anpfuel.domain.enterprise.sync

import java.time.Instant

/** Outcome value validation only; an untrusted receipt is never proof of server authority. */
class OperationAcknowledgement private constructor(
    val operationId: String,
    val payloadDigest: String,
    val result: SyncResult,
    val code: String?,
    val entityRevision: Long?,
    val receivedAt: Instant?,
    val reviewItemId: String?,
    warnings: List<String>,
    canonical: ByteArray,
) {
    private val warningValues = warnings.toList()
    private val encoded = canonical.copyOf()
    val warnings: List<String> get() = warningValues.toList()
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    val isFinal: Boolean get() = result != SyncResult.RETRY
    fun matches(operation: EnterpriseOperation): Boolean = operation.operationId == operationId && operation.payloadDigest == payloadDigest
    override fun toString(): String = "OperationAcknowledgement(redacted)"

    companion object {
        private val keys = setOf("schema_version", "operation_id", "payload_digest", "result", "code", "entity_revision", "received_at", "review_item_id", "warnings")
        private val reviewCodes = setOf("ORPHAN_REFERENCE", "ACCESS_ENDED_BEFORE_RECEIPT", "PERMISSION_CHANGED", "ASSET_UNAVAILABLE", "ASSET_BUSY", "TRIP_ALREADY_CLOSED", "PERIOD_LOCKED", "STALE_REVISION", "ORG_SUSPENDED_GRACE", "PLAN_CHANGED")
        private val rejectionCodes = setOf("OPERATION_ID_REUSED", "MALFORMED", "DEVICE_REVOKED", "MEMBERSHIP_ENDED", "INTAKE_WINDOW_CLOSED", "FORBIDDEN", "ORG_SUSPENDED")
        private val retryCodes = setOf("DEPENDENCY_PENDING", "UNAVAILABLE")
        fun parse(raw: ByteArray, expected: EnterpriseOperation, limits: SyncLimits = SyncLimits.DEFAULT): OperationAcknowledgement {
            val acknowledgement = decode(raw, limits)
            if (acknowledgement.operationId != expected.operationId) throw invalid(SyncFailure.BINDING)
            if (acknowledgement.payloadDigest != expected.payloadDigest) throw invalid(SyncFailure.DIGEST)
            return acknowledgement
        }
        private fun decode(raw: ByteArray, limits: SyncLimits): OperationAcknowledgement {
            val root = objectValue(CanonicalJson.parse(raw, limits.maxOperationBytes, limits.maxOperationDepth))
            exactKeys(root, keys); version(root)
            val operationId = uuid(text(root, "operation_id"))
            val payloadDigest = text(root, "payload_digest").also(::digest)
            val result = SyncResult.entries.firstOrNull { it.name == text(root, "result") } ?: throw invalid()
            val code = nullableText(root["code"])
            val revision = nullablePositive(root["entity_revision"])
            val receipt = nullableText(root["received_at"])?.let(::canonicalTime)
            val review = nullableText(root["review_item_id"])?.let(::uuid)
            val warnings = array(root, "warnings").map(::string).sorted()
            if (warnings.any { it != "CLOCK_SKEW" } || warnings.toSet().size != warnings.size) throw invalid()
            if (result == SyncResult.RETRY) {
                if (code !in retryCodes || revision != null || receipt != null || review != null) throw invalid()
            } else {
                if (receipt == null || receipt.toString() == "0001-01-01T00:00:00Z") throw invalid()
                when (result) {
                    SyncResult.ACCEPTED -> if (code != null || revision == null || review != null) throw invalid()
                    SyncResult.ACCEPTED_FOR_REVIEW -> if (code !in reviewCodes || review == null) throw invalid()
                    SyncResult.REJECTED -> if (code !in rejectionCodes || revision != null || review != null) throw invalid()
                    SyncResult.RETRY -> throw invalid()
                }
            }
            val normalized = SyncJson.Object(root.fields + ("warnings" to SyncJson.Array(warnings.map { SyncJson.Text(it) })))
            val canonical = CanonicalJson.encode(normalized, limits.maxOperationDepth)
            if (canonical.size > limits.maxOperationBytes) throw invalid(SyncFailure.LIMIT)
            return OperationAcknowledgement(operationId, payloadDigest, result, code, revision, receipt, review, warnings, canonical)
        }
    }
}

enum class SyncResult { ACCEPTED, ACCEPTED_FOR_REVIEW, REJECTED, RETRY }
private fun nullableText(value: SyncJson?): String? = if (value === SyncJson.Null) null else string(value)
