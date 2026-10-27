package com.anpfuel.domain.premium

import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class LegacyOdometerReviewTest {
    private val fill = RefuelDraft("11111111-1111-4111-8111-111111111111","22222222-2222-4222-8222-222222222222",
        Instant.parse("2026-03-01T12:00:00Z"),"America/Cuiaba",PremiumFuel.ETHANOL,40000,26000,null,10000,LedgerFill.FULL,null,"original")
    @Test fun `classification never guesses a legacy unit from its magnitude`() {
        assertEquals(10000L,LegacyOdometerReview.preview(fill,OdometerReviewChoice.AS_METRES).correctedMetres)
        assertEquals(10000000L,LegacyOdometerReview.preview(fill,OdometerReviewChoice.AS_KILOMETRES).correctedMetres)
        assertEquals(10000L,fill.odometerMetres)
        assertEquals(26000L,fill.totalCentavos)
        assertEquals(40000L,fill.quantityMilli)
    }
    @Test fun `canonical fingerprint changes on any late financial edit`() {
        val a=LegacyOdometerReview.fingerprint(fill)
        assertEquals(64,a.length)
        assertEquals(a,LegacyOdometerReview.fingerprint(fill.copy()))
        assertNotEquals(a,LegacyOdometerReview.fingerprint(fill.copy(totalCentavos=25999)))
        assertNotEquals(a,LegacyOdometerReview.fingerprint(fill.copy(note="changed")))
    }
    @Test fun `review reuses actual dataset bounds and refuses missing or excessive values`() {
        assertThrows(PremiumValidationException::class.java) { LegacyOdometerReview.preview(fill.copy(odometerMetres=null),OdometerReviewChoice.AS_METRES) }
        assertThrows(PremiumValidationException::class.java) { LegacyOdometerReview.preview(fill.copy(odometerMetres=PremiumBounds.MAX_DISTANCE_METRES),OdometerReviewChoice.AS_KILOMETRES) }
        val charge=ChargeDraft(fill.id,fill.vehicleId,fill.occurredAt,"UTC",1000,200,null,null,1000000000L,null,null,null,null)
        assertThrows(PremiumValidationException::class.java) { LegacyOdometerReview.preview(charge,OdometerReviewChoice.AS_KILOMETRES) }
    }
}
