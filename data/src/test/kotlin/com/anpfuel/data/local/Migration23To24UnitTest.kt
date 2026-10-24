package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * EE-21 snapshot identity guard: the 23→24 migration only adds the two
 * nullable identity columns. No table is rebuilt, no row is touched, so
 * every generation and private row survives with identity absent until
 * the next resnapshot fills it.
 */
class Migration23To24UnitTest {

    @Test
    fun `migration adds only identity columns`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_23_24.migrate(db)

        assertTrue(statements.size == 2)
        assertTrue(statements.any { it.contains("ADD COLUMN `entity_key` TEXT DEFAULT NULL") })
        assertTrue(statements.any { it.contains("ADD COLUMN `external_id` TEXT DEFAULT NULL") })
        assertTrue(statements.none { it.contains("DELETE FROM") })
        assertTrue(statements.none { it.contains("DROP TABLE") })
        assertTrue(statements.none { it.contains("CREATE TABLE") })
    }
}
