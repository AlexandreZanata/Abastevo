package com.anpfuel.domain.repository

import com.anpfuel.domain.model.StorageUsage

/**
 * Port for querying imported data storage statistics (UC-008).
 */
interface StorageStatsRepository {

    suspend fun getStorageUsage(): StorageUsage
}
