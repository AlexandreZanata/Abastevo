package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P27-T03 schema-8 upgrade guard (unit side).
 *
 * The device-side migration test proves old installs keep their rows;
 * here we prove MIGRATION_7_8 issues only the two additive catalog
 * tables, creates no undeclared secondary index (every catalog read
 * is by primary key or full ordered scan) and never drops existing
 * tables.
 */
class Migration7To8UnitTest {

    @Test
    fun `migration creates additive catalog tables only`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_7_8.migrate(db)

        assertTrue(
            statements.any {
                it.contains("CREATE TABLE IF NOT EXISTS `server_station_cache`") &&
                    it.contains("`station_id` TEXT NOT NULL") &&
                    it.contains("`latitude` REAL") &&
                    it.contains("PRIMARY KEY(`station_id`)")
            },
        )
        assertTrue(
            statements.any {
                it.contains("CREATE TABLE IF NOT EXISTS `server_catalog_meta`") &&
                    it.contains("PRIMARY KEY(`key`)")
            },
        )
        assertTrue(statements.none { it.contains("DROP TABLE") })
        assertTrue(statements.none { it.contains("CREATE INDEX") })
    }
}
