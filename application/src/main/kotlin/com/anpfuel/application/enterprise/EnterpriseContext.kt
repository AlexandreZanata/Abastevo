package com.anpfuel.application.enterprise

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

typealias EnterpriseContextSummary = com.anpfuel.domain.enterprise.EnterpriseContextSummary
typealias EnterpriseContextPage = com.anpfuel.domain.enterprise.EnterpriseContextPage

/** Every descriptor is a selector; read rechecks current server membership before selection. */
interface EnterpriseContextGateway {
    fun currentBinding(): EnterpriseAccessBinding?
    suspend fun open(): EnterpriseAccessResult<EnterpriseAccessBinding>
    suspend fun list(binding: EnterpriseAccessBinding, afterOrganizationId: String? = null): EnterpriseAccessResult<EnterpriseContextPage>
    suspend fun read(binding: EnterpriseAccessBinding, context: EnterpriseContextSummary): EnterpriseAccessResult<EnterpriseContextSummary>
    fun invalidate()
}

data class EnterpriseContextState(
    val signedIn: Boolean = false,
    val busy: Boolean = false,
    val contexts: List<EnterpriseContextSummary> = emptyList(),
    val selected: EnterpriseContextSummary? = null,
    val catalogConfirmed: Boolean = false,
    val selectionConfirmed: Boolean = false,
    val nextAfterOrganizationId: String? = null,
    val error: EnterpriseAccessError? = null,
) {
    override fun toString(): String = "EnterpriseContextState(busy=$busy,confirmed=$selectionConfirmed,redacted)"
}

