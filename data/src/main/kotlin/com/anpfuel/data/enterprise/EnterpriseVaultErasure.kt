package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.domain.enterprise.EnterprisePartition

/** Independent file and trusted-provider tombstones survive a lost process and restorable files. */
class EnterpriseVaultErasure(
    private val files: EnterpriseVaultFiles,
    private val keys: EnterpriseVaultKeys,
    private val fence: EnterpriseSessionFence,
) {
    private val denied = mutableSetOf<String>()
    private val failed = mutableSetOf<String>()
    val cleanupRequired: Boolean get() = fence.access {
        failed.isNotEmpty() || try { durableDenied().any(::hasContent) } catch (_: Exception) { true }
    }

    internal fun knownIntents(): List<String> = (durableDenied() + denied + failed).distinct().sorted()

    internal fun durableDenied(): List<String> = (files.blockedTokens() + keys.aliases(EnterpriseVaultGenerationNames.PREFIX)
        .filter { it.startsWith(EnterpriseVaultGenerationNames.PREFIX + "denied_") }
        .map { it.removePrefix(EnterpriseVaultGenerationNames.PREFIX + "denied_") }).distinct().sorted()

    internal fun deniedLocked(token: String): Boolean {
        val account = token.substringBefore('-')
        return token in denied || account in denied || files.denied(account) || files.denied(token) ||
            keys.existing(EnterpriseVaultGenerationNames.denied(token)) != null || keys.existing(EnterpriseVaultGenerationNames.denied(account)) != null
    }

    internal fun requireContactLocked(token: String) {
        try {
            val alias = EnterpriseVaultGenerationNames.retained(token)
            if (keys.existing(alias) == null) keys.create(alias, null)
        } finally { fence.lock() }
    }

    private fun intent(token: String): Boolean {
        denied += token
        var success = true
        try { files.deny(token) } catch (_: Exception) { success = false }
        try {
            val alias = EnterpriseVaultGenerationNames.denied(token)
            if (keys.existing(alias) == null) keys.create(alias, null)
        } catch (_: Exception) { success = false }
        return success
    }

    private fun contentAliases(token: String): List<String> = keys.aliases(EnterpriseVaultGenerationNames.prefix(token)) +
        listOf(EnterpriseVaultCodec.alias(token), EnterpriseVaultGenerationNames.retained(token))

    private fun hasContent(token: String): Boolean {
        if (token.length == 64) return tokens(token).any(::hasContent)
        return files.read(token) != null || files.generations(token).isNotEmpty() || contentAliases(token).any { keys.existing(it) != null }
    }

    internal fun eraseLocked(token: String, invalidate: Boolean = true): Boolean {
        if (invalidate) fence.lock()
        val intentEstablished = intent(token)
        var success = intentEstablished
        val aliases = mutableSetOf(EnterpriseVaultCodec.alias(token), EnterpriseVaultGenerationNames.retained(token))
        try { aliases += keys.aliases(EnterpriseVaultGenerationNames.prefix(token)) } catch (_: Exception) { success = false }
        val generations = try { files.generations(token) } catch (_: Exception) { success = false; emptyList() }
        generations.forEach { generation ->
            aliases += EnterpriseVaultGenerationNames.data(token, generation)
            aliases += EnterpriseVaultGenerationNames.maintenance(token, generation)
            aliases += EnterpriseVaultGenerationNames.head(token, generation)
        }
        // Heads are rollback floors, not decryption material. Never lower the maximum
        // while any old material could survive, even if both denial channels failed.
        aliases.filterNot { it.endsWith("_h") }.forEach { alias ->
            try { keys.delete(alias) } catch (_: Exception) { success = false }
        }
        generations.forEach { generation ->
            try { files.eraseGeneration(token, generation) } catch (_: Exception) { success = false }
        }
        try { files.erase(token) } catch (_: Exception) { success = false }
        val materialGone = try {
            files.read(token) == null && files.generations(token).isEmpty() &&
                contentAliases(token).filterNot { it.endsWith("_h") }.none { keys.existing(it) != null }
        } catch (_: Exception) { success = false; false }
        // Even fully disposed work must remain distinguishable from a never-used
        // partition until a durable denial can require explicit loss acknowledgement.
        if (materialGone && intentEstablished) aliases.filter { it.endsWith("_h") }.forEach { alias ->
            try { keys.delete(alias) } catch (_: Exception) { success = false }
        } else success = false
        try { if (hasContent(token)) success = false } catch (_: Exception) { success = false }
        if (success) failed -= token else failed += token
        return success
    }

    private fun tokens(tag: String): List<String> = (files.allTokens().filter { it.startsWith(tag + "-") } +
        keys.aliases(EnterpriseVaultGenerationNames.PREFIX).mapNotNull(EnterpriseVaultGenerationNames::token).filter { it.startsWith(tag + "-") } +
        keys.aliases(EnterpriseVaultCodec.alias(tag + "-")).map { it.removePrefix("abastevo_enterprise_v1_") }).distinct()

    private fun eraseAccountLocked(tag: String, invalidate: Boolean = true): Boolean {
        var success = intent(tag)
        try { tokens(tag).forEach { if (!eraseLocked(it, invalidate)) success = false } } catch (_: Exception) { success = false }
        // A failed token catalog must not turn a rollback floor into an ordinary disposable key.
        try {
            keys.aliases(EnterpriseVaultCodec.alias(tag + "-")).forEach { alias ->
                try { keys.delete(alias) } catch (_: Exception) { success = false }
            }
        } catch (_: Exception) { success = false }
        if (success) failed -= tag else failed += tag
        return success
    }

    internal fun prepareFreshLocked(token: String): Boolean {
        val tag = token.substringBefore('-')
        return if (tag in denied || files.denied(tag) || keys.existing(EnterpriseVaultGenerationNames.denied(tag)) != null) eraseAccountLocked(tag, false) else eraseLocked(token, false)
    }

    /** Invoked only after fresh proof, explicit loss acknowledgement and a durable empty generation. */
    internal fun permitFreshLocked(token: String) {
        val tag = token.substringBefore('-')
        files.allowFresh(token); keys.delete(EnterpriseVaultGenerationNames.denied(token))
        files.allowFresh(tag); keys.delete(EnterpriseVaultGenerationNames.denied(tag))
        denied -= token; denied -= tag; failed -= token; failed -= tag
    }

    fun partitionRevoked(p: EnterprisePartition): Boolean = fence.access {
        val tag = EnterpriseVaultCodec.accountTag(p.ownerAccount, p.origin)
        val token = EnterpriseVaultCodec.token(p)
        fence.lock(); denied += token
        try { files.locked(tag) { eraseLocked(token) } } catch (_: Exception) {
            // No file lock: persist trusted denial and try every scoped key independently.
            intent(token)
            try { contentAliases(token).filterNot { it.endsWith("_h") }.forEach { alias -> try { keys.delete(alias) } catch (_: Exception) { } } } catch (_: Exception) { }
            failed += token; false
        }
    }

    fun accountRevoked(owner: String, origin: String): Boolean = fence.access {
        fence.lock()
        val tag = try { EnterpriseVaultCodec.accountTag(owner, origin) } catch (_: Exception) { return@access true }
        val held = tag in denied || try {
            files.present(tag) || tokens(tag).isNotEmpty() || keys.existing(EnterpriseVaultGenerationNames.denied(tag)) != null
        } catch (_: Exception) { true }
        if (!held) { failed.removeAll { it == tag || it.startsWith("$tag-") }; return@access true }
        denied += tag
        try { files.locked(tag) { eraseAccountLocked(tag) } } catch (_: Exception) {
            intent(tag)
            listOf(EnterpriseVaultGenerationNames.PREFIX + tag + "-", EnterpriseVaultCodec.alias(tag + "-")).forEach { prefix ->
                try { keys.aliases(prefix).filterNot { it.endsWith("_h") }.forEach { alias -> try { keys.delete(alias) } catch (_: Exception) { } } } catch (_: Exception) { }
            }
            failed += tag; false
        }
    }

    internal fun settleAccountLocked(tag: String): Boolean {
        require(tag.length == 64)
        val success = intent(tag) && !hasContent(tag)
        if (success) failed -= tag else failed += tag
        return success
    }

    internal fun retryLocked(token: String): Boolean = if (token.length == 64) eraseAccountLocked(token) else eraseLocked(token)
}
