package com.anpfuel.domain.account

import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64

/** The credential provider receives [claim], while the account API receives [raw]. */
class GoogleNonce private constructor(val raw: String, val claim: String) {
    companion object {
        private val random = SecureRandom()

        fun generate(): GoogleNonce {
            val bytes = ByteArray(32)
            random.nextBytes(bytes)
            val raw = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
            return GoogleNonce(raw, claimFor(raw))
        }

        fun claimFor(raw: String): String {
            val digest = MessageDigest.getInstance("SHA-256").digest(raw.toByteArray(Charsets.UTF_8))
            val hex = "0123456789abcdef"
            return buildString(digest.size * 2) {
                for (byte in digest) {
                    val unsigned = byte.toInt() and 0xff
                    append(hex[unsigned ushr 4])
                    append(hex[unsigned and 0x0f])
                }
            }
        }
    }
}
