package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumFeedFilters
import com.anpfuel.domain.forum.ForumFeedPage
import com.anpfuel.domain.forum.ForumPostKind
import com.anpfuel.domain.repository.ForumGateway

/**
 * Reads paginated public forum feed items (B-BR-FOR-01..07 / BUC-FOR-01).
 *
 * Enforces client-side bounds:
 * - limit: between 1 and 50 inclusive (default 20).
 * - cursor: maximum length 512 characters.
 */
class GetForumFeedUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(
        category: ForumCategory? = null,
        kind: ForumPostKind? = null,
        limit: Int = DEFAULT_LIMIT,
        cursor: String? = null,
        filters: ForumFeedFilters = ForumFeedFilters(),
    ): ForumFeedPage {
        if (limit !in MIN_LIMIT..MAX_LIMIT) {
            throw ForumException.Invalid("forum.bad-limit")
        }
        val trimmedCursor = cursor?.trim()?.takeIf { it.isNotEmpty() }
        if (trimmedCursor != null && trimmedCursor.length > MAX_CURSOR_LENGTH) {
            throw ForumException.Invalid("forum.bad-cursor")
        }

        return gateway.getFeed(
            category = category,
            kind = kind,
            limit = limit,
            cursor = trimmedCursor,
            filters = validateFilters(filters),
        )
    }

    private fun validateFilters(filters: ForumFeedFilters): ForumFeedFilters {
        val uf = filters.uf?.trim()?.uppercase()?.takeIf { it.isNotEmpty() }
        if (uf != null && !SubmitForumTopicUseCase.VALID_UFS.containsKey(uf)) {
            throw ForumException.Invalid("forum.bad-region-uf")
        }

        val ibge = filters.municipalityIbge
        if (ibge != null) {
            if (ibge !in 1000000..9999999) {
                throw ForumException.Invalid("forum.bad-region-ibge")
            }
            if (uf != null && ibge / 100000 != SubmitForumTopicUseCase.VALID_UFS.getValue(uf)) {
                throw ForumException.Invalid("forum.bad-region-prefix")
            }
        }

        val fuel = filters.fuelProduct?.trim()?.takeIf { it.isNotEmpty() }
        if (fuel != null && !SubmitForumTopicUseCase.VALID_FUELS.contains(fuel)) {
            throw ForumException.Invalid("forum.bad-fuel-product")
        }

        val stationId = filters.stationId?.trim()?.lowercase()?.takeIf { it.isNotEmpty() }
        if (stationId != null && !SubmitForumTopicUseCase.UUID_REGEX.matches(stationId)) {
            throw ForumException.Invalid("forum.bad-station-id")
        }

        return ForumFeedFilters(
            uf = uf,
            municipalityIbge = ibge,
            fuelProduct = fuel,
            stationId = stationId,
        )
    }

    companion object {
        const val DEFAULT_LIMIT = 20
        const val MIN_LIMIT = 1
        const val MAX_LIMIT = 50
        const val MAX_CURSOR_LENGTH = 512
    }
}
