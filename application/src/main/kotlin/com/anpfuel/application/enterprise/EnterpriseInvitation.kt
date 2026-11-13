package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterprisePartition
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

typealias EnterpriseInvitationGrant = com.anpfuel.domain.enterprise.EnterpriseInvitationGrant
typealias EnterpriseInvitationPreview = com.anpfuel.domain.enterprise.EnterpriseInvitationPreview
typealias EnterpriseInvitationAcknowledgement = com.anpfuel.domain.enterprise.EnterpriseInvitationAcknowledgement
typealias EnterpriseInvitationContract = com.anpfuel.domain.enterprise.EnterpriseInvitationContract

data class EnterpriseAccessBinding(val accountId: String, val origin: String, val familyId: String, val epoch: Long) {
    init {
        EnterprisePartition.requireId(accountId); EnterprisePartition.requireId(familyId)
        require(EnterprisePartition.canonicalOrigin(origin) == origin && epoch >= 0) { "Invalid enterprise access binding" }
    }
    override fun toString(): String = "EnterpriseAccessBinding(redacted)"
}

enum class EnterpriseAccessError {
    SIGN_IN_REQUIRED, FRESH_AUTH_REQUIRED, INVALID_CODE, UNAVAILABLE, CONFLICT,
    ALREADY_MEMBER, MANAGED_RESTRICTED, RATE_LIMITED, STALE, ACCESS_DENIED,
}

sealed interface EnterpriseAccessResult<out T> {
    data class Ok<T>(val value: T) : EnterpriseAccessResult<T> {
        override fun toString(): String = "EnterpriseAccessResult.Ok(redacted)"
    }
    data class Err(val error: EnterpriseAccessError) : EnterpriseAccessResult<Nothing>
}

