package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * EE-19 energy-capable vehicle upgrade guard: the 19→20 migration
 * rebuilds only the `vehicle` table, preserving every row with legacy
 * values (ICE, no battery, photo kept). No other table is touched, so
 * fuel/history/snapshot/private rows survive untouched.
 */
class Migration19To20UnitTest {

    @Test
    fun `migration rebuilds only vehicle and preserves every row`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_19_20.migrate(db)

        val create = statements.single { it.contains("CREATE TABLE") && it.contains("`vehicle_new`") }
        assertTrue(create.contains("`propulsion` TEXT NOT NULL DEFAULT 'ICE'"))
        assertTrue(create.contains("`battery_wh` INTEGER"))
        assertTrue(create.contains("`charge_connector` TEXT"))
        assertTrue(create.contains("`tank_capacity_liters` REAL,"))
        assertTrue(create.contains("`fuel_product` TEXT,"))
        val copy = statements.single { it.contains("INSERT INTO `vehicle_new`") }
        assertTrue(copy.contains("SELECT `id`, `display_name`, `tank_capacity_liters`, `fuel_product`"))
        assertTrue(copy.contains("'ICE', NULL, NULL, `photo_path` FROM `vehicle`"))
        assertTrue(statements.indexOf(copy) < statements.indexOf("DROP TABLE `vehicle`"))
        assertTrue(statements.contains("ALTER TABLE `vehicle_new` RENAME TO `vehicle`"))
        assertTrue(statements.none { it.contains("DELETE FROM `vehicle`") })
        for (preserved in listOf(
            "average_price", "station_price", "survey_week", "server_station_cache",
            "backend_price_cache", "contribution_outbox", "premium_outbox",
            "quality_report", "community_feed", "photo_upload", "municipality_catalog",
            "charging_snapshot", "premium_cursor",
        )) {
            assertTrue(statements.none { it.contains(preserved) }, "touched $preserved")
        }
    }
}
