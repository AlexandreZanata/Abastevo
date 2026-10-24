package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * MEX-08 capability-lease upgrade guard: the 20→21 migration only
 * creates the additive lease table. No existing table is altered, no row
 * is moved or deleted, so every vehicle and private row survives.
 */
class Migration20To21UnitTest {

    @Test
    fun `migration creates only the capability lease table`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_20_21.migrate(db)

        assertTrue(
            statements.any {
                it.contains("CREATE TABLE") && it.contains("`vehicle_capability_lease`") &&
                    it.contains("`verified_at_millis` INTEGER NOT NULL") &&
                    it.contains("PRIMARY KEY(`owner_account`, `origin`)")
            },
        )
        assertTrue(statements.none { it.contains("DELETE FROM") })
        assertTrue(statements.none { it.contains("DROP TABLE") })
        assertTrue(statements.none { it.contains("ALTER TABLE") })
        for (preserved in listOf(
            "`average_price`", "`station_price`", "`survey_week`", "`server_station_cache`",
            "`backend_price_cache`", "`contribution_outbox`", "`premium_outbox`", "`vehicle`",
            "`quality_report`", "`community_feed`", "`photo_upload`", "`municipality_catalog`",
            "`charging_snapshot", "`premium_cursor`",
        )) {
            assertTrue(statements.none { it.contains(preserved) }, "touched $preserved")
        }
    }
}
