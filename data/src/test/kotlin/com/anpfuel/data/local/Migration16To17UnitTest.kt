package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * OPT community snapshot upgrade guard: the 16→17 migration only creates
 * the additive `community_feed_cache` table. No existing table is altered,
 * no row is moved or deleted, so reopening an upgraded database keeps every
 * ANP/catalog/outbox row intact.
 */
class Migration16To17UnitTest {

    @Test
    fun `migration creates only the community snapshot table`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_16_17.migrate(db)

        assertTrue(
            statements.any {
                it.contains("CREATE TABLE") && it.contains("`community_feed_cache`") &&
                    it.contains("`payload_json` TEXT NOT NULL") &&
                    it.contains("`fetched_at_millis` INTEGER NOT NULL") &&
                    it.contains("PRIMARY KEY(`key`)")
            },
        )
        assertTrue(statements.none { it.contains("DELETE FROM") })
        assertTrue(statements.none { it.contains("DROP TABLE") })
        assertTrue(statements.none { it.contains("ALTER TABLE") })
        assertTrue(statements.none { it.contains("average_price") })
        assertTrue(statements.none { it.contains("station_price") })
        assertTrue(statements.none { it.contains("survey_week") })
        assertTrue(statements.none { it.contains("server_station_cache") })
        assertTrue(statements.none { it.contains("backend_price_cache") })
    }
}
