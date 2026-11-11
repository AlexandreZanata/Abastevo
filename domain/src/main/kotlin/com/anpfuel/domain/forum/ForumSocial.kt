package com.anpfuel.domain.forum

/** One account's reaction to a post or comment (B-BR-FOR-17). */
enum class ForumReaction(val wire: String) {
    LIKE("LIKE"),
    DISLIKE("DISLIKE"),
    NONE("NONE"),
    ;

    /** Tapping the active reaction clears it; tapping the other one switches. */
    fun toggle(tapped: ForumReaction): ForumReaction = if (this == tapped) NONE else tapped

    companion object {
        fun fromWire(wire: String?): ForumReaction = entries.firstOrNull { it.wire == wire } ?: NONE
    }
}

/** What a reaction applies to; the value is the API path segment. */
enum class ForumReactionTarget(val pathSegment: String) {
    POST("posts"),
    COMMENT("comments"),
}

/** Order of top-level replies (B-BR-FOR-18). */
enum class ForumReplySort(val wire: String) {
    /** Likes minus dislikes, newest first on ties: the app default. */
    RELEVANT("relevant"),
    RECENT("recent"),
    LIKED("liked"),
}

/** Public counts of a post or comment. */
data class ForumReactionCounts(val likes: Long = 0, val dislikes: Long = 0) {

    /** Moves one account's reaction, never below zero; mirrors the server counters. */
    fun applying(from: ForumReaction, to: ForumReaction): ForumReactionCounts {
        if (from == to) return this
        var likes = likes
        var dislikes = dislikes
        when (from) {
            ForumReaction.LIKE -> likes = (likes - 1).coerceAtLeast(0)
            ForumReaction.DISLIKE -> dislikes = (dislikes - 1).coerceAtLeast(0)
            ForumReaction.NONE -> Unit
        }
        when (to) {
            ForumReaction.LIKE -> likes += 1
            ForumReaction.DISLIKE -> dislikes += 1
            ForumReaction.NONE -> Unit
        }
        return ForumReactionCounts(likes, dislikes)
    }
}

/** Server answer to a reaction command. */
data class ForumReactionResult(
    val targetId: String,
    val reaction: ForumReaction,
    val counts: ForumReactionCounts,
)

/** The caller's own reactions in one conversation (signed owner read). */
data class ForumOwnReactions(
    val post: ForumReaction = ForumReaction.NONE,
    val comments: Map<String, ForumReaction> = emptyMap(),
)

/** Events of a live conversation stream (B-BR-FOR-18). */
sealed interface ForumLiveEvent {
    data object Ready : ForumLiveEvent

    /** A reply became public; it is offered behind "new replies", never inserted under the reader. */
    data class CommentPublished(val comment: ForumComment) : ForumLiveEvent

    data class CommentReactions(
        val commentId: String,
        val parentId: String?,
        val counts: ForumReactionCounts,
    ) : ForumLiveEvent

    /** A reply left public reads (hidden, removed or edited back into review). */
    data class CommentGone(val commentId: String, val parentId: String?) : ForumLiveEvent

    data class PostReactions(val counts: ForumReactionCounts) : ForumLiveEvent

    data object PostGone : ForumLiveEvent

    /** Events may have been lost: reload the conversation. */
    data object Resync : ForumLiveEvent

    /** The server ended the stream on purpose; reconnect later. */
    data object Reconnect : ForumLiveEvent
}
