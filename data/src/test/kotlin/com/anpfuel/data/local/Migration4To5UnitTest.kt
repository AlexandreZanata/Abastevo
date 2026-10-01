package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T02 schema-4 upgrade guard (unit side).
 *
 * The device-side `V4ToV5DatabaseMigrationTest` proves vehicles/history
 * survive; here we prove MIGRATION_4_5 issues the additive
 * `backend_price_cache` DDL and never drops existing tables.
 */
class Migration4To5UnitTest {

    @Test
    fun `migration creates additive cache table`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_4_5.migrate(db)

        assertTrue(statements.any { it.contains("CREATE TABLE IF NOT EXISTS `backend_price_cache`") })
        assertTrue(statements.any { it.contains("index_backend_price_cache_station_id") })
        verify(exactly = 0) { db.execSQL(match { it.contains("DROP TABLE") }) }
    }
}
