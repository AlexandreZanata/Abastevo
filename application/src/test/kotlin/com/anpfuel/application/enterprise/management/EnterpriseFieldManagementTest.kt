package com.anpfuel.application.enterprise.management

import com.anpfuel.domain.enterprise.maintenance.ComplianceState
import com.anpfuel.domain.enterprise.stock.StockError
import com.anpfuel.domain.enterprise.stock.StockPool
import java.time.Instant
import java.time.LocalDate
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EnterpriseFieldManagementTest {
    private fun uuid(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')

    @Test
    fun `summarizes compliance expiry respecting licence toggle without financial data exposure`() {
        val today = LocalDate.of(2026, 11, 18)
        val book = FieldManagementBook(
            membershipId = uuid(1),
            enrollmentId = uuid(2),
            complianceNotices = listOf(
                FieldComplianceNotice(uuid(10), "ASSET", "CRLV", today.plusDays(10), ComplianceState.DUE_SOON, 30),
                FieldComplianceNotice(uuid(11), "EMPLOYEE", "CNH", today.minusDays(2), ComplianceState.EXPIRED, 30),
            ),
            maintenanceRequests = listOf(
                EnterpriseFieldManagementService.prepareMaintenanceRequest(uuid(20), uuid(30), "  Freio rangendo  ", 120_000L, Instant.parse("2026-11-18T10:00:00Z")),
            ),
            internalDispenseQueueCount = 2,
            updatedAt = Instant.parse("2026-11-18T10:00:00Z"),
        )

        val disabledLicence = EnterpriseFieldManagementService.summarizeForField(book, licenceControlEnabled = false, evalDay = today)
        assertEquals(0, disabledLicence.expiredComplianceCount)
        assertEquals(1, disabledLicence.dueSoonComplianceCount)
        assertEquals(10, disabledLicence.nearestDaysRemaining)
        assertFalse(disabledLicence.hasOrgFinancialDataExposed)

        val enabledLicence = EnterpriseFieldManagementService.summarizeForField(book, licenceControlEnabled = true, evalDay = today)
        assertEquals(1, enabledLicence.expiredComplianceCount)
        assertEquals(1, enabledLicence.dueSoonComplianceCount)
        assertEquals(-2, enabledLicence.nearestDaysRemaining)
        assertEquals(1, enabledLicence.openMaintenanceRequestCount)
        assertEquals(2, enabledLicence.offlineInternalDispenseQueued)
    }

    @Test
    fun `canonical book roundtrip enforces binding and redacts stock financial values for drivers`() {
        val book = FieldManagementBook(
            membershipId = uuid(1),
            enrollmentId = uuid(2),
            complianceNotices = listOf(FieldComplianceNotice(uuid(3), "ASSET", "INSURANCE", LocalDate.of(2026, 12, 1), ComplianceState.VALID, 15)),
            maintenanceRequests = listOf(FieldMaintenanceRequestDraft(uuid(4), uuid(5), "Lanterna", null, Instant.parse("2026-11-18T08:00:00Z"))),
            internalDispenseQueueCount = 1,
            updatedAt = Instant.parse("2026-11-18T08:05:00Z"),
        )
        val decoded = FieldManagementBook.decode(book.canonicalBytes).requireBinding(uuid(1), uuid(2))
        assertEquals(book, decoded)
        assertThrows(IllegalArgumentException::class.java) { decoded.requireBinding(uuid(9), uuid(2)) }

        val pool = StockPool("DIESEL_S10", quantityMl = 100_000L, valueCentavos = 60_000L)
        val (draft, outcome) = EnterpriseFieldManagementService.prepareInternalDispense(pool, uuid(6), "Tanque Sede", uuid(5), "DIESEL_S10", 40_000L)
        assertEquals("ACCEPTED", outcome.status)
        assertEquals(60_000L, outcome.afterQuantityMl)
        assertEquals(0L, outcome.recognizedExpenseCentavos)
        assertEquals(0L, outcome.afterValueCentavos)
        assertTrue(draft.toString().contains("redacted"))

        val (_, rejected) = EnterpriseFieldManagementService.prepareInternalDispense(pool, uuid(6), "Tanque Sede", uuid(5), "ETHANOL", 10_000L)
        assertEquals(StockError.STOCK_PRODUCT_MISMATCH, rejected.error)
    }
}
