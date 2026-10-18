package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P27 scope isolation upgrade guard: the catalog primary key becomes
 * (station_id, scope_key); surviving rows are backfilled from the
 * stored cursor scope and nothing outside server_station_cache moves.
 */
class Migration13To14UnitTest {

    @Test
    fun `migration rebuilds scoped catalog preserving rows without touching other tables`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_13_14.migrate(db)

        assertTrue(
            statements.any {
                it.contains("CREATE TABLE") && it.contains("`server_station_cache_new`") &&
                    it.contains("`scope_key` TEXT NOT NULL") &&
                    it.contains("PRIMARY KEY(`station_id`, `scope_key`)")
            },
        )
        assertTrue(
            statements.any {
                it.contains("INSERT INTO `server_station_cache_new`") &&
                    it.contains("FROM `server_station_cache`") &&
                    it.contains("COALESCE((SELECT `key` FROM `server_catalog_meta` LIMIT 1), 'page')")
            },
        )
        assertTrue(statements.any { it.contains("DROP TABLE `server_station_cache`") })
        assertTrue(
            statements.any {
                it.contains("ALTER TABLE `server_station_cache_new` RENAME TO `server_station_cache`")
            },
        )
        verify(exactly = 0) { db.execSQL(match { it.contains("DELETE FROM") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("station_price") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("survey_week") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("average_price") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("DROP TABLE `station_price") }) }
    }
}
