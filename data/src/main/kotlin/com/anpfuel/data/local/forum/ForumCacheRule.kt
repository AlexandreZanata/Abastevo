package com.anpfuel.data.local.forum

import com.anpfuel.domain.forum.ForumCategory
import com.anpfuel.domain.forum.ForumPostKind

object ForumCacheRule {
    const val MAX_CACHE_AGE_MILLIS: Long = 24 * 60 * 60 * 1000L // 24 hours
    const val MAX_TOTAL_BYTES: Long = 2L * 1024L * 1024L // 2 MiB
    const val TARGET_PRUNE_BYTES: Long = 1_800_000L // 1.8 MiB
    const val MAX_FEED_SCOPES: Int = 5 // five scopes
    const val MAX_PAYLOAD_CHARS: Int = 256 * 1024 // 256 KiB

    const val KIND_CATEGORIES = "CATEGORIES"
    const val KIND_FEED = "FEED"
    const val KIND_DETAIL = "DETAIL"

    fun categoriesKey(): String = "categories"

    fun feedKey(category: ForumCategory?, kind: ForumPostKind?): String =
        "feed:${category?.name ?: "ALL"}:${kind?.name ?: "ALL"}"

    fun detailKey(postId: String): String = "post:$postId"

    fun normalizeOrigin(raw: String): String = raw.trimEnd('/')
}
