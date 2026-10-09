package com.anpfuel.data.local.auth

import com.anpfuel.application.portable.AuthOperationLock
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

/** One singleton flow owns this lock, including refresh and explicit logout. */
class SerializedAuthOperations : AuthOperationLock {
    private val lock = ReentrantLock()

    override fun <T> withLock(operation: () -> T): T = lock.withLock(operation)
}
