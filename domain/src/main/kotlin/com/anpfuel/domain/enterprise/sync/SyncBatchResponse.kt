package com.anpfuel.domain.enterprise.sync

/** Complete response preflight. No prefix is returned for an invalid suffix. */
class SyncBatchResponse private constructor(acknowledgements: List<OperationAcknowledgement>) {
    private val values = acknowledgements.toList()
    val acknowledgements: List<OperationAcknowledgement> get() = values.toList()
    override fun toString(): String = "SyncBatchResponse(redacted)"
    companion object {
        fun parse(raw: ByteArray, expected: List<EnterpriseOperation>, limits: SyncLimits = SyncLimits.DEFAULT): SyncBatchResponse {
            if (expected.size > limits.maxOperations || expected.map { it.operationId }.toSet().size != expected.size) throw invalid()
            val root = objectValue(CanonicalJson.parse(raw, limits.maxBatchBytes, limits.maxTransportDepth))
            exactKeys(root, setOf("schema_version", "acknowledgements")); version(root)
            val values = array(root, "acknowledgements")
            if (values.size != expected.size) throw invalid(SyncFailure.BINDING)
            val acknowledgements = values.mapIndexed { index, value ->
                OperationAcknowledgement.parse(CanonicalJson.encode(value, limits.maxOperationDepth), expected[index], limits)
            }
            return SyncBatchResponse(acknowledgements)
        }
    }
}
