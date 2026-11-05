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
