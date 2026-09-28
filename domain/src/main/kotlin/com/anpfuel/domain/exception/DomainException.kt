package com.anpfuel.domain.exception

open class DomainException(
    message: String,
    cause: Throwable? = null,
) : RuntimeException(message, cause)
