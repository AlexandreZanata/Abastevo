package com.anpfuel.domain.repository

/** Durable intent identity is independent from transport nonce and wire revision. */
data class TrainingConsentCommand(
    val id: String,
    val grant: Boolean,
    val revision: Long?,
    val termsVersion: String = "",
    val termsDigest: String = "",
) {
    init {
        require(id.matches(Regex("[A-Za-z0-9-]{1,64}")))
        require(revision == null || revision > 0)
        require(termsVersion.length <= 128 && termsDigest.length <= 128)
        require(!grant || (termsVersion.isNotBlank() && termsDigest.isNotBlank()))
    }
}

data class TrainingConsentLocalState(
    val revisionFloor: Long = 0,
    val granted: Boolean = false,
    val pending: TrainingConsentCommand? = null,
    val retryAtMillis: Long = 0,
    val attempts: Int = 0,
) {
    init {
        require(revisionFloor >= 0 && retryAtMillis >= 0 && attempts in 0..16)
        require(pending?.revision == null || pending.revision <= revisionFloor)
        require(pending == null || !granted)
    }

    fun revoke(id: String): TrainingConsentLocalState = copy(
        granted = false,
        pending = pending?.takeUnless { it.grant } ?: TrainingConsentCommand(id, false, null),
        retryAtMillis = if (pending?.grant == false) retryAtMillis else 0,
        attempts = if (pending?.grant == false) attempts else 0,
    )

    fun grant(id: String, version: String, digest: String, serverRevision: Long): TrainingConsentLocalState {
        if (pending != null) {
            if (pending.grant && pending.termsVersion == version && pending.termsDigest == digest) return this
            throw TrainingConsentException(TrainingConsentRejectKind.CONFLICT, "resolve the pending privacy decision first")
        }
        if (serverRevision < revisionFloor) throw TrainingConsentException(TrainingConsentRejectKind.STALE_REVISION,"privacy history requires reconciliation")
        val revision = nextRevision(serverRevision)
        return copy(revisionFloor = revision, granted = false,
            pending = TrainingConsentCommand(id, true, revision, version, digest))
    }

    /** Ambiguous delivery keeps the assigned revision. Rebinding is only for a confirmed 409 on OFF. */
    fun bindRevocation(id: String, serverRevision: Long, confirmedStale: Boolean = false): TrainingConsentLocalState {
        val command = pending ?: return this
        if (command.id != id || command.grant || (command.revision != null && !confirmedStale)) return this
        val revision = nextRevision(serverRevision)
        return copy(revisionFloor = revision, pending = command.copy(revision = revision))
    }

    fun acknowledge(command: TrainingConsentCommand, receipt: TrainingConsentReceipt): TrainingConsentLocalState {
        if (pending != command || command.revision != receipt.revision) return this
        val expected = if (command.grant) "GRANTED" else "REVOKED"
        if (receipt.state != expected || (command.grant && receipt.termsVersion != command.termsVersion)) {
            throw TrainingConsentException(TrainingConsentRejectKind.TRANSPORT, "invalid privacy receipt")
        }
        return copy(granted = command.grant, pending = null, retryAtMillis = 0, attempts = 0)
    }

    fun failed(command: TrainingConsentCommand, nowMillis: Long, retryAfterSeconds: Long?): TrainingConsentLocalState {
        if (pending != command) return this
        require(nowMillis >= 0 && (retryAfterSeconds == null || retryAfterSeconds >= 0))
        val seconds = maxOf(30L * (1L shl minOf(attempts, 6)), retryAfterSeconds ?: 0)
        val at = if (seconds > (Long.MAX_VALUE - nowMillis) / 1000) Long.MAX_VALUE else nowMillis + seconds * 1000
        return copy(retryAtMillis = at, attempts = minOf(attempts + 1, 16))
    }

    private fun nextRevision(serverRevision: Long): Long {
        require(serverRevision >= 0)
        val highest = maxOf(revisionFloor, serverRevision)
        if (highest == Long.MAX_VALUE) throw TrainingConsentException(TrainingConsentRejectKind.INVALID, "privacy revision exhausted")
        return highest + 1
    }
}
