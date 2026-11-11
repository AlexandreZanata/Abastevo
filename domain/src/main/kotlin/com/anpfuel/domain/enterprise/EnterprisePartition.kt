package com.anpfuel.domain.enterprise

import java.net.URI

/** Local ownership only. Membership, credentials and server authority are separate. */
class EnterprisePartition private constructor(
    val ownerAccount: String,
    val origin: String,
    val organizationId: String,
) {
    override fun equals(other: Any?): Boolean = other is EnterprisePartition &&
        ownerAccount == other.ownerAccount && origin == other.origin && organizationId == other.organizationId

    override fun hashCode(): Int = 31 * (31 * ownerAccount.hashCode() + origin.hashCode()) + organizationId.hashCode()
    override fun toString(): String = "EnterprisePartition(redacted)"

    companion object {
        private val uuid = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")

        fun requireId(value: String): String {
            require(uuid.matches(value) && value != "00000000-0000-0000-0000-000000000000") { "Invalid enterprise reference" }
            return value
        }

        fun canonicalOrigin(value: String): String {
            val uri = try { URI(value) } catch (_: Exception) { throw IllegalArgumentException("Invalid enterprise origin") }
            require(uri.scheme?.lowercase() == "https" && !uri.host.isNullOrBlank() &&
                uri.rawUserInfo == null && uri.rawQuery == null && uri.rawFragment == null &&
                (uri.rawPath.isNullOrEmpty() || uri.rawPath == "/") &&
                (uri.port == -1 || uri.port in 1..65535)) { "Invalid enterprise origin" }
            val port = if (uri.port == -1 || uri.port == 443) "" else ":${uri.port}"
            return "https://${uri.host.lowercase()}$port"
        }

        fun of(ownerAccount: String, origin: String, organizationId: String): EnterprisePartition =
            EnterprisePartition(requireId(ownerAccount), canonicalOrigin(origin), requireId(organizationId))
    }
}

/** Opaque storage envelopes. Encryption/decoding belongs to the protected-store owner. */
class EnterpriseLocalRecord(
    val partition: EnterprisePartition,
    id: String,
    envelope: ByteArray,
) {
    val id: String = EnterprisePartition.requireId(id)
    private val bytes = envelope.copyOf()
    val envelope: ByteArray get() = bytes.copyOf()
    override fun toString(): String = "EnterpriseLocalRecord(redacted)"
}

class EnterprisePendingOperation(
    val partition: EnterprisePartition,
    id: String,
    targetId: String,
    envelope: ByteArray,
) {
    val id: String = EnterprisePartition.requireId(id)
    val targetId: String = EnterprisePartition.requireId(targetId)
    private val bytes = envelope.copyOf()
    val envelope: ByteArray get() = bytes.copyOf()
    override fun toString(): String = "EnterprisePendingOperation(redacted)"
}
