package com.anpfuel.application.usecase.quality

import com.anpfuel.domain.quality.*
import com.anpfuel.domain.repository.QualityGateway
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ReadQualityTest {
    private val station = "eeeeeeee-5555-4555-8555-000000000001"
    private val revision = "eeeeeeee-6666-4666-8666-000000000001"
    private val gateway = mockk<QualityGateway>()

    @Test fun `guest assay read pins the exact summary revision`() = runTest {
        val lease = QualityLease("pmqc", "pmqc-assay-v1", "2026-09", revision, true, false)
        val summary = QualitySummary(station, lease)
        coEvery { gateway.summary(station, "ETHANOL", null) } returns summary
        coEvery { gateway.assays(station, "ETHANOL", revision, null) } returns QualityPage(emptyList(), null, lease)
        val result = ReadQuality(gateway).invoke(QualitySelection(station,"ETHANOL"))
        assertEquals(revision, result.assays.lease?.revisionId)
        coVerify(exactly=1) { gateway.assays(station,"ETHANOL",revision,null) }
    }

    @Test fun `unavailable source never fabricates a sample denominator`() = runTest {
        val lease = QualityLease("pmqc","pmqc-assay-v1","",null,false,true)
        coEvery { gateway.summary(station,"ETHANOL",null) } returns QualitySummary(station,lease)
        val result=ReadQuality(gateway).invoke(QualitySelection(station,"ETHANOL"))
        assertTrue(result.assays.items.isEmpty())
        assertFalse(result.summary.dimensionsAvailable)
        coVerify(exactly=0) { gateway.assays(any(),any(),any(),any()) }
    }

    @Test fun `mixed revision response fails closed`() = runTest {
        val lease = QualityLease("pmqc","pmqc-assay-v1","2026-09",revision,true,false)
        coEvery { gateway.summary(station,"ETHANOL",null) } returns QualitySummary(station,lease)
        coEvery { gateway.assays(station,"ETHANOL",revision,null) } returns QualityPage(emptyList(),null,lease.copy(revisionId=station))
        assertThrows(IllegalStateException::class.java) { kotlinx.coroutines.runBlocking { ReadQuality(gateway).invoke(QualitySelection(station,"ETHANOL")) } }
    }
}
