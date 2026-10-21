package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.CommunityFeedCacheDao
import com.anpfuel.data.local.entity.CommunityFeedCacheEntity
import com.anpfuel.data.remote.CityCommunityFeedHttpClient
import com.anpfuel.data.remote.CommunityFeedCacheCodec
import com.anpfuel.domain.community.CityCommunityFeedReader
import com.anpfuel.domain.community.CityCommunitySummary
import com.anpfuel.domain.community.CityCommunitySummaryReader
import com.anpfuel.domain.community.CommunityFeedCacheRule
import com.anpfuel.domain.community.CommunityFeedLocalSnapshot
import com.anpfuel.domain.community.CommunityFeedPage
import com.anpfuel.domain.community.CommunityFeedScope
import com.anpfuel.domain.community.CommunitySummaryScope
import com.anpfuel.domain.community.FeedCity
import com.anpfuel.domain.community.FeedQuery
import com.anpfuel.domain.community.FeedSort
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.CancellationException

/**
 * Write-through private snapshot for community first-pages and summaries.
 *
 * Network behavior is unchanged except for a best-effort snapshot write on
 * each validated success: cursor pages never touch the cache, failures never
 * overwrite it, and cache problems never fail a read. Snapshot reads filter
 * expired rows against the call clock and fail closed (null) on any drift,
 * so stale data can only be shown explicitly aged by the caller, never as
 * current. No conditional requests are issued: the backend has no
 * ETag/304 support and `refresh_after_seconds` stays advisory.
 */
@Singleton
class CommunityFeedCacheStore @Inject constructor(
    private val networkFeed: CityCommunityFeedReader,
    private val networkSummary: CityCommunitySummaryReader,
    private val dao: CommunityFeedCacheDao,
    private val origin: String,
    private val nowMillis: () -> Long = { System.currentTimeMillis() },
) : CityCommunityFeedReader, CityCommunitySummaryReader, CommunityFeedLocalSnapshot {

    override suspend fun read(query: FeedQuery, cursor: String?): CommunityFeedPage {
        // Deeper pages keep their live HMAC cursors and stay network-only.
        if (cursor != null) return networkFeed.read(query, cursor)
        val page = networkFeed.read(query, null)
        bestEffort { storeFeed(CommunityFeedScope(origin, query.city, query.fuel, query.sort), query, page) }
        return page
    }

    override suspend fun readSummary(city: FeedCity): CityCommunitySummary {
        val summary = networkSummary.readSummary(city)
        bestEffort { storeSummary(CommunitySummaryScope(origin, city), summary) }
        return summary
    }

    override suspend fun snapshotFeed(city: FeedCity, fuel: FuelProduct, sort: FeedSort): CommunityFeedPage? {
        val scope = CommunityFeedScope(origin, city, fuel, sort)
        val entity = try {
            dao.findByKey(CommunityFeedCacheRule.feedKey(scope))
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            return null
        } ?: return null
        if (entity.payloadJson.length > CommunityFeedCacheRule.MAX_PAYLOAD_CHARS) return null
        val decoded = try {
            CityCommunityFeedHttpClient.decode(
                entity.payloadJson,
                FeedQuery(scope.city, scope.fuel, scope.sort),
            )
        } catch (_: IllegalArgumentException) {
            return null
        }
        val now = Instant.ofEpochMilli(nowMillis())
        return decoded.copy(items = decoded.items.filter { it.expiresAt > now })
    }

    override suspend fun snapshotSummary(city: FeedCity): CityCommunitySummary? {
        val scope = CommunitySummaryScope(origin, city)
        val entity = try {
            dao.findByKey(CommunityFeedCacheRule.summaryKey(scope))
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            return null
        } ?: return null
        if (entity.payloadJson.length > CommunityFeedCacheRule.MAX_PAYLOAD_CHARS) return null
        val decoded = try {
            CityCommunityFeedHttpClient.decodeSummary(entity.payloadJson, city)
        } catch (_: IllegalArgumentException) {
            return null
        }
        val now = Instant.ofEpochMilli(nowMillis())
        return CityCommunitySummary(
            city = decoded.city,
            items = decoded.items.filter { it.expiresAt > now },
            generatedAt = decoded.generatedAt,
        )
    }

    private suspend fun storeFeed(scope: CommunityFeedScope, query: FeedQuery, page: CommunityFeedPage) {
        val payload = CommunityFeedCacheCodec.encodeFeed(query, page)
        if (payload.length > CommunityFeedCacheRule.MAX_PAYLOAD_CHARS) return
        dao.upsert(
            CommunityFeedCacheEntity(
                key = CommunityFeedCacheRule.feedKey(scope),
                kind = CommunityFeedCacheRule.KIND_FEED,
                origin = CommunityFeedCacheRule.normalizeOrigin(scope.origin),
                state = scope.city.state.abbreviation,
                municipalityCode = scope.city.code,
                fuel = scope.fuel.name,
                sort = scope.sort.name,
                payloadJson = payload,
                generatedAtMillis = page.generatedAt.toEpochMilli(),
                fetchedAtMillis = nowMillis(),
            ),
        )
        prune()
    }

    private suspend fun storeSummary(scope: CommunitySummaryScope, summary: CityCommunitySummary) {
        val payload = CommunityFeedCacheCodec.encodeSummary(summary)
        if (payload.length > CommunityFeedCacheRule.MAX_PAYLOAD_CHARS) return
        dao.upsert(
            CommunityFeedCacheEntity(
                key = CommunityFeedCacheRule.summaryKey(scope),
                kind = CommunityFeedCacheRule.KIND_SUMMARY,
                origin = CommunityFeedCacheRule.normalizeOrigin(scope.origin),
                state = scope.city.state.abbreviation,
                municipalityCode = scope.city.code,
                fuel = null,
                sort = null,
                payloadJson = payload,
                generatedAtMillis = summary.generatedAt.toEpochMilli(),
                fetchedAtMillis = nowMillis(),
            ),
        )
        prune()
    }

    private suspend fun prune() {
        dao.deleteOlderThan(nowMillis() - CommunityFeedCacheRule.MAX_STORAGE_AGE_MILLIS)
        dao.trimToNewest()
    }

    private suspend fun bestEffort(block: suspend () -> Unit) {
        try {
            block()
        } catch (error: CancellationException) {
            throw error
        } catch (_: Exception) {
            // Snapshot writes/prunes must never fail a validated network read.
        }
    }

}
