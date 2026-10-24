package com.anpfuel.data.local.map

import androidx.sqlite.db.SupportSQLiteDatabase
import com.anpfuel.data.local.AnpFuelDatabaseMigrations
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class Migration25To26UnitTest {
    @Test
    fun `migration creates append-only map tables without touching existing rows`() {
        val db = mockk<SupportSQLiteDatabase>(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers { statements.add(sql.captured); Unit }

        AnpFuelDatabaseMigrations.MIGRATION_25_26.migrate(db)

        assertEquals(4, statements.size)
        assertTrue(statements[0].contains("CREATE TABLE IF NOT EXISTS `map_generation`"))
        assertTrue(statements[1].contains("CREATE TABLE IF NOT EXISTS `map_package`"))
        assertTrue(statements[1].contains("FOREIGN KEY(`generation`)"))
        assertTrue(statements.any { it.contains("index_map_package_generation") })
        assertFalse(statements.any { it.contains("DROP TABLE") || it.contains("DELETE FROM") })
    }
}
