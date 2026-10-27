package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class DriverMigration29To30UnitTest {
    @Test
    fun `migration adds zero lease without rewriting queued commands`() {
        val db = mockk<SupportSQLiteDatabase>(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers { statements.add(sql.captured); Unit }

        AnpFuelDatabaseMigrations.MIGRATION_29_30.migrate(db)

        assertEquals(1, statements.size)
        assertTrue(statements[0].contains("ALTER TABLE `driver_local_command` ADD COLUMN `lease_until_millis` INTEGER NOT NULL DEFAULT 0"))
        assertFalse(statements.any { it.contains("DROP") || it.contains("DELETE") || it.contains("UPDATE") })
    }
}
