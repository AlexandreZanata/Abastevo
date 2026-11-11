package com.anpfuel.application.usecase.forum

import com.anpfuel.application.port.ForumLivePort
import com.anpfuel.domain.forum.ForumCommentsPage
import com.anpfuel.domain.forum.ForumException
import com.anpfuel.domain.forum.ForumLiveEvent
import com.anpfuel.domain.forum.ForumOwnReactions
import com.anpfuel.domain.forum.ForumReaction
import com.anpfuel.domain.forum.ForumReactionResult
import com.anpfuel.domain.forum.ForumReactionTarget
import com.anpfuel.domain.forum.ForumReplySort
import com.anpfuel.domain.repository.ForumGateway
import kotlinx.coroutines.flow.Flow

private val UUID_REGEX = Regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

private fun requireUuid(value: String): String {
    val trimmed = value.trim()
    if (!UUID_REGEX.matches(trimmed)) throw ForumException.Invalid("forum.bad-id")
    return trimmed
}

/** Ranked first page of replies with best answer previews (B-BR-FOR-18). */
class GetRankedForumCommentsUseCase(private val gateway: ForumGateway) {
    suspend operator fun invoke(
        postId: String,
        sort: ForumReplySort,
        limit: Int = 20,
        cursor: String? = null,
    ): ForumCommentsPage =
        gateway.getRankedComments(requireUuid(postId), sort, limit.coerceIn(1, 50), cursor?.takeIf { it.isNotBlank() })
}

/** Sets the caller's reaction on a post or comment (B-BR-FOR-17). */
class SetForumReactionUseCase(private val gateway: ForumGateway) {
    suspend operator fun invoke(
        target: ForumReactionTarget,
        targetId: String,
        reaction: ForumReaction,
        operationId: String,
    ): ForumReactionResult = gateway.setReaction(target, requireUuid(targetId), reaction, requireUuid(operationId))
}

/** Reads the caller's own reactions in one conversation. */
class GetOwnForumReactionsUseCase(private val gateway: ForumGateway) {
    suspend operator fun invoke(postId: String): ForumOwnReactions = gateway.getOwnReactions(requireUuid(postId))
}

/** Follows one open conversation live. */
class ObserveForumConversationUseCase(private val live: ForumLivePort) {
    operator fun invoke(postId: String): Flow<ForumLiveEvent> = live.conversation(requireUuid(postId))
}
