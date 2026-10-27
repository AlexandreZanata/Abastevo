package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class DriverMigration30To31UnitTest {
    @Test
    fun `migration adds shift odometers payouts costs and gps tables`() {
        val db = mockk<SupportSQLiteDatabase>(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers { statements.add(sql.captured); Unit }
        AnpFuelDatabaseMigrations.MIGRATION_30_31.migrate(db)
        assertEquals(7, statements.size)
        assertTrue(statements.any { it.contains("ADD COLUMN `start_odometer_metres`") })
        assertTrue(statements.any { it.contains("ADD COLUMN `end_odometer_metres`") })
        listOf("driver_payout", "driver_cost_setting", "driver_gps_observation").forEach { table ->
            assertTrue(statements.any { it.contains("CREATE TABLE IF NOT EXISTS `$table`") }, table)
        }
    }
}
