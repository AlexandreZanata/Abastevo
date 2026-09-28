package com.anpfuel.data.local.dao

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import com.anpfuel.data.local.entity.MunicipalityCatalogEntity

@Dao
interface MunicipalityCatalogDao {

    @Query("SELECT COUNT(*) FROM municipality_catalog")
    suspend fun count(): Int

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertAll(entries: List<MunicipalityCatalogEntity>)

    @Query(
        """
        SELECT * FROM municipality_catalog
        WHERE state = :state AND normalized_name = :normalizedName
        LIMIT 1
        """,
    )
    suspend fun findByStateAndNormalizedName(
        state: String,
        normalizedName: String,
    ): MunicipalityCatalogEntity?

    @Query(
        """
        SELECT * FROM municipality_catalog
        WHERE state = :state AND municipality = :municipality
        LIMIT 1
        """,
    )
    suspend fun findByStateAndMunicipality(
        state: String,
        municipality: String,
    ): MunicipalityCatalogEntity?

    @Query(
        """
        SELECT * FROM municipality_catalog
        WHERE state || char(31) || municipality IN (:compositeKeys)
        """,
    )
    suspend fun findByStateMunicipalityKeys(compositeKeys: List<String>): List<MunicipalityCatalogEntity>

    @Query("UPDATE municipality_catalog SET anp_alias = :alias WHERE id = :id")
    suspend fun updateAnpAlias(id: String, alias: String)

    @Query("SELECT * FROM municipality_catalog")
    suspend fun findAll(): List<MunicipalityCatalogEntity>

    @Query(
        """
        SELECT DISTINCT state FROM municipality_catalog
        ORDER BY state ASC
        """,
    )
    suspend fun findDistinctStates(): List<String>

    @Query(
        """
        SELECT * FROM municipality_catalog
        WHERE state = :state
        ORDER BY municipality COLLATE NOCASE ASC
        """,
    )
    suspend fun findByState(state: String): List<MunicipalityCatalogEntity>
}
