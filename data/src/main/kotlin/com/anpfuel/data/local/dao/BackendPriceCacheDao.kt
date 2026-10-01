package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.BackendPriceCacheEntity

@Dao
interface BackendPriceCacheDao {

    @Query("SELECT * FROM backend_price_cache WHERE `key` = :key LIMIT 1")
    suspend fun findByKey(key: String): BackendPriceCacheEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(entity: BackendPriceCacheEntity)

    @Query("DELETE FROM backend_price_cache WHERE `key` = :key")
    suspend fun deleteByKey(key: String)

    @Query("DELETE FROM backend_price_cache WHERE expires_at_millis <= :nowMillis")
    suspend fun deleteExpired(nowMillis: Long)

    @Query("DELETE FROM backend_price_cache")
    suspend fun clear()
}
