package com.anpfuel.domain.enterprise

import java.io.File
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

class EnterpriseTimeTest {
    @Test
    fun receiptAuthorityAndHistoricalSnapshotSurviveDeviceAndContextChanges() {
        val start = EnterpriseTime.parseInstant("2026-11-11T00:00:00Z")
        val receipt = EnterpriseTime.parseInstant("2026-11-12T00:00:00Z")
        val declared = TimeDeclaration(start, "Asia/Tokyo", 32400)
        val zones = ReportingZones("America/Sao_Paulo", "America/Cuiaba")
        val record = EnterpriseTime.capture(declared, receipt, zones)
        assertTrue(EnterpriseTime.inPeriod(record.occurredAt, start, receipt))
        assertFalse(EnterpriseTime.inPeriod(record.authorityAt(), start, receipt))
        listOf(start.minusSeconds(3600), receipt.plusSeconds(3600)).forEach { occurred ->
            assertEquals(record.authorityAt(), EnterpriseTime.capture(declared.copy(occurredAt = occurred), receipt, zones).authorityAt())
        }
        val skew = EnterpriseTime.assessClockSkew(receipt.plusSeconds(720), receipt)
        assertEquals("CLOCK_SKEW", skew.flag)
        val changed = EnterpriseTime.capture(declared, receipt, zones.copy(unit = "Asia/Tokyo"))
        assertEquals("2026-11-11", changed.reportingDay)
        assertEquals("2026-11-10", record.reportingDay)
        assertEquals("America/Cuiaba", record.reportingZone)
        assertEquals("unit", record.reportingSource)
        assertEquals(start, record.occurredAt)
        assertEquals(receipt, record.receivedAt)
        assertEquals("Asia/Tokyo", record.entryZone)
        assertEquals(32400L, record.deviceOffsetSeconds)
    }

    @Test
    fun nativeInstantInputsCannotBypassSupportedRange() {
        val valid = EnterpriseTime.parseInstant("2026-11-11T00:00:00Z")
        listOf(Instant.parse("0000-01-01T00:00:00Z"), Instant.parse("+10000-01-01T00:00:00Z"), Instant.MIN, Instant.MAX).forEach { bad ->
            val declaration = TimeDeclaration(valid, "UTC", 0)
            val zones = ReportingZones("UTC")
            assertThrows(EnterpriseTimeException::class.java) { EnterpriseTime.capture(declaration.copy(occurredAt = bad), valid, zones) }
            assertThrows(EnterpriseTimeException::class.java) { EnterpriseTime.capture(declaration, bad, zones) }
            listOf(listOf(bad, valid, valid.plusSeconds(1)), listOf(valid, bad, valid), listOf(valid, valid, bad)).forEach { (at, start, end) ->
                assertThrows(EnterpriseTimeException::class.java) { EnterpriseTime.inPeriod(at, start, end) }
            }
            assertThrows(EnterpriseTimeException::class.java) { EnterpriseTime.assessClockSkew(bad, valid) }
            assertThrows(EnterpriseTimeException::class.java) { EnterpriseTime.assessClockSkew(valid, bad) }
        }
    }

    @Test
    fun declarationValidationAcceptsMinimumWithoutReceiptOrAnInventedReportingCalendar() {
        val declared = TimeDeclaration(Instant.parse("0001-01-01T00:00:00Z"), "America/Cuiaba", 64800)
        assertEquals(declared, EnterpriseTime.validateDeclaration(declared))
        assertThrows(EnterpriseTimeException::class.java) {
            EnterpriseTime.capture(declared, Instant.parse("2026-11-14T00:00:00Z"), ReportingZones("America/Cuiaba"))
        }
    }

    @Test
    fun declarationValidationPreservesOffsetDisagreementAndCaptureReceiptAuthority() {
        val occurred = EnterpriseTime.parseInstant("2026-11-14T03:00:00+03:00")
        val declared = TimeDeclaration(occurred, "Asia/Tokyo", -64800)
        val validated = EnterpriseTime.validateDeclaration(declared)
        assertEquals("2026-11-14T00:00:00Z", EnterpriseTime.formatInstant(validated.occurredAt))
        assertEquals(-64800L, validated.deviceOffsetSeconds)
        val received = Instant.parse("2026-11-15T00:00:00Z")
        val captured = EnterpriseTime.capture(validated, received, ReportingZones("UTC"))
        assertEquals(received, captured.authorityAt())
        assertEquals("2026-11-14", captured.reportingDay)
        assertThrows(EnterpriseTimeException::class.java) {
            EnterpriseTime.capture(validated, Instant.parse("0001-01-01T00:00:00Z"), ReportingZones("UTC"))
        }
    }

