package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class DriverMigration31To32UnitTest {
    @Test fun `migration adds reconciliation goals and maintenance`() {
        val db = mockk<SupportSQLiteDatabase>(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers { statements.add(sql.captured); Unit }
        AnpFuelDatabaseMigrations.MIGRATION_31_32.migrate(db)
        assertEquals(3, statements.size)
        listOf("driver_reconciliation", "driver_goal", "driver_maintenance").forEach { table ->
            assertTrue(statements.any { it.contains("CREATE TABLE IF NOT EXISTS `$table`") }, table)
        }
    }
}
