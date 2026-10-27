package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class DriverMigration28To29UnitTest {
    @Test
    fun `migration creates owner-scoped driver tables without touching existing rows`() {
        val db = mockk<SupportSQLiteDatabase>(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers { statements.add(sql.captured); Unit }

        AnpFuelDatabaseMigrations.MIGRATION_28_29.migrate(db)

        assertEquals(14, statements.size)
        listOf("driver_shift", "driver_shift_gap", "driver_earning", "driver_earning_revision", "driver_distance", "driver_local_command", "driver_draft").forEach { table ->
            assertTrue(statements.any { it.contains("CREATE TABLE IF NOT EXISTS `$table`") }, table)
        }
        listOf("index_driver_shift_owner_state", "index_driver_shift_gap_owner_shift", "index_driver_earning_owner_window", "index_driver_earning_revision_owner_record", "index_driver_distance_owner_window", "index_driver_command_owner_state", "index_driver_draft_owner_updated").forEach { index ->
            assertTrue(statements.any { it.contains("CREATE INDEX IF NOT EXISTS `$index`") }, index)
        }
        assertFalse(statements.any { it.contains("DROP TABLE") || it.contains("DELETE FROM") || it.contains("ALTER TABLE") })
        assertFalse(statements.any { it.contains("latitude") || it.contains("longitude") || it.contains("gps") })
    }
}
