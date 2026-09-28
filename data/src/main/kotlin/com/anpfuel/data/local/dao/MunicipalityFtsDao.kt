package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Query
import com.anpfuel.data.local.model.MunicipalityFtsRow

@Dao
interface MunicipalityFtsDao {

    @Query(
        """
        SELECT DISTINCT municipality, state
        FROM municipality_fts
        WHERE municipality_fts MATCH :matchQuery
        ORDER BY municipality COLLATE NOCASE ASC
        LIMIT :limit
        """,
    )
    suspend fun search(matchQuery: String, limit: Int): List<MunicipalityFtsRow>

    @Query("INSERT INTO municipality_fts(municipality_fts) VALUES('rebuild')")
    suspend fun rebuildIndex()
}
