package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * EE-20 local-only session context guard: the 22→23 migration only adds
 * the nullable context column. No table is rebuilt, no row is touched,
 * so every draft and private row survives with context absent.
 */
class Migration22To23UnitTest {

    @Test
    fun `migration adds only the context column`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_22_23.migrate(db)

        assertTrue(statements.size == 1)
        assertTrue(statements.single().contains("ADD COLUMN `context` TEXT DEFAULT NULL"))
        assertTrue(statements.none { it.contains("DELETE FROM") })
        assertTrue(statements.none { it.contains("DROP TABLE") })
        assertTrue(statements.none { it.contains("CREATE TABLE") })
        for (preserved in listOf(
            "`vehicle`", "`average_price`", "`station_price`", "`survey_week`",
            "`server_station_cache`", "`backend_price_cache`", "`contribution_outbox`",
            "`premium_outbox`", "`quality_report`", "`community_feed`", "`photo_upload`",
            "`municipality_catalog`", "`charging_snapshot", "`premium_cursor`",
            "`vehicle_capability_lease`", "`ledger_draft` (`",
        )) {
            assertTrue(statements.none { it.contains(preserved) }, "touched $preserved")
        }
    }
}
