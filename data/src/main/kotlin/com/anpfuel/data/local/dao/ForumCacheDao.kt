package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.ForumCacheEntity

@Dao
interface ForumCacheDao {

    @Query("SELECT * FROM forum_cache WHERE origin = :origin AND `key` = :key LIMIT 1")
    suspend fun findByKey(origin: String, key: String): ForumCacheEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: ForumCacheEntity)

    @Query("DELETE FROM forum_cache WHERE origin = :origin AND `key` = :key")
    suspend fun deleteByKey(origin: String, key: String)

    @Query("DELETE FROM forum_cache WHERE fetched_at_millis < :notBeforeMillis")
    suspend fun deleteOlderThan(notBeforeMillis: Long)

    @Query(
        "DELETE FROM forum_cache WHERE origin = :origin AND kind = 'FEED' AND `key` NOT IN " +
            "(SELECT `key` FROM forum_cache WHERE origin = :origin AND kind = 'FEED' ORDER BY fetched_at_millis DESC LIMIT :maxScopes)",
    )
    suspend fun trimFeedScopes(origin: String, maxScopes: Int)

    @Query("SELECT COALESCE(SUM(payload_bytes), 0) FROM forum_cache WHERE origin = :origin")
    suspend fun totalBytes(origin: String): Long

    @Query(
        "DELETE FROM forum_cache WHERE origin = :origin AND `key` IN " +
            "(SELECT `key` FROM forum_cache WHERE origin = :origin ORDER BY fetched_at_millis ASC LIMIT :count)",
    )
    suspend fun deleteOldest(origin: String, count: Int)

    @Query("SELECT COUNT(*) FROM forum_cache WHERE origin = :origin")
    suspend fun count(origin: String): Int

    @Query("DELETE FROM forum_cache WHERE origin = :origin")
    suspend fun clearOrigin(origin: String)
}
