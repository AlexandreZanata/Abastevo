package com.anpfuel.data.local.importing

import android.content.Context
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.catalog.MunicipalityAnpAliasMerger
import com.anpfuel.data.local.catalog.MunicipalityCatalogResolver
import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.repository.MunicipalityCatalogRepositoryImpl
import com.anpfuel.data.local.fts.MunicipalityFtsIndexer
import com.anpfuel.data.repository.MunicipalitySearchRepositoryImpl

internal object ImportTestCatalogSupport {

    fun createCatalogSeeder(
        context: Context,
        database: AnpFuelDatabase,
    ): MunicipalityCatalogSeeder =
        MunicipalityCatalogSeeder(
            context = context,
            database = database,
            municipalityCatalogDao = database.municipalityCatalogDao(),
            ftsIndexer = MunicipalityFtsIndexer(database.municipalityFtsDao()),
        )

    fun createBatchImporter(
        context: Context,
        database: AnpFuelDatabase,
    ): PriceTableBatchImporter {
        val ftsIndexer = MunicipalityFtsIndexer(database.municipalityFtsDao())
        val catalogSeeder = createCatalogSeeder(context, database)
        val aliasMerger = MunicipalityAnpAliasMerger(
            averagePriceDao = database.averagePriceDao(),
            municipalityCatalogDao = database.municipalityCatalogDao(),
            catalogResolver = MunicipalityCatalogResolver(database.municipalityCatalogDao()),
            ftsIndexer = ftsIndexer,
        )
        return PriceTableBatchImporter(
            database = database,
            surveyWeekDao = database.surveyWeekDao(),
            averagePriceDao = database.averagePriceDao(),
            stationPriceDao = database.stationPriceDao(),
            importAuditLogger = ImportAuditLogger(database.importAuditLogDao()),
            catalogSeeder = catalogSeeder,
            aliasMerger = aliasMerger,
        )
    }

    fun createCatalogRepository(
        context: Context,
        database: AnpFuelDatabase,
    ): MunicipalityCatalogRepositoryImpl =
        MunicipalityCatalogRepositoryImpl(
            municipalityCatalogDao = database.municipalityCatalogDao(),
            averagePriceDao = database.averagePriceDao(),
            catalogSeeder = createCatalogSeeder(context, database),
        )

    fun createSearchRepository(
        context: Context,
        database: AnpFuelDatabase,
    ): MunicipalitySearchRepositoryImpl =
        MunicipalitySearchRepositoryImpl(
            municipalityFtsDao = database.municipalityFtsDao(),
            municipalityCatalogDao = database.municipalityCatalogDao(),
            catalogSeeder = createCatalogSeeder(context, database),
        )
}
