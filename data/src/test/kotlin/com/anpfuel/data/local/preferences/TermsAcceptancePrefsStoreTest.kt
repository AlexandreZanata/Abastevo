package com.anpfuel.data.local.preferences

import com.anpfuel.domain.legal.TermsAcceptance
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class TermsAcceptancePrefsStoreTest {
    private class Disk {
        var raw: String? = null
        var accept = true
        var broken = false
        fun open() = TermsAcceptancePrefsStore({ if (broken) error("disk") else raw }, { value -> if (accept) raw = value; accept })
    }
    private val record = TermsAcceptance("2026-10", "1.0", 1_800_000_000_000L)

    @Test fun `record survives reconstruction and holds only versions and the instant`() {
        val disk = Disk()
        assertNull(disk.open().load())
        assertTrue(disk.open().save(record))
        assertEquals(record, disk.open().load())
        assertEquals(setOf("terms_version", "privacy_version", "accepted_at"), org.json.JSONObject(disk.raw!!).keys().asSequence().toSet())
    }

    @Test fun `clear removes the record`() {
        val disk = Disk()
        disk.open().save(record)
        assertTrue(disk.open().clear())
        assertNull(disk.raw)
        assertNull(disk.open().load())
    }

    @Test fun `refused writes report failure and never claim a record`() {
        val disk = Disk().apply { accept = false }
        assertFalse(disk.open().save(record))
        assertNull(disk.open().load())
        disk.accept = true
        disk.open().save(record)
        disk.accept = false
        assertFalse(disk.open().clear())
        assertEquals(record, disk.open().load())
    }

    @Test fun `corrupt partial foreign or oversized data loads as no record`() {
        val disk = Disk()
        for (raw in listOf(
            "", "not json", "[]", "{}",
            """{"terms_version":"2026-10","privacy_version":"1.0"}""",
            """{"terms_version":"2026-10","privacy_version":"1.0","accepted_at":0}""",
            """{"terms_version":"2026-10","privacy_version":"1.0","accepted_at":-5}""",
            """{"terms_version":"2026-10","privacy_version":"1.0","accepted_at":"1800000000000"}""",
            """{"terms_version":"2026-10","privacy_version":"1.0","accepted_at":1.5}""",
            """{"terms_version":"","privacy_version":"1.0","accepted_at":1800000000000}""",
            """{"terms_version":7,"privacy_version":"1.0","accepted_at":1800000000000}""",
            """{"terms_version":"2026-10","privacy_version":"1.0","accepted_at":1800000000000,"account":"x"}""",
            """{"terms_version":"${"x".repeat(600)}","privacy_version":"1.0","accepted_at":1800000000000}""",
        )) {
            disk.raw = raw
            assertNull(disk.open().load(), raw.take(60))
        }
    }

    @Test fun `an unreadable disk loads as no record and an invalid record is not written`() {
        val disk = Disk().apply { broken = true }
        assertNull(disk.open().load())
        disk.broken = false
        assertFalse(disk.open().save(TermsAcceptance("2026-10", "1.0", 0)))
        assertFalse(disk.open().save(TermsAcceptance("bad version", "1.0", 5)))
        assertNull(disk.raw)
    }
}
