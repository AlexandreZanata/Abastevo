package com.anpfuel.application.error

import com.anpfuel.domain.exception.DomainException
import java.io.IOException
import java.io.InterruptedIOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import java.nio.charset.MalformedInputException
import javax.net.ssl.SSLException

/**
 * Maps infrastructure and domain failures to [AppError] for use cases and ViewModels.
 */
object AppErrorResolver {

    fun fromThrowable(throwable: Throwable): AppError {
        val root = rootCause(throwable)
        if (root is DomainException) {
            fromDomainException(root)?.let { return it }
        }
        return fromInfrastructureThrowable(root)
    }

    fun fromDomainException(exception: DomainException): AppError? {
        val message = exception.message.orEmpty()
        return when {
            message.contains("BR-001") || message.contains("BR-002") -> AppError.SyncParseError
            else -> null
        }
    }

    internal fun fromInfrastructureThrowable(throwable: Throwable): AppError {
        if (isStorageFull(throwable)) {
            return AppError.StorageFull
        }
        if (isNetworkError(throwable)) {
            return AppError.SyncNetworkError
        }
        if (isParseError(throwable)) {
            return AppError.SyncParseError
        }
        return AppError.SyncNetworkError
    }

    internal fun isNetworkError(throwable: Throwable): Boolean =
        throwable is IOException ||
            throwable is UnknownHostException ||
            throwable is SocketTimeoutException ||
            throwable is SSLException ||
            throwable is InterruptedIOException

    internal fun isParseError(throwable: Throwable): Boolean =
        throwable is IllegalArgumentException ||
            throwable is IllegalStateException ||
            throwable is MalformedInputException ||
            throwable.message?.contains("parse", ignoreCase = true) == true ||
            throwable.message?.contains("xlsx", ignoreCase = true) == true

    internal fun isStorageFull(throwable: Throwable): Boolean {
        val message = throwable.message?.lowercase().orEmpty()
        return message.contains("disk full") ||
            message.contains("sqlite_full") ||
            message.contains("no space left") ||
            throwable.javaClass.simpleName.contains("SQLiteFull", ignoreCase = true)
    }

    private fun rootCause(throwable: Throwable): Throwable {
        var current = throwable
        while (current.cause != null && current.cause !== current) {
            current = requireNotNull(current.cause)
        }
        return current
    }
}
