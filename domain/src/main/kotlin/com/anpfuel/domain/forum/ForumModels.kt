package com.anpfuel.domain.forum

/**
 * Source attribution for an editorial or news post (B-BR-FOR-02).
 */
data class ForumSourceAttribution(
    val name: String,
    val url: String,
    val attribution: String,
    val licenseUrl: String? = null,
)

/**
 * Geographic scope for regional fuel news and regulations.
 */
data class ForumRegion(
    val uf: String? = null,
    val municipalityIbge: Int? = null,
)

/**
 * Public summary of a forum post in feeds and cards.
 */
data class ForumPostSummary(
    val id: String,
    val kind: ForumPostKind,
    val rawKind: String,
    val category: ForumCategory,
    val rawCategory: String,
    val title: String,
    val excerpt: String,
    val source: ForumSourceAttribution? = null,
    val publicAlias: String? = null,
    val firstPublishedAt: String,
    val updatedAt: String,
    val revision: Long = 1,
    val commentsEnabled: Boolean = true,
    val region: ForumRegion? = null,
    val fuelProduct: String? = null,
    val stationId: String? = null,
)

/**
 * Full detail of a forum post for the reading screen.
 */
data class ForumPostDetail(
    val id: String,
    val kind: ForumPostKind,
    val rawKind: String,
    val category: ForumCategory,
    val rawCategory: String,
    val title: String,
    val excerpt: String,
    val bodyText: String? = null,
    val source: ForumSourceAttribution? = null,
    val publicAlias: String? = null,
    val sourcePublishedAt: String? = null,
    val firstPublishedAt: String,
    val updatedAt: String,
    val revision: Long = 1,
    val commentsEnabled: Boolean = true,
    val correction: String? = null,
    val region: ForumRegion? = null,
    val fuelProduct: String? = null,
    val stationId: String? = null,
)

/**
 * A page of public forum posts with opaque cursor and snapshot revision.
 */
data class ForumFeedPage(
    val items: List<ForumPostSummary>,
    val nextCursor: String? = null,
    val snapshot: String,
    val fetchedAt: String,
)

/**
 * Public comment on a published forum post or reply to a comment (FOR-14, FOR-17, FOR-19).
 */
data class ForumComment(
    val id: String,
    val postId: String,
    val parentId: String? = null,
    val depth: Int = 0,
    val sequence: Long,
    val publicAlias: String? = null,
    val bodyText: String,
    val createdAt: String,
    val updatedAt: String,
    val publishedAt: String? = null,
    val replyCount: Int = 0,
    val replies: List<ForumComment> = emptyList(),
)

/**
 * A page of public top-level comments with opaque cursor.
 */
data class ForumCommentsPage(
    val items: List<ForumComment>,
    val nextCursor: String? = null,
)

/**
 * A page of public single-level replies with opaque cursor.
 */
data class ForumRepliesPage(
    val items: List<ForumComment>,
    val nextCursor: String? = null,
)

/**
 * Immutable receipt of a comment creation, edit, or removal (FOR-14, FOR-15).
 */
data class ForumCommentReceipt(
    val id: String,
    val postId: String,
    val parentId: String? = null,
    val depth: Int = 0,
    val sequence: Long = 0,
    val revision: Long = 1,
    val publicationState: String,
    val visibility: String,
    val bodyText: String,
    val createdAt: String,
    val updatedAt: String,
    val publishedAt: String? = null,
    val replayed: Boolean = false,
)

/**
 * Report reason options matching the backend moderation contract (FOR-18).
 */
enum class ForumReportReason(val wireCode: String) {
    SPAM("SPAM"),
    OFFENSIVE("OFFENSIVE"),
    MISINFORMATION("MISINFORMATION"),
    COMMERCIAL("COMMERCIAL"),
    LEGAL_NOTICE("LEGAL_NOTICE"),
    OTHER("OTHER");

    companion object {
        fun fromWire(value: String): ForumReportReason {
            return entries.firstOrNull { it.wireCode.equals(value, ignoreCase = true) } ?: OTHER
        }
    }
}

/**
 * Immutable acknowledgment of a filed moderation report (FOR-18).
 */
data class ForumReportReceipt(
    val reportId: String,
    val caseId: String,
    val targetType: String,
    val targetId: String,
    val reason: String,
    val status: String,
    val createdAt: String,
    val replayed: Boolean = false,
)
