package com.anpfuel.data.local.media

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.security.KeyStore
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

/**
 * Android key custody for sealed transient photos (P15-T02B).
 *
 * The AES/GCM photo key lives in AndroidKeyStore under [alias] and
 * never leaves it; sealed envelopes rest in private app-cache files.
 * Custody is the only Android boundary, kept behind [PhotoKeyProvider]
 * so JVM tests inject in-memory keys while production uses
 * [AndroidPhotoKeystoreKeys]. A rotation (or foreign blob) simply
 * fails to open: reads return null and the sweep purges them.
 */
fun interface PhotoKeyProvider {
    fun getOrCreateKey(alias: String): SecretKey?
}

/**
 * AndroidKeyStore AES/GCM key source (API 26+, no user authentication:
 * unlock gating would strand launch/resume sweeps on a locked device;
 * the key material itself never leaves hardware-backed storage when
 * present).
 *
 * The ~30 lines of KeyStore boilerplate below cannot run on JVM unit
 * tests (no Android runtime): they are statically reviewed and
 * lint-guarded, while [PhotoBlobSeal] and [AndroidPhotoCache] carry
 * the behavioral suites.
 */
class AndroidPhotoKeystoreKeys : PhotoKeyProvider {

    override fun getOrCreateKey(alias: String): SecretKey? {
        return try {
            val store = KeyStore.getInstance(ANDROID_KEYSTORE)
            store.load(null)
            val existing = store.getKey(alias, null) as? SecretKey
            if (existing != null) return existing
            val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
            generator.init(
                KeyGenParameterSpec.Builder(
                    alias,
                    KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                )
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setUserAuthenticationRequired(false)
                    .setRandomizedEncryptionRequired(true)
                    .build(),
            )
            generator.generateKey()
        } catch (_: Exception) {
            null
        }
    }

    companion object {
        const val ANDROID_KEYSTORE: String = "AndroidKeyStore"
    }
}
