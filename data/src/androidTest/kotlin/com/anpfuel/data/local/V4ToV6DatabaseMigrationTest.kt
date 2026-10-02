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
 * P10-T08 — Full schema-4 upgrade chain preserves user data and ANP
 * history while adding the community cache (v5) and the contribution
 * outbox (v6). No table holding user data is dropped; rollback is the
 * tested previous compatible release with community flags OFF.
 */
@RunWith(AndroidJUnit4::class)
class V4ToV6DatabaseMigrationTest {

    private val testDb = "v4-to-v6-user-data-migration"

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
    fun migrateFromV4ToV6PreservesVehiclesHistoryAndAddsCommunityTables() {
        LegacyFtsMigrationSupport.assumeLegacyFtsCreatable()
        helper.createDatabase(testDb, 4).apply {
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
                INSERT INTO average_price (
                    id, survey_week_id, state, municipality, fuel_product,
                    station_count, unit, avg_price, min_price, max_price, std_dev
                ) VALUES (
                    'avg-sp-ethanol', 'week-2026-06-07', 'SP', 'SAO PAULO', 'ETHANOL',
                    42, 'R$/l', 3.42, 3.10, 3.80, 0.12
                )
                """.trimIndent(),
            )
            close()
        }

        helper.runMigrationsAndValidate(
            testDb,
            6,
            true,
            AnpFuelDatabaseMigrations.MIGRATION_4_5,
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
            query("SELECT avg_price FROM average_price").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(3.42, cursor.getDouble(0), 0.0001)
            }
            query(
                """
                SELECT name FROM sqlite_master
                WHERE type = 'table' AND name IN ('backend_price_cache', 'contribution_outbox')
                """.trimIndent(),
            ).use { cursor ->
                val tables = mutableSetOf<String>()
                while (cursor.moveToNext()) {
                    tables += cursor.getString(0)
                }
                assertTrue(tables.contains("backend_price_cache"))
                assertTrue(tables.contains("contribution_outbox"))
            }
            close()
        }
    }
}
