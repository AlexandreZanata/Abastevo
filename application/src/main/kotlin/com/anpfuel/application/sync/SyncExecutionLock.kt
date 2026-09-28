package com.anpfuel.application.sync

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Serializes UC-001 sync runs so concurrent callers cannot corrupt [SyncJobState] (BR-015).
 */
class SyncExecutionLock {

    private val mutex = Mutex()

    suspend fun <T> withLock(block: suspend () -> T): T = mutex.withLock { block() }
}
