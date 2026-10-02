package com.anpfuel.application.usecase.feedback

import com.anpfuel.application.port.FeedbackFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.repository.FeedbackCacheRepository
import com.anpfuel.domain.repository.FeedbackException
import com.anpfuel.domain.repository.FeedbackGateway
import com.anpfuel.domain.repository.FeedbackPage
import com.anpfuel.domain.repository.RatingStatsSnapshot

/**
 * P17-T01 — Shared feedback reads (B-BR-F06/F07, BUC-F05).
 *
 * Public reads need no login: anonymous browsing stays available and
 * serves only opaque aliases. Fresh gateway pages are cached; gateway
 * transport failure falls back to the cached first page
 * ([FeedbackPageOutcome.StaleCache], explicit, never fresh); without
 * cache the outcome is [FeedbackPageOutcome.Unavailable]. Limits
 * clamp to 1…100 (default 20), mirroring the backend. Corrupt cursors
 * and other server refusals fail closed as unavailable, never as a
 * crash; blank targets throw [DomainException] fail-fast.
 */
sealed interface FeedbackPageOutcome {
    data object Disabled : FeedbackPageOutcome
    data class Fresh(val page: FeedbackPage) : FeedbackPageOutcome
    data class StaleCache(val page: FeedbackPage, val cause: Throwable) : FeedbackPageOutcome
    data class Unavailable(val cause: Throwable) : FeedbackPageOutcome
}

/**
 * Public rating aggregate outcome (P22-T02, B-BR-F02): exact
 * count/sum, zeroed when no rating exists — never an error or an
 * invented mean. Reads need no login.
 */
sealed interface FeedbackStatsOutcome {
    data object Disabled : FeedbackStatsOutcome
    data class Fresh(val stats: RatingStatsSnapshot) : FeedbackStatsOutcome
    data class Unavailable(val cause: Throwable) : FeedbackStatsOutcome
}

class GetFeedbackPageUseCase(
    private val flagProvider: FeedbackFlagProvider,
    private val gateway: FeedbackGateway,
    private val cache: FeedbackCacheRepository,
) {
    suspend fun comments(
        stationId: String,
        product: String,
        cursor: String,
        limit: Int,
    ): FeedbackPageOutcome {
        if (!flagProvider.isEnabled()) return FeedbackPageOutcome.Disabled
        if (stationId.isBlank() || product.isBlank()) throw DomainException("station/product target is blank")
        val want = clampLimit(limit)
        return try {
            val page = gateway.listComments(stationId.trim(), product.trim(), cursor, want)
            cache.saveComments(stationId.trim(), product.trim(), page)
            FeedbackPageOutcome.Fresh(page)
        } catch (refused: FeedbackException) {
            FeedbackPageOutcome.Unavailable(refused)
        } catch (transport: Exception) {
            val cached = try {
                cache.loadComments(stationId.trim(), product.trim())
            } catch (_: Exception) {
                null
            }
            if (cached != null) FeedbackPageOutcome.StaleCache(cached, transport)
            else FeedbackPageOutcome.Unavailable(transport)
        }
    }

    suspend fun replies(
        parentId: String,
        cursor: String,
        limit: Int,
    ): FeedbackPageOutcome {
        if (!flagProvider.isEnabled()) return FeedbackPageOutcome.Disabled
        if (parentId.isBlank()) throw DomainException("parent_id is blank")
        val want = clampLimit(limit)
        return try {
            FeedbackPageOutcome.Fresh(gateway.listReplies(parentId.trim(), cursor, want))
        } catch (refused: FeedbackException) {
            FeedbackPageOutcome.Unavailable(refused)
        } catch (transport: Exception) {
            FeedbackPageOutcome.Unavailable(transport)
        }
    }
    private fun clampLimit(limit: Int): Int =
        when {
            limit <= 0 -> DEFAULT_LIMIT
            limit > MAX_LIMIT -> MAX_LIMIT
            else -> limit
        }

    suspend fun stats(stationId: String, product: String): FeedbackStatsOutcome {
        if (!flagProvider.isEnabled()) return FeedbackStatsOutcome.Disabled
        if (stationId.isBlank() || product.isBlank()) throw DomainException("station/product target is blank")
        return try {
            FeedbackStatsOutcome.Fresh(gateway.stats(stationId.trim(), product.trim()))
        } catch (error: Exception) {
            FeedbackStatsOutcome.Unavailable(error)
        }
    }

    companion object {
        const val DEFAULT_LIMIT = 20
        const val MAX_LIMIT = 100
    }
}
