package com.anpfuel.application.enterprise

import java.util.concurrent.locks.ReentrantLock
import java.util.concurrent.atomic.AtomicLong
import kotlin.concurrent.withLock
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** Shared with session mutation, local I/O and backgrounding; contains no credentials. */
class EnterpriseSessionFence {
    private val mutex = ReentrantLock()
    private val generation = AtomicLong()
    private val background = AtomicLong()
    private val observedEpoch = MutableStateFlow(0L)
    val epochs: StateFlow<Long> = observedEpoch.asStateFlow()
    val epoch: Long get() = generation.get()
    val backgroundEpoch: Long get() = background.get()
    private fun advance() {
        val next = generation.incrementAndGet()
        observedEpoch.update { maxOf(it, next) }
    }
    fun lock() { mutex.withLock { advance() } }
    // Main-thread lifecycle invalidation must not wait for filesystem I/O.
    fun markBackground() {
        background.incrementAndGet()
        advance()
    }
    fun <T> access(work: () -> T): T = mutex.withLock(work)
}
