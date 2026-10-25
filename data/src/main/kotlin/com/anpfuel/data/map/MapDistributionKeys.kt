package com.anpfuel.data.map

import com.anpfuel.domain.map.MapContractException

/**
 * Pinned map-distribution trust configuration. Keys are raw 32-byte Ed25519
 * public keys (base64url, no padding) so verification also works where the
 * platform has no Ed25519 provider (API 26): the owned Monocypher bridge is
 * the fallback. Format is `keyid:base64url,...` with
 * `keyid = [A-Za-z0-9._-]{1,64}`; duplicates and malformed entries are
 * refused. An empty input means "not configured", never a default key.
 */
object MapDistributionKeys {
    private val KEY_ID = Regex("^[A-Za-z0-9._-]{1,64}$")

    fun parse(raw: String): Map<String, ByteArray> {
        if (raw.isBlank()) return emptyMap()
        val out = LinkedHashMap<String, ByteArray>()
        for (entry in raw.split(",")) {
            val item = entry.trim()
            val parts = item.split(":")
            if (parts.size != 2) throw MapContractException("invalid-map-keys")
            val id = parts[0].trim()
            val key = try {
                java.util.Base64.getUrlDecoder().decode(pad(parts[1].trim()))
            } catch (_: Exception) {
                throw MapContractException("invalid-map-keys")
            }
            if (!KEY_ID.matches(id) || key.size != 32 || id in out) {
                throw MapContractException("invalid-map-keys")
            }
            out[id] = key
        }
        return out
    }

    private fun pad(value: String): String {
        if (value.isEmpty()) throw MapContractException("invalid-map-keys")
        return value + "=".repeat((4 - value.length % 4) % 4)
    }
}
