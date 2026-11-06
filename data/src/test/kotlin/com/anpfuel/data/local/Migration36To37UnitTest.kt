package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class Migration36To37UnitTest {

    @Test
    fun `migration creates forum_cache table with composite primary key`() {
        val db = mockk<SupportSQLiteDatabase>(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers { statements.add(sql.captured); Unit }

        AnpFuelDatabaseMigrations.MIGRATION_36_37.migrate(db)

        assertEquals(1, statements.size)
        val statement = statements.single()
        assertTrue(statement.contains("CREATE TABLE IF NOT EXISTS `forum_cache`"))
        assertTrue(statement.contains("`origin` TEXT NOT NULL"))
        assertTrue(statement.contains("`key` TEXT NOT NULL"))
        assertTrue(statement.contains("`kind` TEXT NOT NULL"))
        assertTrue(statement.contains("`payload_json` TEXT NOT NULL"))
        assertTrue(statement.contains("`fetched_at_millis` INTEGER NOT NULL"))
        assertTrue(statement.contains("`payload_bytes` INTEGER NOT NULL"))
        assertTrue(statement.contains("PRIMARY KEY(`origin`, `key`)"))
        assertFalse(statement.contains("DROP"))
        assertFalse(statement.contains("DELETE"))
    }
}
