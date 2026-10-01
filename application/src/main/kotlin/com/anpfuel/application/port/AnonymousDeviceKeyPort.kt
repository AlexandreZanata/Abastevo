package com.anpfuel.application.port

/**
 * P10-T03 device-key custody and signing port.
 *
 * The data layer owns the Android Keystore key (non-exportable P-256);
 * the application flow only orchestrates challenges, base lines and
 * rotation intents. Null means the keystore is unavailable and the
 * operation fails closed. Raw signatures are 64-byte `R || S`.
 */
interface AnonymousDeviceKeyPort {
    /** Device fingerprint `fp:<hex>` for the current key, or null. */
    fun fingerprint(): String?

    /** Public JWK coordinates (base64url, 32 bytes each), or null. */
    fun publicJwk(): Pair<String, String>?

    /**
     * Signs the 64-byte SHA-512 digest of the signature base.
     * Returns the raw 64-byte signature, or null on failure.
     */
    fun signDigest(digestSha512: ByteArray): ByteArray?

    /** True when a device key exists (no key material leaves this port). */
    fun hasKey(): Boolean

    /** Forgets the local key reference (reinstall/lost-key path). */
    fun forgetKey()
}
