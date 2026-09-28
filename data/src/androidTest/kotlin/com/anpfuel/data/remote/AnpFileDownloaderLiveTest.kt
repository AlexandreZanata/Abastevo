package com.anpfuel.data.remote

import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.domain.valueobject.PriceTableType
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertTrue
import org.junit.Ignore
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AnpFileDownloaderLiveTest {

    @Test
    @Ignore("Requires network access to gov.br — enable manually for live POC validation")
    fun downloadsLatestSummaryAndStationFiles() = runBlocking {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        val scraper = AnpListingScraper(OkHttpClientFactory.create())
        val downloader = AnpFileDownloader(context, OkHttpClientFactory.create())

        val discovered = scraper.discoverPriceTables()
        val latestWeekTables = discovered
            .groupBy { it.surveyWeek }
            .maxByOrNull { it.key.endDate }
            ?.value
            .orEmpty()

        val summary = latestWeekTables.first { it.tableType == PriceTableType.WEEKLY_SUMMARY }
        val station = latestWeekTables.first { it.tableType == PriceTableType.STATION_DETAIL }

        val summaryDownload = downloader.download(summary)
        val stationDownload = downloader.download(station)

        assertTrue(summaryDownload.sizeBytes > 0)
        assertTrue(stationDownload.sizeBytes > 0)
        assertTrue(summaryDownload.file.name.endsWith(".xlsx"))
        assertTrue(stationDownload.file.name.endsWith(".xlsx"))
    }
}
