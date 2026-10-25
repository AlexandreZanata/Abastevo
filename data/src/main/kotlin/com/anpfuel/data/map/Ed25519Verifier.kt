package com.anpfuel.data.map

import java.security.PublicKey

/**
 * Narrow Ed25519 verification port. The platform provider is preferred; the
 * owned Monocypher bridge is the API-26 fallback. No digest-only acceptance
 * is permitted; unknown sizes fail closed.
 */
fun interface Ed25519Verifier {
    @Throws(MapSignatureException::class)
    fun verify(publicKey: PublicKey, message: ByteArray, signature: ByteArray): Boolean
}