/** Memory-only selector and home shell. It never returns a store handle or an offline permission. */
class EnterpriseContextFlow(
    private val gateway: EnterpriseContextGateway,
    private val localContext: EnterpriseLocalContext,
) {
    private val monitor = Any()
    private var observed = current()
    private val mutable = MutableStateFlow(EnterpriseContextState(signedIn = observed != null))
    val state: StateFlow<EnterpriseContextState> = mutable.asStateFlow()
    private var generation = 0L
    private var source: EnterpriseAccessBinding? = null
    private var cacheOwner: EnterpriseAccessBinding? = null
    private var closed = false
    private val localOwner = EnterpriseLocalContext.Owner()

    private data class Attempt(val token: Long, val binding: EnterpriseAccessBinding, val desired: EnterpriseContextSummary?, val lease: EnterpriseLocalContext.Lease? = null)

    private fun current(): EnterpriseAccessBinding? = try { gateway.currentBinding() } catch (_: Exception) { null }
    private fun sameIdentity(a: EnterpriseAccessBinding?, b: EnterpriseAccessBinding?): Boolean =
        a != null && b != null && a.accountId == b.accountId && a.origin == b.origin && a.familyId == b.familyId

    private fun withdrawLocked(error: EnterpriseAccessError? = null, clear: Boolean = false): EnterpriseLocalContext.Lease? {
        val now = current()
        val keep = !clear && sameIdentity(cacheOwner, now)
        generation++
        source = null
        observed = now
        val localLease = localContext.invalidate(localOwner)
        try { gateway.invalidate() } catch (_: Exception) { /* No stale local authority survives a transport failure. */ }
        mutable.value = EnterpriseContextState(
            signedIn = now != null,
            contexts = if (keep) mutable.value.contexts else emptyList(),
            selected = if (keep) mutable.value.selected else null,
            error = error,
        )
        if (!keep) cacheOwner = null
        return localLease
    }

    fun invalidateIfStale() {
        synchronized(monitor) { if (!closed && observed != current()) withdrawLocked(EnterpriseAccessError.STALE) }
    }

    fun unconfirm() { synchronized(monitor) { if (!closed) withdrawLocked() } }

    fun close() {
        synchronized(monitor) { if (!closed) { withdrawLocked(clear = true); closed = true } }
    }

    private fun begin(desired: EnterpriseContextSummary?, refresh: Boolean): Attempt? = synchronized(monitor) {
        if (closed || mutable.value.busy) return@synchronized null
        withdrawLocked()
        val initial = current()
        if (initial == null) {
            mutable.value = mutable.value.copy(error = EnterpriseAccessError.SIGN_IN_REQUIRED)
            return@synchronized null
        }
        val selected = if (refresh) mutable.value.selected else desired
        if (selected != null && (selected.partition.ownerAccount != initial.accountId || selected.partition.origin != initial.origin)) {
            withdrawLocked(EnterpriseAccessError.ACCESS_DENIED, clear = true)
            return@synchronized null
        }
        observed = initial
        mutable.value = mutable.value.copy(busy = true)
        Attempt(generation, initial, selected, localContext.acquire(localOwner))
    }

    private fun retainedLocked(attempt: Attempt): Boolean {
        if (closed || generation != attempt.token) return false
        if (current() != attempt.binding) { withdrawLocked(EnterpriseAccessError.STALE); return false }
        return true
    }

    private suspend fun prepare(attempt: Attempt): Boolean {
        try {
            if (!localContext.lock(attempt.lease ?: return false)) {
                synchronized(monitor) { if (retainedLocked(attempt)) failureLocked(EnterpriseAccessError.STALE) }
                return false
            }
        }
        catch (cancelled: CancellationException) { cancel(attempt); throw cancelled }
        catch (_: Exception) { synchronized(monitor) { if (retainedLocked(attempt)) failureLocked(EnterpriseAccessError.UNAVAILABLE) }; return false }
        if (!synchronized(monitor) { retainedLocked(attempt) }) return false
        val result = request(attempt) { gateway.open() }
        return synchronized(monitor) {
            if (!retainedLocked(attempt)) false
            else when (result) {
                is EnterpriseAccessResult.Err -> { failureLocked(result.error); false }
                is EnterpriseAccessResult.Ok -> {
                    if (result.value != attempt.binding) { withdrawLocked(EnterpriseAccessError.STALE); false }
                    else { source = result.value; true }
                }
            }
        }
    }

    suspend fun refresh() {
        val attempt = begin(null, refresh = true) ?: return
        if (!prepare(attempt)) return
        val result = request(attempt) { gateway.list(attempt.binding) }
        val accepted = synchronized(monitor) {
            if (!retainedLocked(attempt)) false
            else when (result) {
                is EnterpriseAccessResult.Err -> { failureLocked(result.error); false }
                is EnterpriseAccessResult.Ok -> acceptPageLocked(attempt, result.value)
            }
        }
        if (!accepted) return
        if (attempt.desired != null) selectCurrent(attempt, attempt.desired)
        else synchronized(monitor) { if (retainedLocked(attempt)) mutable.value = mutable.value.copy(busy = false) }
    }

    suspend fun loadMore() {
        val paging = synchronized(monitor) {
            if (closed || mutable.value.busy || !mutable.value.catalogConfirmed) return
            val binding = source ?: return
            val cursor = mutable.value.nextAfterOrganizationId ?: return
            val attempt = Attempt(++generation, binding, mutable.value.selected)
            if (!retainedLocked(attempt)) return
            mutable.value = mutable.value.copy(busy = true, error = null)
            attempt to cursor
        }
        val (attempt, cursor) = paging
        val result = request(attempt) { gateway.list(attempt.binding, cursor) }
        synchronized(monitor) {
            if (!retainedLocked(attempt)) return
            when (result) {
                is EnterpriseAccessResult.Err -> failureLocked(result.error)
                is EnterpriseAccessResult.Ok -> {
                    val page = result.value
                    val next = page.nextAfterOrganizationId
                    if (page.contexts.any { it.partition.organizationId <= cursor } ||
                        (next != null && next <= cursor)) {
                        failureLocked(EnterpriseAccessError.UNAVAILABLE)
                    } else if (acceptPageLocked(attempt, page)) mutable.value = mutable.value.copy(busy = false)
                }
            }
        }
    }

    private fun acceptPageLocked(attempt: Attempt, page: EnterpriseContextPage): Boolean {
        if (page.contexts.any { it.partition.ownerAccount != attempt.binding.accountId || it.partition.origin != attempt.binding.origin }) {
            failureLocked(EnterpriseAccessError.ACCESS_DENIED); return false
        }
        cacheOwner = attempt.binding
        mutable.value = mutable.value.copy(contexts = page.contexts.toList(), catalogConfirmed = true,
            nextAfterOrganizationId = page.nextAfterOrganizationId, error = null)
        return true
    }

    suspend fun select(context: EnterpriseContextSummary) {
        val attempt = begin(context, refresh = false) ?: return
        if (prepare(attempt)) selectCurrent(attempt, context)
    }

    private suspend fun selectCurrent(attempt: Attempt, expected: EnterpriseContextSummary) {
        val result = request(attempt) { gateway.read(attempt.binding, expected) }
        val selected = synchronized(monitor) {
            if (!retainedLocked(attempt)) return
            when (result) {
                is EnterpriseAccessResult.Err -> { failureLocked(result.error); return }
                is EnterpriseAccessResult.Ok -> {
                    val value = result.value
                    if (value.partition != expected.partition || value.membershipId != expected.membershipId) {
                        failureLocked(EnterpriseAccessError.ACCESS_DENIED); return
                    }
                    value
                }
            }
        }
        val handle = try { localContext.select(attempt.lease ?: return, selected.partition) }
        catch (cancelled: CancellationException) { cancel(attempt); throw cancelled }
        catch (_: Exception) { synchronized(monitor) { if (retainedLocked(attempt)) failureLocked(EnterpriseAccessError.UNAVAILABLE) }; return }
        synchronized(monitor) {
            if (!retainedLocked(attempt)) return
            if (!handle.isCurrent()) { failureLocked(EnterpriseAccessError.STALE); return }
            cacheOwner = attempt.binding
            mutable.value = mutable.value.copy(busy = false, selected = selected, selectionConfirmed = true, error = null)
        }
    }

    suspend fun selectPersonal() {
        val lease = synchronized(monitor) {
            if (closed) return
            val lease = withdrawLocked()
            mutable.value = mutable.value.copy(selected = null)
            lease
        }
        // Local invalidation above precedes this first suspension. Existing writes keep their original scope.
        if (lease != null) localContext.lock(lease) // A newer owner's lease makes this a harmless no-op.
    }

    private fun failureLocked(error: EnterpriseAccessError) {
        withdrawLocked(error, clear = error in setOf(EnterpriseAccessError.ACCESS_DENIED, EnterpriseAccessError.SIGN_IN_REQUIRED,
            EnterpriseAccessError.FRESH_AUTH_REQUIRED, EnterpriseAccessError.MANAGED_RESTRICTED))
    }

    private fun cancel(attempt: Attempt) = synchronized(monitor) { if (generation == attempt.token) withdrawLocked() }

    private suspend fun <T> request(attempt: Attempt, work: suspend () -> EnterpriseAccessResult<T>): EnterpriseAccessResult<T> = try {
        work()
    } catch (cancelled: CancellationException) { cancel(attempt); throw cancelled }
    catch (_: Exception) { EnterpriseAccessResult.Err(EnterpriseAccessError.UNAVAILABLE) }
}
