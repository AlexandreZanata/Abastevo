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
 * P10-T02 — schema-4 upgrade preserves vehicles/history and adds the
 * additive backend price cache. ANP screens keep working unchanged.
 */
@RunWith(AndroidJUnit4::class)
class V4ToV5DatabaseMigrationTest {

    private val testDb = "v4-user-data-migration"

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
    fun migrateFromV4PreservesVehiclesAndAddsBackendCache() {
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
            close()
        }

        helper.runMigrationsAndValidate(
            testDb,
            5,
            true,
            AnpFuelDatabaseMigrations.MIGRATION_4_5,
        ).apply {
            query("SELECT COUNT(*) FROM vehicle").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(1, cursor.getInt(0))
            }
            query("SELECT COUNT(*) FROM survey_week").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(1, cursor.getInt(0))
            }
            query(
                """
                SELECT name FROM sqlite_master
                WHERE type = 'table' AND name = 'backend_price_cache'
                """.trimIndent(),
            ).use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("backend_price_cache", cursor.getString(0))
            }
            close()
        }
    }
}
