package com.anpfuel.data.local.importing

import androidx.room.withTransaction
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.dao.AveragePriceDao
import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.entity.AveragePriceEntity
import com.anpfuel.data.local.entity.StationPriceEntity
import com.anpfuel.data.local.catalog.MunicipalityAnpAliasMerger
import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.mapper.EntityDomainMapper
import com.anpfuel.data.parser.StationDetailSheetParser
import com.anpfuel.data.parser.WeeklySummarySheetParser
import com.anpfuel.data.parser.dto.AveragePriceRow
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.SurveyWeek
import java.io.File

data class ImportResult(
    val surveyWeekId: String,
    val rowsImported: Int,
)

/**
 * Streams parsed ANP rows into Room in batches with audit logging and FTS sync.
 */
class PriceTableBatchImporter(
    private val database: AnpFuelDatabase,
    private val surveyWeekDao: SurveyWeekDao,
    private val averagePriceDao: AveragePriceDao,
    private val stationPriceDao: StationPriceDao,
    private val importAuditLogger: ImportAuditLogger,
    private val catalogSeeder: MunicipalityCatalogSeeder,
    private val aliasMerger: MunicipalityAnpAliasMerger,
    private val summaryParser: WeeklySummarySheetParser = WeeklySummarySheetParser(),
    private val stationParser: StationDetailSheetParser = StationDetailSheetParser(),
) {

    suspend fun importWeeklySummary(file: File): ImportResult {
        importAuditLogger.append(
            action = ImportAuditAction.DOWNLOADED,
            detail = "local WEEKLY_SUMMARY file=${file.name}",
        )
        importAuditLogger.append(
            action = ImportAuditAction.IMPORTED,
            detail = "WEEKLY_SUMMARY import started file=${file.name}",
        )

        return try {
            catalogSeeder.seedIfEmpty()

            var firstRow: AveragePriceRow? = null
            var surveyWeekId: String? = null
            val pendingBatch = ArrayList<AveragePriceEntity>(BATCH_SIZE)
            val batches = mutableListOf<List<AveragePriceEntity>>()

            summaryParser.parse(file) { row ->
                if (firstRow == null) {
                    firstRow = row
                    val surveyWeek = SurveyWeek(row.startDate, row.endDate)
                    surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
                }

                pendingBatch += EntityDomainMapper.toAveragePriceEntity(row, requireNotNull(surveyWeekId))
                if (pendingBatch.size >= BATCH_SIZE) {
                    batches += pendingBatch.toList()
                    pendingBatch.clear()
                }
            }

            if (pendingBatch.isNotEmpty()) {
                batches += pendingBatch.toList()
            }

            val weekId = requireNotNull(surveyWeekId) { "Summary file contained no data rows" }
            val headerRow = requireNotNull(firstRow)
            surveyWeekDao.insert(
                EntityDomainMapper.toSurveyWeekEntity(
                    surveyWeek = SurveyWeek(headerRow.startDate, headerRow.endDate),
                    summaryImportedAt = System.currentTimeMillis(),
                ),
            )

            var totalRows = 0
            for (batch in batches) {
                totalRows += insertAveragePriceBatch(batch)
            }

            aliasMerger.mergeAliasesFromSurveyWeek(weekId)

            importAuditLogger.append(
                action = ImportAuditAction.IMPORTED,
                surveyWeekId = weekId,
                detail = "WEEKLY_SUMMARY imported rows=$totalRows",
            )
            ImportResult(surveyWeekId = weekId, rowsImported = totalRows)
        } catch (error: Exception) {
            importAuditLogger.append(
                action = ImportAuditAction.FAILED,
                detail = "WEEKLY_SUMMARY import failed: ${error.message}",
            )
            throw error
        }
    }

    suspend fun importStationDetail(file: File, surveyWeekId: String): ImportResult {
        importAuditLogger.append(
            action = ImportAuditAction.DOWNLOADED,
            surveyWeekId = surveyWeekId,
            detail = "local STATION_DETAIL file=${file.name}",
        )
        importAuditLogger.append(
            action = ImportAuditAction.IMPORTED,
            surveyWeekId = surveyWeekId,
            detail = "STATION_DETAIL import started file=${file.name}",
        )

        return try {
            val pendingBatch = ArrayList<StationPriceEntity>(BATCH_SIZE)
            val batches = mutableListOf<List<StationPriceEntity>>()

            stationParser.parse(file) { row ->
                pendingBatch += EntityDomainMapper.toStationPriceEntity(row, surveyWeekId)
                if (pendingBatch.size >= BATCH_SIZE) {
                    batches += pendingBatch.toList()
                    pendingBatch.clear()
                }
            }

            if (pendingBatch.isNotEmpty()) {
                batches += pendingBatch.toList()
            }

            var totalRows = 0
            for (batch in batches) {
                totalRows += insertStationPriceBatch(batch)
            }

            surveyWeekDao.updateStationImportedAt(
                id = surveyWeekId,
                stationImportedAt = System.currentTimeMillis(),
            )

            importAuditLogger.append(
                action = ImportAuditAction.IMPORTED,
                surveyWeekId = surveyWeekId,
                detail = "STATION_DETAIL imported rows=$totalRows",
            )
            ImportResult(surveyWeekId = surveyWeekId, rowsImported = totalRows)
        } catch (error: Exception) {
            importAuditLogger.append(
                action = ImportAuditAction.FAILED,
                surveyWeekId = surveyWeekId,
                detail = "STATION_DETAIL import failed: ${error.message}",
            )
            throw error
        }
    }

    private suspend fun insertAveragePriceBatch(batch: List<AveragePriceEntity>): Int {
        database.withTransaction {
            averagePriceDao.insertAll(batch)
        }
        return batch.size
    }

    private suspend fun insertStationPriceBatch(batch: List<StationPriceEntity>): Int {
        database.withTransaction {
            stationPriceDao.insertAll(batch)
        }
        return batch.size
    }

    companion object {
        const val BATCH_SIZE = 1000
    }
}
