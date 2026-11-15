package com.anpfuel.application.usecase.safety

import com.anpfuel.domain.safety.BlockedUser
import com.anpfuel.domain.safety.CommunitySafetyGateway
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Process-wide knowledge of how many blocks the signed account holds, so
 * screens skip the hidden-content question while it has none
 * (B-BR-SAFE-06). The count loads lazily from the block list once per
 * account, follows block/unblock writes, and is keyed by the account id:
 * signing out or switching accounts never reuses another account's state.
 *
 * [revisions] increases on every block or unblock so screens already on
 * display can filter again.
 */
class CommunityBlockState(
    private val gateway: CommunitySafetyGateway,
    private val accountId: () -> String?,
) {
    private val mutex = Mutex()
    private var account: String? = null

    /** Exact active block count of [account], or null while unknown. */
    private var count: Int? = null

    private val _revisions = MutableStateFlow(0L)
    val revisions: StateFlow<Long> = _revisions.asStateFlow()

    /**
     * False only when nobody is signed in or the account is known to hold no
     * blocks. When the list cannot be read the answer is true, so callers
     * still ask (and fail open themselves).
     */
    suspend fun mayHaveBlocks(): Boolean {
        val current = currentAccount() ?: return false
        return mutex.withLock {
            syncAccount(current)
            val known = count
            if (known != null) return@withLock known > 0
            val loaded = try {
                gateway.listBlocks()
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                return@withLock true
            }
            count = loaded.size
            loaded.isNotEmpty()
        }
    }

    /** A fresh list read is the exact count. */
    suspend fun onListed(items: List<BlockedUser>) {
        val current = currentAccount() ?: return
        mutex.withLock {
            syncAccount(current)
            count = items.size
        }
    }

    suspend fun onBlocked(replayed: Boolean) {
        val current = currentAccount() ?: return
        mutex.withLock {
            syncAccount(current)
            // An unknown count stays unknown: the next question reloads it.
            if (!replayed) count = count?.plus(1)
        }
        _revisions.update { it + 1 }
    }

    suspend fun onUnblocked() {
        val current = currentAccount() ?: return
        mutex.withLock {
            syncAccount(current)
            count = count?.minus(1)?.coerceAtLeast(0)
        }
        _revisions.update { it + 1 }
    }

    /** Forgets the count of the current account; the next question reloads it. */
    suspend fun invalidate() {
        mutex.withLock { count = null }
    }

    private fun currentAccount(): String? = try {
        accountId()?.takeIf { it.isNotBlank() }
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        null
    }

    private fun syncAccount(current: String) {
        if (current != account) {
            account = current
            count = null
        }
    }
}
