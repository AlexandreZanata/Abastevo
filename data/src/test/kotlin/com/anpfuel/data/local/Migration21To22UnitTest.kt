package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * MEX-11 durable ledger draft guard: the 21→22 migration only creates
 * the additive draft table. No existing table is altered, no row is
 * moved or deleted, so every vehicle/snapshot/private row survives.
 */
class Migration21To22UnitTest {

    @Test
    fun `migration creates only the ledger draft table`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_21_22.migrate(db)

        assertTrue(
            statements.any {
                it.contains("CREATE TABLE") && it.contains("`ledger_draft`") &&
                    it.contains("`body_json` TEXT NOT NULL") &&
                    it.contains("PRIMARY KEY(`draft_id`)")
            },
        )
        assertTrue(statements.any { it.contains("`index_ledger_draft_owner_record`") })
        assertTrue(statements.none { it.contains("DELETE FROM") })
        assertTrue(statements.none { it.contains("DROP TABLE") })
        assertTrue(statements.none { it.contains("ALTER TABLE") })
        for (preserved in listOf(
            "`vehicle`", "`average_price`", "`station_price`", "`survey_week`",
            "`server_station_cache`", "`backend_price_cache`", "`contribution_outbox`",
            "`premium_outbox`", "`quality_report`", "`community_feed`", "`photo_upload`",
            "`municipality_catalog`", "`charging_snapshot", "`premium_cursor`",
            "`vehicle_capability_lease`",
        )) {
            assertTrue(statements.none { it.contains(preserved) }, "touched $preserved")
        }
    }
}