    @Test
    fun declarationValidationRejectsTheExistingInstantZoneAndOffsetFailures() {
        val at = Instant.parse("2026-11-14T00:00:00Z")
        val base = TimeDeclaration(at, "UTC", 0)
        val invalid = listOf(base.copy(occurredAt = Instant.MIN), base.copy(occurredAt = Instant.MAX),
            base.copy(occurredAt = Instant.parse("0000-01-01T00:00:00Z")), base.copy(entryZone = "UTC "),
            base.copy(entryZone = "Not/A_Zone"), base.copy(entryZone = "posix/UTC"),
            base.copy(deviceOffsetSeconds = 64801), base.copy(deviceOffsetSeconds = -64801))
        for (declared in invalid) assertThrows(EnterpriseTimeException::class.java) { EnterpriseTime.validateDeclaration(declared) }
    }

    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map {
            val pair = it.split('=', limit = 2)
            require(pair.size == 2 && pair[0].isNotEmpty())
            pair[0] to pair[1]
        }
        require(pairs.map { it.first }.toSet().size == pairs.size)
        return pairs.toMap()
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "INSTANT" -> mapOf("value" to EnterpriseTime.formatInstant(EnterpriseTime.parseInstant(a.getValue("text"))))
        "RECORD" -> EnterpriseTime.capture(
            TimeDeclaration(
                EnterpriseTime.parseInstant(a.getValue("occurred")),
                a.getValue("entry"), a.getValue("offset").toLong(),
            ),
            EnterpriseTime.parseInstant(a.getValue("received")),
            ReportingZones(a.getValue("org"), a.getValue("unit").takeUnless { it == "?" }),
        ).let {
            mapOf(
                "occurred" to EnterpriseTime.formatInstant(it.occurredAt),
                "received" to EnterpriseTime.formatInstant(it.receivedAt),
                "authority" to EnterpriseTime.formatInstant(it.authorityAt()),
                "entry" to it.entryZone, "offset" to it.deviceOffsetSeconds.toString(),
                "day" to it.reportingDay, "zone" to it.reportingZone, "source" to it.reportingSource,
            )
        }
        "PERIOD" -> mapOf(
            "inside" to EnterpriseTime.inPeriod(
                EnterpriseTime.parseInstant(a.getValue("at")),
                EnterpriseTime.parseInstant(a.getValue("start")),
                EnterpriseTime.parseInstant(a.getValue("end")),
            ).toString(),
        )
        "SKEW" -> EnterpriseTime.assessClockSkew(
            EnterpriseTime.parseInstant(a.getValue("device")),
            EnterpriseTime.parseInstant(a.getValue("server")),
            a.getValue("limit").takeUnless { it == "?" }?.toLong(),
        ).let {
            mapOf(
                "seconds" to it.seconds.toString(), "nanos" to it.nanos.toString(),
                "flag" to it.flag, "limit" to it.limitSeconds.toString(),
            )
        }
        else -> error("Unknown shared operation")
    }

    @TestFactory
    fun sharedEnterpriseTimeVectors(): List<DynamicTest> {
        val lines = File("../contracts/testdata/enterprise/time-v1.psv").readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        assertEquals(99, rows.size)
        require(rows.all { it.size == 4 && it[0].isNotEmpty() })
        assertEquals(rows.size, rows.map { it[0] }.toSet().size)
        return rows.map { (id, operation, rawArgs, rawExpected) ->
            DynamicTest.dynamicTest(id) {
                val got = try {
                    execute(operation, fields(rawArgs))
                } catch (_: EnterpriseTimeException) {
                    mapOf("error" to "invalid-time")
                }
                assertEquals(fields(rawExpected), got)
            }
        }
    }
}