/** open refreshes personal proof; all calls must withhold replies after identity/background changes. */
interface EnterpriseInvitationGateway {
    fun currentBinding(): EnterpriseAccessBinding?
    suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding>
    suspend fun preview(binding: EnterpriseAccessBinding, code: String): EnterpriseAccessResult<EnterpriseInvitationPreview>
    suspend fun accept(binding: EnterpriseAccessBinding, operationId: String, code: String, consentDigest: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement>
    suspend fun decline(binding: EnterpriseAccessBinding, operationId: String, code: String): EnterpriseAccessResult<EnterpriseInvitationAcknowledgement>
    fun invalidate()
}

enum class EnterpriseInvitationStage { ENTRY, PREVIEW, RESULT }
enum class EnterpriseInvitationDecision { ACCEPT, DECLINE }

data class EnterpriseInvitationState(
    val stage: EnterpriseInvitationStage = EnterpriseInvitationStage.ENTRY,
    val busy: Boolean = false,
    val signedIn: Boolean = false,
    val preview: EnterpriseInvitationPreview? = null,
    val acknowledgement: EnterpriseInvitationAcknowledgement? = null,
    val error: EnterpriseAccessError? = null,
    val retryDecision: EnterpriseInvitationDecision? = null,
) {
    override fun toString(): String = "EnterpriseInvitationState(stage=$stage,busy=$busy,redacted)"
}

/** Memory-only ceremony. A server receipt is displayed, never installed as local authority. */
class EnterpriseInvitationFlow(
    private val gateway: EnterpriseInvitationGateway,
    private val newOperationId: () -> String,
) {
    private val monitor = Any()
    private val mutable = MutableStateFlow(EnterpriseInvitationState(signedIn = current() != null))
    val state: StateFlow<EnterpriseInvitationState> = mutable.asStateFlow()
    private var generation = 0L
    private var binding: EnterpriseAccessBinding? = null
    private var code: String? = null
    private var decision: Pair<EnterpriseInvitationDecision, String>? = null

    private fun current(): EnterpriseAccessBinding? = try { gateway.currentBinding() } catch (_: Exception) { null }

    private fun clearLocked(error: EnterpriseAccessError? = null) {
        generation++
        binding = null; code = null; decision = null
        try { gateway.invalidate() } catch (_: Exception) { /* Presentation still loses all authority. */ }
        mutable.value = EnterpriseInvitationState(signedIn = current() != null, error = error)
    }

    fun restart() = synchronized(monitor) { clearLocked() }

    fun invalidateIfStale() = synchronized(monitor) {
        val now = current()
        if (binding != null && now != binding) clearLocked(EnterpriseAccessError.STALE)
        else if (mutable.value.signedIn != (now != null)) {
            if (now == null) clearLocked(EnterpriseAccessError.SIGN_IN_REQUIRED)
            else mutable.value = mutable.value.copy(signedIn = true)
        }
        // During open the gateway owns its source-proof fence; no private preview is displayed.
    }

    suspend fun preview(rawCode: String) {
        val normalized = EnterpriseInvitationContract.normalizeCode(rawCode)
        var openingBinding: EnterpriseAccessBinding? = null
        val token = synchronized(monitor) {
            if (mutable.value.busy || decision != null) return
            clearLocked()
            if (normalized.isBlank() || normalized.length > 160) {
                mutable.value = mutable.value.copy(error = EnterpriseAccessError.INVALID_CODE)
                return
            }
            mutable.value = mutable.value.copy(busy = true)
            openingBinding = current()
            generation
        }
        val opened = safely(token) { gateway.open() }
        val selected = synchronized(monitor) {
            if (token != generation) return
            when (opened) {
                is EnterpriseAccessResult.Err -> { failLocked(opened.error); return }
                is EnterpriseAccessResult.Ok -> {
                    if (current() != opened.value || openingBinding != opened.value) { clearLocked(EnterpriseAccessError.STALE); return }
                    binding = opened.value
                    code = normalized
                    mutable.value = mutable.value.copy(signedIn = true)
                    opened.value
                }
            }
        }
        val response = safely(token) { gateway.preview(selected, normalized) }
        synchronized(monitor) {
            if (!retainLocked(token, selected)) return
            when (response) {
                is EnterpriseAccessResult.Err -> failLocked(response.error)
                is EnterpriseAccessResult.Ok -> mutable.value = EnterpriseInvitationState(
                    stage = EnterpriseInvitationStage.PREVIEW, signedIn = true, preview = response.value,
                )
            }
        }
    }

    suspend fun accept(consented: Boolean) { if (consented) decide(EnterpriseInvitationDecision.ACCEPT) }
    suspend fun decline() = decide(EnterpriseInvitationDecision.DECLINE)

    private suspend fun decide(action: EnterpriseInvitationDecision) {
        data class Attempt(val token: Long, val binding: EnterpriseAccessBinding, val code: String, val digest: String, val operation: String, val confirmationRequired: Boolean)
        val attempt = synchronized(monitor) {
            if (mutable.value.busy || mutable.value.stage != EnterpriseInvitationStage.PREVIEW) return
            val selected = binding ?: return
            if (!retainLocked(generation, selected)) return
            val preview = mutable.value.preview ?: return
            if (decision != null && decision!!.first != action) return
            val operation = decision?.second ?: try { EnterprisePartition.requireId(newOperationId()) } catch (_: Exception) {
                mutable.value = mutable.value.copy(error = EnterpriseAccessError.UNAVAILABLE); return
            }
            decision = action to operation
            mutable.value = mutable.value.copy(busy = true, error = null, retryDecision = action)
            Attempt(generation, selected, code ?: return, preview.consentDigest, operation, preview.confirmationRequired)
        }
        val response = safely(attempt.token) {
            when (action) {
                EnterpriseInvitationDecision.ACCEPT -> gateway.accept(attempt.binding, attempt.operation, attempt.code, attempt.digest)
                EnterpriseInvitationDecision.DECLINE -> gateway.decline(attempt.binding, attempt.operation, attempt.code)
            }
        }
        synchronized(monitor) {
            if (!retainLocked(attempt.token, attempt.binding)) return
            when (response) {
                is EnterpriseAccessResult.Err -> failLocked(response.error)
                is EnterpriseAccessResult.Ok -> {
                    val expected = if (action == EnterpriseInvitationDecision.ACCEPT) "ACCEPTED" else "DECLINED"
                    if (response.value.state != expected || (action == EnterpriseInvitationDecision.ACCEPT && response.value.awaitingConfirmation != attempt.confirmationRequired)) {
                        failLocked(EnterpriseAccessError.UNAVAILABLE); return
                    }
                    code = null; decision = null
                    mutable.value = EnterpriseInvitationState(stage = EnterpriseInvitationStage.RESULT, signedIn = true, acknowledgement = response.value)
                }
            }
        }
    }

    private fun retainLocked(token: Long, selected: EnterpriseAccessBinding): Boolean {
        if (token != generation) return false
        if (binding != selected || current() != selected) { clearLocked(EnterpriseAccessError.STALE); return false }
        return true
    }

    private fun failLocked(error: EnterpriseAccessError) {
        if (error in setOf(EnterpriseAccessError.SIGN_IN_REQUIRED, EnterpriseAccessError.FRESH_AUTH_REQUIRED, EnterpriseAccessError.STALE,
                EnterpriseAccessError.INVALID_CODE, EnterpriseAccessError.CONFLICT, EnterpriseAccessError.ALREADY_MEMBER, EnterpriseAccessError.MANAGED_RESTRICTED)) {
            clearLocked(error)
        } else mutable.value = mutable.value.copy(busy = false, error = error)
    }

    private suspend fun <T> safely(token: Long, work: suspend () -> EnterpriseAccessResult<T>): EnterpriseAccessResult<T> = try {
        work()
    } catch (cancelled: CancellationException) {
        synchronized(monitor) { if (token == generation) clearLocked() }
        throw cancelled
    } catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
}
