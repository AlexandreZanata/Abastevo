package com.anpfuel.domain.enterprise.sync

/** Raw candidate, not an admitted operation. OFFL-002 must preflight authority before effects. */
class SyncOperationCandidate internal constructor(raw: ByteArray, private val limits: SyncLimits) {
    private val encoded = raw.copyOf()
    val rawBytes: ByteArray get() = encoded.copyOf()
    fun parse(binding: SyncBinding): EnterpriseOperation = EnterpriseOperation.parse(encoded, binding, limits)
    override fun toString(): String = "SyncOperationCandidate(redacted)"
}

/** Replaceable delivery proof stays outside the hashed intent and confers no authority here. */
class SyncBatch private constructor(val familyId: String, val accessToken: String, operations: List<SyncOperationCandidate>) {
    private val candidates = operations.toList()
    val operations: List<SyncOperationCandidate> get() = candidates.toList()
    override fun toString(): String = "SyncBatch(redacted)"
    companion object {
        fun parse(raw: ByteArray, limits: SyncLimits = SyncLimits.DEFAULT): SyncBatch {
            val node = CanonicalJson.parseSpanned(raw, limits.maxBatchBytes, limits.maxTransportDepth)
            val root = objectValue(node.value)
            exactKeys(root, setOf("schema_version", "family_id", "access_token", "operations")); version(root)
            val family = uuid(text(root, "family_id"))
            val token = text(root, "access_token")
            if (token.length !in 1..512 || token.any { it !in '!'..'~' }) throw invalid()
            val values = array(root, "operations")
            if (values.size > limits.maxOperations) throw invalid(SyncFailure.LIMIT)
            val operationArray = node.children.first { it.value === root["operations"] }
            val source = String(raw, Charsets.UTF_8) // Strict decoding has already succeeded above.
            val identities = mutableSetOf<String>()
            val candidates = operationArray.children.map { child ->
                val id = try { uuid(text(objectValue(child.value), "operation_id")) } catch (_: SyncProtocolException) { null }
                if (id != null && !identities.add(id)) throw invalid()
                SyncOperationCandidate(source.substring(child.start, child.end).toByteArray(Charsets.UTF_8), limits)
            }
            return SyncBatch(family, token, candidates)
        }
    }
}
