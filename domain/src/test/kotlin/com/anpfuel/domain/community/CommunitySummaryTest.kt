package com.anpfuel.domain.community

import com.anpfuel.domain.model.BackendCommunitySection
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CommunitySummaryTest {
    private val now=Instant.parse("2026-10-14T12:00:00Z")
    private fun price()=CommunitySummaryPrice(FuelProduct.ETHANOL,4500,4000,5000,65,now,now.plusSeconds(3600))
    @Test fun `population bounds mean range dates unique products and units remain exact`() {
        val valid=price();assertEquals("L",valid.unit);assertEquals("M3",valid.copy(fuel=FuelProduct.CNG).unit);assertEquals("KG_13",valid.copy(fuel=FuelProduct.LPG_P13).unit)
        listOf<()->Unit>({valid.copy(minimumMilliBrl=0)},{valid.copy(maximumMilliBrl=3999)},{valid.copy(maximumMilliBrl=1000001)},{valid.copy(averageMilliBrl=5001)},{valid.copy(stationCount=0)},{valid.copy(expiresAt=now)}).forEach {assertThrows(IllegalArgumentException::class.java){it()}}
        val city=FeedCity("5103403",BrazilianState.MATO_GROSSO,"Synthetic city")
        assertEquals(65,CityCommunitySummary(city,listOf(valid),now).items.single().stationCount)
        assertThrows(IllegalArgumentException::class.java){CityCommunitySummary(city,listOf(valid,valid),now)}
        assertThrows(IllegalArgumentException::class.java){CityCommunitySummary(city,listOf(valid),now.minusSeconds(1))}
        assertThrows(IllegalArgumentException::class.java){CityCommunitySummary(city,listOf(valid),now.plusSeconds(3600))}
    }
    @Test fun `typed source refuses false provenance invalid values and unavailable fresh offers`() {
        val c=BackendCommunitySection("COMMUNITY","AVAILABLE",4500,"BRL","LOW","FRESH",1,0,null,now,now.plusSeconds(3600),"consensus-community-v2",1)
        assertEquals(4500L,c.amountMilliBrl)
        listOf<()->Unit>({c.copy(source="ANP")},{c.copy(currency="USD")},{c.copy(availability="FAKE")},{c.copy(amountMilliBrl=0)},{c.copy(confidence="VERIFIED")},{c.copy(freshness="FOREVER")},{c.copy(supporters=-1)},{c.copy(confirmations=-1)},{c.copy(projectionVersion=0)},{c.copy(algorithmVersion=" ")},{c.copy(representativeObservationId="private-owner")},{c.copy(expiresAt=now)},{c.copy(amountMilliBrl=null)},{c.copy(lastReceivedAt=null)},{c.copy(expiresAt=null)}).forEach {assertThrows(IllegalArgumentException::class.java){it()}}
        assertNull(c.copy(availability="UNKNOWN",amountMilliBrl=null,lastReceivedAt=null,expiresAt=null,confidence="",freshness="STALE").amountMilliBrl)
        assertEquals("DISPUTED",c.copy(availability="DISPUTED").availability)
    }
}
