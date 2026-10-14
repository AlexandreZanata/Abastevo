package com.anpfuel.domain.community

import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.model.PhotoContributionContext
import com.anpfuel.domain.premium.PremiumBounds
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.profile.StationProfile
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CommunityFirstContractValidationTest {
    private val stationId="123e4567-e89b-12d3-a456-426614174000"
    private val now=Instant.parse("2026-10-14T12:00:00Z")
    private fun item()=CommunityFeedItem(stationId,"Synthetic nominal station",FuelProduct.ETHANOL,5999,now,now.plusSeconds(86400),2,1,"MEDIUM",1)

    @Test fun `feed refuses invalid identity money lifetime counts confidence and ratings`() {
        val valid=item()
        val invalid=listOf<()->Unit>(
            {valid.copy(stationId="station-name")}, {valid.copy(stationName=" ")},
            {valid.copy(amountMilliBrl=0)}, {valid.copy(amountMilliBrl=1000001)},
            {valid.copy(expiresAt=now)}, {valid.copy(expiresAt=now.minusSeconds(1))},
            {valid.copy(supporters=-1)}, {valid.copy(confirmations=-1)},
            {valid.copy(version=0)}, {valid.copy(confidence="VERIFIED")},
            {valid.copy(ratingsCount=-1)}, {valid.copy(ratingsCount=1)},
            {valid.copy(ratingsAvg=3.0)}, {valid.copy(ratingsCount=1,ratingsAvg=0.0)},
            {valid.copy(ratingsCount=1,ratingsAvg=5.1)}, {valid.copy(ratingsCount=1,ratingsAvg=Double.NaN)},
        )
        invalid.forEach {reject->assertThrows(IllegalArgumentException::class.java){reject()}}
        assertEquals(5999L,valid.amountMilliBrl)
        assertEquals("L",valid.unit)
        assertEquals("M3",valid.copy(fuel=FuelProduct.CNG).unit)
        assertEquals("KG_13",valid.copy(fuel=FuelProduct.LPG_P13).unit)
        assertEquals(4.5,valid.copy(ratingsCount=2,ratingsAvg=4.5).ratingsAvg)
        assertEquals(stationId,valid.stationId)
        assertEquals(now,valid.updatedAt)
        assertEquals(now.plusSeconds(86400),valid.expiresAt)
    }

    @Test fun `manual city needs canonical code and name while filter edits preserve the original selection`() {
        assertThrows(IllegalArgumentException::class.java){FeedCity("city-name",BrazilianState.SAO_PAULO,"Synthetic city")}
        assertThrows(IllegalArgumentException::class.java){FeedCity("3550308",BrazilianState.SAO_PAULO," ")}
        val city=FeedCity("3550308",BrazilianState.SAO_PAULO,"Synthetic city")
        val query=FeedQuery(city,FuelProduct.ETHANOL)
        val changed=query.copy(fuel=FuelProduct.CNG,sort=FeedSort.CHEAPEST)
        assertEquals(city,changed.city)
        assertEquals("3550308",changed.city.code)
        assertEquals("Synthetic city",changed.city.name)
        assertEquals(BrazilianState.SAO_PAULO,changed.city.state)
        assertEquals(FeedSort.RECENT,query.sort)
        assertEquals(FuelProduct.ETHANOL,query.fuel)
    }

    @Test fun `photo context refuses missing identity deadline and unsafe owner origin envelopes`() {
        val context=PhotoContributionContext(stationId,86400000,"synthetic-owner","https://example.invalid")
        assertEquals(ContributionScope("synthetic-owner","https://example.invalid"),context.scope)
        assertThrows(DomainException::class.java){context.copy(captureId="invalid")}
        assertThrows(DomainException::class.java){context.copy(expiresAtMillis=0)}
        listOf("", "x".repeat(257), "synthetic\u0000owner").forEach {owner->
            assertThrows(DomainException::class.java){context.copy(ownerScope=owner)}
        }
        listOf("http://example.invalid","https://example.invalid/path","https://example.invalid?owner=other").forEach {origin->
            assertThrows(DomainException::class.java){context.copy(origin=origin)}
        }
    }

    @Test fun `public station profile cannot acquire private identity GPS or a badge from nominal fields`() {
        val public=StationProfile.projectBusiness(mapOf("description" to "Synthetic business","cpf" to "private","gps" to "private","password" to "private"))
        val profile=StationProfile(stationId,"Synthetic station",public)
        assertEquals(mapOf("description" to "Synthetic business"),profile.business)
        assertFalse(profile.hasBadge)
        assertEquals("",profile.operatorSource)
    }

    @Test fun `retained financial inputs enforce exact ceilings unicode note length and rational unknowns`() {
        listOf(0L,PremiumBounds.MAX_QUANTITY_MILLI+1).forEach {assertThrows(PremiumValidationException::class.java){PremiumBounds.checkQuantityMilli(it)}}
        listOf(-1L,PremiumBounds.MAX_TOTAL_CENTAVOS+1).forEach {assertThrows(PremiumValidationException::class.java){PremiumBounds.checkTotalCentavos(it)}}
        listOf(-1L,PremiumBounds.MAX_DISTANCE_METRES+1).forEach {assertThrows(PremiumValidationException::class.java){PremiumBounds.checkDistanceMetres(it)}}
        PremiumBounds.checkQuantityMilli(1);PremiumBounds.checkQuantityMilli(PremiumBounds.MAX_QUANTITY_MILLI)
        PremiumBounds.checkTotalCentavos(0);PremiumBounds.checkTotalCentavos(PremiumBounds.MAX_TOTAL_CENTAVOS)
        PremiumBounds.checkDistanceMetres(0);PremiumBounds.checkDistanceMetres(PremiumBounds.MAX_DISTANCE_METRES)
        PremiumBounds.checkNote(null);PremiumBounds.checkNote("a".repeat(280));PremiumBounds.checkNote("🚗".repeat(280))
        assertThrows(PremiumValidationException::class.java){PremiumBounds.checkNote("a".repeat(281))}
        assertThrows(PremiumValidationException::class.java){PremiumBounds.checkNote("🚗".repeat(281))}
        assertEquals("",PremiumBounds.rational(10,0));assertEquals("",PremiumBounds.rational(10,-1))
        assertEquals("0",PremiumBounds.rational(0,1));assertEquals("-5/2",PremiumBounds.rational(-10,4))
    }

    @Test fun `nominal station identity refuses Unicode digits that are not UUID hexadecimal`() {
        val unicode=stationId.map {if(it in '0'..'9') (it.code-'0'.code+0x0660).toChar() else it}.joinToString("")
        assertThrows(DomainException::class.java){ServerStation.create(unicode,"Synthetic station",StationLocationQuality.UNKNOWN,null,null,null,"3550308","SP",null)}
        assertThrows(DomainException::class.java){ContributionDraft.create("synthetic-command",unicode,FuelProduct.ETHANOL,5999,"BRL","L","STANDARD",1)}
    }
    @Test fun `nominal selection never bypasses draft money owner intent and capture validation`() {
        fun draft(amount: Long = 5999, currency: String = "BRL", unit: String = "L",
                  condition: String = "STANDARD", capturedAt: Long = 1,
                  photo: String? = null, supersedes: String? = null) = ContributionDraft.create(
            "synthetic-command", stationId, FuelProduct.ETHANOL, amount, currency, unit,
            condition, capturedAt, photo, supersedes,
        )
        listOf<() -> Unit>(
            { draft(amount = 0) }, { draft(amount = 1000001) }, { draft(currency = "USD") },
            { draft(unit = " ") }, { draft(condition = " ") }, { draft(capturedAt = 0) },
            { draft(photo = " ") }, { draft(supersedes = " ") },
        ).forEach { reject -> assertThrows(DomainException::class.java) { reject() } }
        assertNull(draft().photoId)
        assertNull(draft().supersedesObservationId)
        assertEquals("synthetic-photo", draft(photo = "synthetic-photo").photoId)
        assertEquals(stationId, draft(supersedes = stationId).supersedesObservationId)
    }

}
