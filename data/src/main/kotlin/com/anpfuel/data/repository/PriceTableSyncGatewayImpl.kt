package com.anpfuel.data.repository

import com.anpfuel.data.local.importing.PriceTableBatchImporter
import com.anpfuel.data.remote.AnpFileDownloader
import com.anpfuel.data.remote.AnpListingScraper
import com.anpfuel.domain.event.PriceTableImported
import com.anpfuel.domain.model.PriceTable
import com.anpfuel.domain.repository.PriceTableSyncGateway
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.PriceTableType
import java.io.File
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class PriceTableSyncGatewayImpl @Inject constructor(
    private val listingScraper: AnpListingScraper,
    private val fileDownloader: AnpFileDownloader,
    private val batchImporter: PriceTableBatchImporter,
) : PriceTableSyncGateway {

    override suspend fun discoverPriceTables(): List<PriceTable> =
        listingScraper.discoverPriceTables()

    override suspend fun discoverSurveyWeekCatalog() =
        listingScraper.discoverSurveyWeekCatalog()

    override suspend fun downloadPriceTable(priceTable: PriceTable): PriceTable {
        val downloadResult = fileDownloader.download(priceTable)
        return priceTable.copy(checksum = downloadResult.sha256)
    }

    override suspend fun importWeeklySummary(priceTable: PriceTable): PriceTableImported.Payload {
        val result = batchImporter.importWeeklySummary(resolveLocalFile(priceTable))
        return PriceTableImported.Payload(
            surveyWeekId = DomainId.from(result.surveyWeekId),
            tableType = PriceTableType.WEEKLY_SUMMARY,
            rowCount = result.rowsImported,
        )
    }

    override suspend fun importStationDetail(priceTable: PriceTable): PriceTableImported.Payload {
        val surveyWeekId = DomainId.forSurveyWeek(priceTable.surveyWeek).value
        val result = batchImporter.importStationDetail(
            file = resolveLocalFile(priceTable),
            surveyWeekId = surveyWeekId,
        )
        return PriceTableImported.Payload(
            surveyWeekId = DomainId.from(result.surveyWeekId),
            tableType = PriceTableType.STATION_DETAIL,
            rowCount = result.rowsImported,
        )
    }

    private fun resolveLocalFile(priceTable: PriceTable): File {
        val file = fileDownloader.resolveDownloadedFile(priceTable.sourceUrl)
        require(file.exists() && file.length() > 0L) {
            "Downloaded file not found for ${priceTable.sourceUrl}"
        }
        return file
    }
}
