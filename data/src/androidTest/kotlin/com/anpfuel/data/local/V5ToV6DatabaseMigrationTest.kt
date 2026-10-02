package com.anpfuel.data.local

import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * P10-T05 — schema-5 upgrade preserves vehicles, survey weeks and cache
 * and adds the additive contribution outbox. ANP screens keep working
 * unchanged. Strengthened in P10-T08: vehicle and survey-week rows are
 * now inserted and asserted, not just the cache row.
 */
@RunWith(AndroidJUnit4::class)
class V5ToV6DatabaseMigrationTest {

    private val testDb = "v5-user-data-migration"

    @get:Rule
    val helper: MigrationTestHelper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Before
    fun clearStaleMigrationDatabase() {
        InstrumentationRegistry.getInstrumentation().targetContext.deleteDatabase(testDb)
    }

    @Test
    fun migrateFromV5PreservesCacheAndAddsOutbox() {
        LegacyFtsMigrationSupport.assumeLegacyFtsCreatable()
        helper.createDatabase(testDb, 5).apply {
            execSQL(
                """
                INSERT INTO vehicle (
                    id, display_name, tank_capacity_liters, fuel_product,
                    price_source_mode, specific_station_cnpj,
                    price_drop_alert_enabled, sort_order
                ) VALUES (
                    'vehicle-1', 'Civic', 44.0, 'GASOLINE_REGULAR',
                    'CHEAPEST', NULL, 0, 0
                )
                """.trimIndent(),
            )
            execSQL(
                """
                INSERT INTO survey_week (
                    id, start_date, end_date, summary_imported_at, station_imported_at
                ) VALUES (
                    'week-2026-06-07', '2026-06-07', '2026-06-13', 1718236800000, NULL
                )
                """.trimIndent(),
            )
            execSQL(
                """
                INSERT INTO backend_price_cache (
                    key, station_id, fuel_filter, payload_json, source, version,
                    fetched_at_millis, expires_at_millis
                ) VALUES (
                    'station|ALL', 'd6c74c23-63db-4c24-a2e5-408cb23bad26', 'ALL',
                    '{"items": []}', 'backend', 'v1', 1000000, 1060000
                )
                """.trimIndent(),
            )
            close()
        }

        helper.runMigrationsAndValidate(
            testDb,
            6,
            true,
            AnpFuelDatabaseMigrations.MIGRATION_5_6,
        ).apply {
            query("SELECT display_name FROM vehicle").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("Civic", cursor.getString(0))
            }
            query("SELECT COUNT(*) FROM survey_week").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(1, cursor.getInt(0))
            }
            query("SELECT COUNT(*) FROM backend_price_cache").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(1, cursor.getInt(0))
            }
            query(
                """
                SELECT name FROM sqlite_master
                WHERE type = 'table' AND name = 'contribution_outbox'
                """.trimIndent(),
            ).use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("contribution_outbox", cursor.getString(0))
            }
            close()
        }
    }
}
