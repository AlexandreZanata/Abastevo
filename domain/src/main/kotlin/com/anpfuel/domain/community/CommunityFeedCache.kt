package com.anpfuel.domain.community

import com.anpfuel.domain.valueobject.FuelProduct

/**
 * Scoped private snapshot of anonymous community responses.
 *
 * The backend answers `Cache-Control: no-store` with no ETag/304 support,
 * so snapshots are never HTTP revalidation: each entry is a private copy of
 * one validated first-page/summary response, keyed by its full scope
 * (origin, city, fuel, sort). Original dates are preserved; expired rows are
 * filtered at read time and never presented as current. Errors, credentials
 * and private data are never stored. Deeper pages (cursor) stay
 * network-only so pagination keeps its server cursor isolation.
 */
data class CommunityFeedScope(
    val origin: String,
    val city: FeedCity,
    val fuel: FuelProduct,
    val sort: FeedSort,
)

data class CommunitySummaryScope(
    val origin: String,
    val city: FeedCity,
)

/**
 * Local-first reads used to paint immediately before a network refresh.
 * Callers pass only the user-visible scope (city, fuel, sort); the store
 * binds its own configured origin so snapshots can never leak across
 * environments.
 */
interface CommunityFeedLocalSnapshot {
    suspend fun snapshotFeed(city: FeedCity, fuel: FuelProduct, sort: FeedSort): CommunityFeedPage?
    suspend fun snapshotSummary(city: FeedCity): CityCommunitySummary?
}

object CommunityFeedCacheRule {
    const val KIND_FEED = "FEED"
    const val KIND_SUMMARY = "SUMMARY"

    /** Payload ceiling mirrors the 256 KiB transport bound. */
    const val MAX_PAYLOAD_CHARS = 256 * 1024

    /** Scope-cap: two cities × full fuel/sort matrix plus summaries, with headroom. */
    const val MAX_ENTRIES = 64

    /** Storage hygiene only; validity always comes from the payload dates. */
    const val MAX_STORAGE_AGE_MILLIS = 7L * 24 * 60 * 60 * 1000

    fun feedKey(scope: CommunityFeedScope): String =
        listOf(
            KIND_FEED,
            normalizeOrigin(scope.origin),
            scope.city.state.abbreviation,
            scope.city.code,
            scope.fuel.name,
            scope.sort.name,
        ).joinToString("|")

    fun summaryKey(scope: CommunitySummaryScope): String =
        listOf(
            KIND_SUMMARY,
            normalizeOrigin(scope.origin),
            scope.city.state.abbreviation,
            scope.city.code,
        ).joinToString("|")

    fun normalizeOrigin(origin: String): String = origin.trim().trimEnd('/').lowercase()
}
