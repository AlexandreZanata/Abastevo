package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.CommunityFeedCacheEntity
import com.anpfuel.domain.community.CommunityFeedCacheRule

/**
 * Scope-keyed community snapshots. Reads are by primary `key` (or a
 * full-table hygiene sweep); no secondary index on purpose, so migrated
 * installs match the entity schema exactly.
 */
@Dao
interface CommunityFeedCacheDao {

    @Query("SELECT * FROM community_feed_cache WHERE `key` = :key LIMIT 1")
    suspend fun findByKey(key: String): CommunityFeedCacheEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: CommunityFeedCacheEntity)

    @Query("DELETE FROM community_feed_cache WHERE `key` = :key")
    suspend fun deleteByKey(key: String)

    @Query("DELETE FROM community_feed_cache WHERE fetched_at_millis < :notBeforeMillis")
    suspend fun deleteOlderThan(notBeforeMillis: Long)

    @Query(
        "DELETE FROM community_feed_cache WHERE `key` NOT IN " +
            "(SELECT `key` FROM community_feed_cache ORDER BY fetched_at_millis DESC, `key` ASC " +
            "LIMIT :maxEntries)",
    )
    suspend fun trimToNewest(maxEntries: Int = CommunityFeedCacheRule.MAX_ENTRIES)

    @Query("SELECT COUNT(*) FROM community_feed_cache")
    suspend fun count(): Int
}
