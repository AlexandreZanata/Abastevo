package com.anpfuel.domain.repository

/**
 * Port for clearing imported ANP data from local storage (UC-008).
 */
interface CacheRepository {

    suspend fun clearAllImportedData()
}
