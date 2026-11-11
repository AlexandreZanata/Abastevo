package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.domain.enterprise.EnterprisePartition

/** Marker and key deletion are independent attempts. A failure is never reported as erased. */
class EnterpriseVaultErasure(
    private val files: EnterpriseVaultFiles,
    private val keys: EnterpriseVaultKeys,
    private val fence: EnterpriseSessionFence,
) {
    private val denied = mutableSetOf<String>()
    private val failed = mutableSetOf<String>()
    val cleanupRequired: Boolean get() = fence.access { failed.isNotEmpty() }

    internal fun deniedLocked(token: String): Boolean {
        val account = token.substringBefore('-')
        return token in denied || account in denied || files.denied(account) || files.denied(token)
    }

    internal fun eraseLocked(token: String): Boolean {
        fence.lock()
        denied += token
        var success = true
        try { files.deny(token) } catch (_: Exception) { success = false }
        try { keys.delete(EnterpriseVaultCodec.alias(token)) } catch (_: Exception) { success = false }
        try { files.erase(token) } catch (_: Exception) { success = false }
        if (success) failed -= token else failed += token
        return success
    }

    private fun eraseAccountLocked(tag: String): Boolean {
        denied += tag
        var success = true
        try { files.deny(tag) } catch (_: Exception) { success = false }
        // Include aliases left by a failed first snapshot write.
        try {
            keys.aliases(EnterpriseVaultCodec.alias(tag + "-")).forEach { alias ->
                try { keys.delete(alias) } catch (_: Exception) { success = false }
            }
        } catch (_: Exception) { success = false }
        try {
            files.tokens(tag).forEach { if (!eraseLocked(it)) success = false }
        } catch (_: Exception) { success = false }
        if (success) failed -= tag else failed += tag
        return success
    }

    /** Already under the account file lock, after fresh authority plus explicit loss acknowledgement. */
    internal fun prepareFreshLocked(token: String): Boolean {
        val tag = token.substringBefore('-')
        return if (tag in denied || files.denied(tag)) eraseAccountLocked(tag) else eraseLocked(token)
    }

    /** Clear denial last, after a new empty authenticated snapshot and key are durable. */
    internal fun permitFreshLocked(token: String) {
        val tag = token.substringBefore('-')
        files.allowFresh(token)
        if (tag in denied || files.denied(tag)) files.allowFresh(tag)
        denied -= token
        denied -= tag
        failed -= token
        failed -= tag
    }

    fun partitionRevoked(p: EnterprisePartition): Boolean = fence.access {
        val tag = EnterpriseVaultCodec.accountTag(p.ownerAccount, p.origin)
        val token = EnterpriseVaultCodec.token(p)
        fence.lock()
        denied += token
        try { files.locked(tag) { eraseLocked(token) } }
        catch (_: Exception) {
            // A failed filesystem lock must not prevent the independent key-disposal attempt.
            try { keys.delete(EnterpriseVaultCodec.alias(token)) } catch (_: Exception) { /* report below */ }
            failed += token
            false
        }
    }

    fun accountRevoked(owner: String, origin: String): Boolean = fence.access {
        fence.lock()
        val tag = try { EnterpriseVaultCodec.accountTag(owner, origin) }
        catch (_: Exception) { failed += "unavailable-scope"; return@access false }
        denied += tag
        try { files.locked(tag) { eraseAccountLocked(tag) } }
        catch (_: Exception) {
            // The marker path may be unavailable; still attempt to discard every scoped key.
            try {
                keys.aliases(EnterpriseVaultCodec.alias(tag + "-")).forEach { alias ->
                    try { keys.delete(alias) } catch (_: Exception) { /* keep trying other scoped keys */ }
                }
            } catch (_: Exception) { /* report below */ }
            failed += tag
            false
        }
    }
}
