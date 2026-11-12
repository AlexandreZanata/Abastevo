package com.anpfuel.application.enterprise

import java.util.concurrent.locks.ReentrantLock
import java.util.concurrent.atomic.AtomicLong
import kotlin.concurrent.withLock

/** Shared with session mutation, local I/O and backgrounding; contains no credentials. */
class EnterpriseSessionFence {
    private val mutex = ReentrantLock()
    private val generation = AtomicLong()
    private val background = AtomicLong()
    val epoch: Long get() = generation.get()
    val backgroundEpoch: Long get() = background.get()
    fun lock() { mutex.withLock { generation.incrementAndGet() } }
    // Main-thread lifecycle invalidation must not wait for filesystem I/O.
    fun markBackground() {
        generation.incrementAndGet()
        background.incrementAndGet()
    }
    fun <T> access(work: () -> T): T = mutex.withLock(work)
}
