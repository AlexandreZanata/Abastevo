package com.anpfuel.application.usecase.forum

import com.anpfuel.domain.forum.ForumCategoriesResult
import com.anpfuel.domain.repository.ForumGateway

/**
 * Reads public taxonomy categories for Forum Abastevo (B-BR-FOR-02 / BUC-FOR-01).
 *
 * Anonymous read: requires no session or credentials.
 */
class GetForumCategoriesUseCase(
    private val gateway: ForumGateway,
) {
    suspend operator fun invoke(): ForumCategoriesResult {
        return gateway.getCategories()
    }
}
