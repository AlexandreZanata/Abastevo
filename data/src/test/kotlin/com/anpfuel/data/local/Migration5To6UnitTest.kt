package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T05 schema-5 upgrade guard (unit side).
 *
 * The device-side `V5ToV6DatabaseMigrationTest` proves cache/vehicles
 * survive; here we prove MIGRATION_5_6 issues the additive
 * `contribution_outbox` DDL and never drops existing tables.
 */
class Migration5To6UnitTest {

    @Test
    fun `migration creates additive outbox table`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_5_6.migrate(db)

        assertTrue(statements.any { it.contains("CREATE TABLE IF NOT EXISTS `contribution_outbox`") })
        assertTrue(statements.any { it.contains("PRIMARY KEY(`command_id`)") })
        verify(exactly = 0) { db.execSQL(match { it.contains("DROP TABLE") }) }
    }
}
