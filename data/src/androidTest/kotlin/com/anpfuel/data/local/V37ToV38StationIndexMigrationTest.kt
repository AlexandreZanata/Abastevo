package com.anpfuel.data.local

import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class V37ToV38StationIndexMigrationTest {
    @get:Rule
    val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test
    fun upgradePreservesStationPriceAndUsesDiscoveryIndex() {
        val name = "station-index-37-38"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 37).use { db ->
                db.execSQL(
                    """
                    INSERT INTO survey_week (id, start_date, end_date, summary_imported_at)
                    VALUES ('week', '2026-11-01', '2026-11-07', 1000)
                    """.trimIndent(),
                )
                db.execSQL(
                    """
                    INSERT INTO station_price
                        (id, survey_week_id, cnpj, address, municipality, state, fuel_product, price)
                    VALUES ('station', 'week', '00000000000000', 'Test address', 'CUIABA', 'MT', 'ETHANOL', 3.59)
                    """.trimIndent(),
                )
            }
            helper.runMigrationsAndValidate(name, 38, true, AnpFuelDatabaseMigrations.MIGRATION_37_38).use { db ->
                val lookup = """
                    SELECT id, price FROM station_price
                    WHERE survey_week_id = 'week' AND state = 'MT'
                        AND municipality = 'CUIABA' AND fuel_product = 'ETHANOL'
                    ORDER BY price, id
                """.trimIndent()
                db.query(lookup).use { rows ->
                    assertTrue(rows.moveToFirst())
                    assertEquals("station", rows.getString(0))
                    assertEquals(3.59, rows.getDouble(1), 0.0)
                }
                db.query("EXPLAIN QUERY PLAN " + lookup).use { plan ->
                    assertTrue(plan.moveToFirst())
                    assertTrue(plan.getString(3).contains("idx_station_discovery_price"))
                }
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
