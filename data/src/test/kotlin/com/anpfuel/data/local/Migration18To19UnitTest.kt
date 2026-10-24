package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * EE-18 public EV snapshot upgrade guard: the 18→19 migration only
 * creates the additive snapshot tables. No existing table is altered,
 * no row is moved or deleted, so reopening an upgraded database keeps
 * every ANP/history/fuel row and every private/outbox row intact.
 */
class Migration18To19UnitTest {

    @Test
    fun `migration creates only the EV snapshot tables`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_18_19.migrate(db)

        assertTrue(
            statements.any {
                it.contains("CREATE TABLE") && it.contains("`charging_snapshot_place`") &&
                    it.contains("`groups_json` TEXT NOT NULL") &&
                    it.contains("PRIMARY KEY(`place_id`, `generation`)")
            },
        )
        assertTrue(
            statements.any {
                it.contains("CREATE TABLE") && it.contains("`charging_snapshot_meta`") &&
                    it.contains("PRIMARY KEY(`key`)")
            },
        )
        assertTrue(statements.any { it.contains("`index_charging_snapshot_place_generation`") })
        assertTrue(statements.none { it.contains("DELETE FROM") })
        assertTrue(statements.none { it.contains("DROP TABLE") })
        assertTrue(statements.none { it.contains("ALTER TABLE") })
        for (preserved in listOf(
            "average_price", "station_price", "survey_week", "server_station_cache",
            "backend_price_cache", "contribution_outbox", "premium_outbox", "vehicle",
            "quality_report", "community_feed", "photo_upload", "municipality_catalog",
            "import_audit", "premium_cursor",
        )) {
            assertTrue(statements.none { it.contains(preserved) }, "touched $preserved")
        }
    }
}
