package com.anpfuel.domain.discovery

import java.io.IOException

/**
 * The server rejected a directory continuation cursor (HTTP 400:
 * tampered, expired after 15 minutes, or bound to other filters).
 * An [IOException] so generic transport handling keeps working; callers
 * that paginate recover by restarting from the first page instead of
 * stranding the list.
 */
class DirectoryCursorExpiredException(
    message: String = "directory cursor expired",
    cause: Throwable? = null,
) : IOException(message, cause)
