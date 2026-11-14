package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterpriseOfflineLease
import com.anpfuel.domain.enterprise.EnterprisePartition

data class EnterpriseLocalIdentity(val account: String, val origin: String, val family: String) {
    init {
        EnterprisePartition.requireId(account)
        EnterprisePartition.requireId(family)
        require(EnterprisePartition.canonicalOrigin(origin) == origin)
    }
    fun owns(partition: EnterprisePartition): Boolean = account == partition.ownerAccount && origin == partition.origin
    override fun toString(): String = "EnterpriseLocalIdentity(redacted)"
}

/** A metadata selector remains separate from the exact source-owned enrollment. */
data class EnterpriseDeviceTarget(val partition: EnterprisePartition, val membershipId: String) {
    init { EnterprisePartition.requireId(membershipId) }
    override fun toString(): String = "EnterpriseDeviceTarget(redacted)"
}

data class EnterpriseEnrollmentBinding(val target: EnterpriseDeviceTarget, val enrollmentId: String, val revision: Long) {
    init { EnterprisePartition.requireId(enrollmentId); require(revision > 0) }
    override fun toString(): String = "EnterpriseEnrollmentBinding(redacted)"
}

/** Existing ciphertext supplies its retained enrollment; a generic denial never discards it. */
data class EnterpriseContactRequest(
    val binding: EnterpriseAccessBinding,
    val target: EnterpriseDeviceTarget,
    val existingEnrollment: EnterpriseEnrollmentBinding? = null,
) {
    init {
        require(target.partition.ownerAccount == binding.accountId && target.partition.origin == binding.origin)
        require(existingEnrollment == null || existingEnrollment.target == target)
    }
    override fun toString(): String = "EnterpriseContactRequest(redacted)"
}

data class EnterpriseContactGrant(
    val enrollment: EnterpriseEnrollmentBinding,
    val family: String,
    val serverMillis: Long,
    val validityMillis: Long = EnterpriseOfflineLease.DEFAULT_VALIDITY_MILLIS,
    val managedUnlockSeconds: Int? = null,
) {
    init {
        EnterprisePartition.requireId(family)
        require(serverMillis >= 0 && validityMillis > 0)
        require(managedUnlockSeconds == null || managedUnlockSeconds > 0)
    }
    val partition: EnterprisePartition get() = enrollment.target.partition
    override fun toString(): String = "EnterpriseContactGrant(redacted)"
}

sealed interface EnterpriseContactReply {
    data class Active(val grant: EnterpriseContactGrant) : EnterpriseContactReply {
        override fun toString(): String = "EnterpriseContactReply.Active(redacted)"
    }
    data class Erase(val enrollment: EnterpriseEnrollmentBinding, val family: String, val serverMillis: Long) : EnterpriseContactReply {
        init { EnterprisePartition.requireId(family); require(serverMillis >= 0) }
        override fun toString(): String = "EnterpriseContactReply.Erase(redacted)"
    }
    data class Lock(val enrollment: EnterpriseEnrollmentBinding, val family: String, val serverMillis: Long) : EnterpriseContactReply {
        init { EnterprisePartition.requireId(family); require(serverMillis >= 0) }
        override fun toString(): String = "EnterpriseContactReply.Lock(redacted)"
    }
}

/** The implementation uses retained personal account proof and an actual identity-v1 signature. */
fun interface EnterpriseContactAuthority {
    suspend fun confirm(request: EnterpriseContactRequest): EnterpriseContactReply
}

object UnavailableEnterpriseContact : EnterpriseContactAuthority {
    override suspend fun confirm(request: EnterpriseContactRequest): EnterpriseContactReply = throw EnterpriseLocalDenied()
}

/** A generic transport/proof refusal is presentation feedback, never an erasure instruction. */
class EnterpriseProtectionRefused(val error: EnterpriseAccessError) : IllegalStateException("Enterprise protected contact refused")

interface EnterpriseContextProtection {
    suspend fun confirm(
        target: EnterpriseDeviceTarget,
        binding: EnterpriseAccessBinding,
        owner: EnterpriseFenceOwner,
        acknowledgeErasedWork: Boolean = false,
    ): EnterpriseFenceHandoff
}

object UnavailableEnterpriseContextProtection : EnterpriseContextProtection {
    override suspend fun confirm(target: EnterpriseDeviceTarget, binding: EnterpriseAccessBinding, owner: EnterpriseFenceOwner,
        acknowledgeErasedWork: Boolean): EnterpriseFenceHandoff = throw EnterprisePersistenceUnavailable()
}
