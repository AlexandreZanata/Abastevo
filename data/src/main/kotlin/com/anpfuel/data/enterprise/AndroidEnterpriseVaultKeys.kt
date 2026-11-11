package com.anpfuel.data.enterprise

import android.content.Context
import android.os.Build
import android.os.SystemClock
import android.provider.Settings
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyInfo
import android.security.keystore.KeyProperties
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import java.security.KeyStore
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.SecretKeyFactory

/** Separate aliases from account sessions; never export/recreate a decryption key on read. */
class AndroidEnterpriseVaultKeys : EnterpriseVaultKeys {
    private fun store(): KeyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
    private fun checkAlias(alias: String) = require(aliasPattern.matches(alias))

    override fun existing(alias: String): EnterpriseVaultKey? {
        checkAlias(alias)
        val key = store().getKey(alias, null) as? SecretKey ?: return null
        val info = SecretKeyFactory.getInstance(key.algorithm, "AndroidKeyStore").getKeySpec(key, KeyInfo::class.java) as KeyInfo
        check(info.keySize == 256 && info.blockModes.contentEquals(arrayOf(KeyProperties.BLOCK_MODE_GCM)) &&
            info.encryptionPaddings.contentEquals(arrayOf(KeyProperties.ENCRYPTION_PADDING_NONE)))
        val seconds = if (info.isUserAuthenticationRequired) info.userAuthenticationValidityDurationSeconds else null
        check(seconds == null || seconds > 0)
        return EnterpriseVaultKey(key, seconds)
    }

    override fun create(alias: String, managedUnlockSeconds: Int?): EnterpriseVaultKey {
        checkAlias(alias)
        require(managedUnlockSeconds == null || managedUnlockSeconds > 0)
        check(!store().containsAlias(alias))
        val builder = KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setKeySize(256)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setRandomizedEncryptionRequired(true)
            .setUserAuthenticationRequired(managedUnlockSeconds != null)
        if (managedUnlockSeconds != null) {
            if (Build.VERSION.SDK_INT >= 30) {
                builder.setUserAuthenticationParameters(
                    managedUnlockSeconds, KeyProperties.AUTH_BIOMETRIC_STRONG or KeyProperties.AUTH_DEVICE_CREDENTIAL,
                )
            } else {
                @Suppress("DEPRECATION")
                builder.setUserAuthenticationValidityDurationSeconds(managedUnlockSeconds)
            }
        }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        generator.init(builder.build())
        generator.generateKey()
        return existing(alias) ?: error("Enterprise key unavailable")
    }

    override fun delete(alias: String) {
        checkAlias(alias)
        val store = store()
        store.deleteEntry(alias)
        check(!store.containsAlias(alias))
    }

    override fun aliases(prefix: String): List<String> {
        require(Regex("abastevo_enterprise_v1_[0-9a-f]{64}-").matches(prefix))
        return store().aliases().toList().filter { it.startsWith(prefix) && aliasPattern.matches(it) }
    }

    companion object {
        private val aliasPattern = Regex("abastevo_enterprise_v1_[0-9a-f]{64}-[0-9a-f]{64}")
    }
}

class AndroidEnterpriseDeviceClock(private val context: Context) {
    fun read(): EnterpriseDeviceClock {
        val boot = Settings.Global.getInt(context.contentResolver, Settings.Global.BOOT_COUNT, -1)
        check(boot >= 0) { "Enterprise clock unavailable" }
        return EnterpriseDeviceClock("boot-" + boot, SystemClock.elapsedRealtime(), System.currentTimeMillis())
    }
}
