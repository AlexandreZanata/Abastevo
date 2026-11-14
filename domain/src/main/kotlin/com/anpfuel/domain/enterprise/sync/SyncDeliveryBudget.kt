package com.anpfuel.domain.enterprise.sync

/** Exact encoded proof size only; no credential or family is retained here. */
class SyncDeliveryBudget private constructor(private val encodedTokenBytes: Int) {
    fun batchBytes(operations: List<EnterpriseOperation>, limits: SyncLimits = SyncLimits.DEFAULT): Int {
        // Graphic ASCII consumes one or two canonical JSON bytes. Construct an
        // unrelated token with exactly that encoded size, never the real proof.
        val symbols = encodedTokenBytes - 2
        val length = minOf(symbols, 512)
        val escaped = symbols - length
        val placeholder = "\\".repeat(escaped) + "x".repeat(length - escaped)
        return SyncBatch.encode(FAMILY, placeholder, operations, limits).size
    }
    override fun toString(): String = "SyncDeliveryBudget(redacted)"
    companion object {
        private const val FAMILY = "00000000-0000-4000-8000-000000000001"
        fun fromProof(familyId: String, accessToken: String): SyncDeliveryBudget {
            uuid(familyId)
            if (accessToken.length !in 1..512 || accessToken.any { it !in '!'..'~' }) throw invalid()
            return SyncDeliveryBudget(CanonicalJson.encode(SyncJson.Text(accessToken)).size)
        }
        /** Bounded compatibility for synthetic/legacy adapters, never an exact production proof. */
        fun maximum(): SyncDeliveryBudget = SyncDeliveryBudget(1026)
    }
}
