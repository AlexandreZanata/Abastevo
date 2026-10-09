package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException

/** Immutable local scope; never a substitute for server proof or proximity. */
data class ContributionScope(val ownerScope: String, val origin: String) {
    init {
        if (ownerScope.isBlank() || ownerScope.length > 256 || ownerScope.any { it.code < 32 } ||
            !Regex("https://[A-Za-z0-9.-]+(:[0-9]{1,5})?").matches(origin)) {
            throw DomainException("contribution scope is invalid")
        }
    }
}

data class PhotoContributionContext(
    val captureId: String,
    val expiresAtMillis: Long,
    val ownerScope: String,
    val origin: String,
) {
    init {
        if (!ContributionDraft.isUuid(captureId) || expiresAtMillis <= 0) {
            throw DomainException("photo capture context is invalid")
        }
        ContributionScope(ownerScope, origin)
    }
    val scope: ContributionScope get() = ContributionScope(ownerScope, origin)
}
