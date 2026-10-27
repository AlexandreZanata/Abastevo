package com.anpfuel.application.usecase.contributor

import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ContributorPage
import com.anpfuel.domain.repository.ContributorsGateway

/**
 * B-BR-CTB public contributors page (CTB-09/10).
 *
 * Anonymous: no session is required or accepted. The cursor is opaque;
 * only its length is guarded before it travels.
 */
class GetContributorsUseCase(
    private val gateway: ContributorsGateway,
) {
    suspend operator fun invoke(cursor: String?): ContributorPage {
        if (cursor != null && cursor.length > 4096) {
            throw ContributorException.Invalid("contributors.invalid-cursor")
        }
        return gateway.page(cursor)
    }
}
