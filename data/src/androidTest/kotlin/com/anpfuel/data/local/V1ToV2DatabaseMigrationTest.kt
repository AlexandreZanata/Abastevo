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
 * Phase 15.2.3 — verifies v1.0.0 Room data survives migrations to v3 (national search catalog).
 */
@RunWith(AndroidJUnit4::class)
class V1ToV2DatabaseMigrationTest {

    private val testDb = "v1-user-data-migration"

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
    fun migrateFromV1PreservesImportedPricesAndAddsMunicipalityCatalog() {
        helper.createDatabase(testDb, 1).apply {
            execSQL(
                """
                INSERT INTO survey_week (
                    id, start_date, end_date, summary_imported_at, station_imported_at
                ) VALUES (
                    'week-2026-06-07',
                    '2026-06-07',
                    '2026-06-13',
                    1718236800000,
                    NULL
                )
                """.trimIndent(),
            )
            execSQL(
                """
                INSERT INTO average_price (
                    id, survey_week_id, state, municipality, fuel_product,
                    station_count, unit, avg_price, min_price, max_price, std_dev
                ) VALUES (
                    'avg-curitiba-ethanol',
                    'week-2026-06-07',
                    'PR',
                    'CURITIBA',
                    'ETHANOL',
                    42,
                    'R$/l',
                    3.42,
                    3.10,
                    3.80,
                    0.12
                )
                """.trimIndent(),
            )
            close()
        }

        helper.runMigrationsAndValidate(
            testDb,
            4,
            true,
            AnpFuelDatabaseMigrations.MIGRATION_1_2,
            AnpFuelDatabaseMigrations.MIGRATION_2_3,
            AnpFuelDatabaseMigrations.MIGRATION_3_4,
        ).apply {
            query("SELECT COUNT(*) FROM average_price").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(1, cursor.getInt(0))
            }

            query(
                """
                SELECT municipality, state, avg_price
                FROM average_price
                WHERE id = 'avg-curitiba-ethanol'
                """.trimIndent(),
            ).use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("CURITIBA", cursor.getString(0))
                assertEquals("PR", cursor.getString(1))
                assertEquals(3.42, cursor.getDouble(2), 0.001)
            }

            query(
                """
                SELECT name FROM sqlite_master
                WHERE type = 'table' AND name = 'municipality_catalog'
                """.trimIndent(),
            ).use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("municipality_catalog", cursor.getString(0))
            }

            query(
                """
                SELECT sql FROM sqlite_master
                WHERE type = 'table' AND name = 'municipality_fts'
                """.trimIndent(),
            ).use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertTrue(cursor.getString(0).contains("content=`municipality_catalog`"))
            }

            query(
                """
                SELECT name FROM sqlite_master
                WHERE type = 'table' AND name = 'vehicle'
                """.trimIndent(),
            ).use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("vehicle", cursor.getString(0))
            }

            close()
        }
    }
}
