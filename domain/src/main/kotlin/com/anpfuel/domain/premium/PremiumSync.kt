package com.anpfuel.domain.premium

/**
 * PRM09-T01 owner scope, opaque cursors, durable commands and conflicts.
 *
 * Transport mirrors `docs/backend/ANDROID_INTEGRATION_GUIDE.md`: POST JSON
 * with `family_id` plus `access_token`; writes add the exact body signed by
 * the account key and the bound `device_id`. Cursors are opaque HMAC strings
 * returned verbatim until empty; all responses are `no-store` without ETags.
 * Paid writes need an active capability (`403` on lapse); reads, deletes and
 * erasures stay free.
 */

/** Account/origin partition: every Room row and cursor is bound to both. */
data class PremiumOwnerScope(
    val accountId: String,
    val origin: String,
) {
    init {
        require(accountId.isNotBlank()) { "account required" }
        require(origin.isNotBlank()) { "origin required" }
    }

    fun cursorKey(dataset: String): String = "$accountId|$origin|$dataset"
}

/**
 * Opaque sync cursor. The client never parses it: it returns `next_cursor`
 * verbatim until empty. Corrupted values (overlong, control chars) force a
 * resnapshot instead of silently skipping history.
 */
@JvmInline
value class PremiumCursor(val raw: String) {
    init {
        if (raw.length > PremiumBounds.MAX_CURSOR_CHARS) {
            throw PremiumValidationException("corrupted-cursor")
        }
        for (c in raw) {
            if (c == '\u0000' || c == '\n' || c == '\r') {
                throw PremiumValidationException("corrupted-cursor")
            }
        }
    }

    val isEmpty: Boolean get() = raw.isEmpty()

    companion object {
        fun start(): PremiumCursor = PremiumCursor("")

        fun parse(raw: String?): PremiumCursor {
            if (raw == null) throw PremiumValidationException("corrupted-cursor")
            return PremiumCursor(raw)
        }
    }
}

/** Durable command intent stored before any upload attempt. */
data class PremiumCommand(
    val operationId: String,
    val scope: PremiumOwnerScope,
    val dataset: String,
    val recordId: String,
    val expectedRevision: Long,
    val delete: Boolean = false,
    val void: Boolean = false,
    val payloadJson: String = "",
) {
    init {
        if (operationId.isBlank()) throw PremiumValidationException("invalid-operation")
        if (dataset.isBlank()) throw PremiumValidationException("unknown-dataset")
        if (recordId.isBlank()) throw PremiumValidationException("invalid-record")
        if (expectedRevision < 0L) throw PremiumValidationException("invalid-revision")
        if (payloadJson.length > 8 * 1024) throw PremiumValidationException("input-out-of-bounds")
    }
}

/**
 * Explicit conflict triple: the server keeps its current value, the local
 * proposed edit is preserved, and resolution is a fresh command at the
 * current revision with a new operation id. No silent last-arrival overwrite.
 */
data class PremiumConflict(
    val operationId: String,
    val dataset: String,
    val recordId: String,
    val expectedRevision: Long,
    val currentRevision: Long,
    val currentDeleted: Boolean,
    val erased: Boolean,
) {
    init {
        if (operationId.isBlank()) throw PremiumValidationException("invalid-operation")
        if (currentRevision < 0L || expectedRevision < 0L) {
            throw PremiumValidationException("invalid-revision")
        }
    }

    fun resolutionOperationId(): String = operationId + "#retry"

    fun requiresExplicitResolution(): Boolean = true
}

/** Capability lease: server status drives paid writes; free exits stay open. */
enum class PremiumCapability {
    ACTIVE,
    LAPSED,
    UNKNOWN,
    ;

    fun canWritePaid(): Boolean = this == ACTIVE
}
