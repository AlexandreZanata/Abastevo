package com.anpfuel.data.local.map

import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.AnpFuelDatabaseMigrations
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase

/**
 * Real historical upgrade for MAP-15: the last exported schema (18) migrates
 * through every append-only step to 26, preserving existing rows and creating
 * only the two public map tables. Binaries never enter Room.
 */
@RunWith(AndroidJUnit4::class)
class V25ToV26DatabaseMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    private val allMigrations = arrayOf(
        AnpFuelDatabaseMigrations.MIGRATION_18_19,
        AnpFuelDatabaseMigrations.MIGRATION_19_20,
        AnpFuelDatabaseMigrations.MIGRATION_20_21,
        AnpFuelDatabaseMigrations.MIGRATION_21_22,
        AnpFuelDatabaseMigrations.MIGRATION_22_23,
        AnpFuelDatabaseMigrations.MIGRATION_23_24,
        AnpFuelDatabaseMigrations.MIGRATION_24_25,
        AnpFuelDatabaseMigrations.MIGRATION_25_26,
    )

    @Test fun historicalUpgradePreservesRowsAndCreatesOnlyMapTables() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val name = "map-migrate-18-26"
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 18).apply {
                execSQL(
                    """INSERT INTO vehicle(id,display_name,tank_capacity_liters,fuel_product,price_source_mode,specific_station_cnpj,price_drop_alert_enabled,sort_order)
                    VALUES ('synthetic-car','Synthetic car',50,'GASOLINE_REGULAR','CITY_AVERAGE',NULL,0,0)""",
                )
                close()
            }
            helper.runMigrationsAndValidate(name, 26, true, *allMigrations).apply {
                query("SELECT display_name,tank_capacity_liters FROM vehicle WHERE id='synthetic-car'").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("Synthetic car", it.getString(0))
                }
                query("SELECT count(*) FROM map_generation").use {
                    assertTrue(it.moveToFirst()); assertEquals(0, it.getInt(0))
                }
                query("SELECT count(*) FROM map_package").use {
                    assertTrue(it.moveToFirst()); assertEquals(0, it.getInt(0))
                }
                // Binaries stay outside Room: no BLOB column on the new tables.
                query("PRAGMA table_info(map_package)").use { cursor ->
                    val names = mutableListOf<String>()
                    while (cursor.moveToNext()) names += cursor.getString(1)
                    assertTrue(names.containsAll(listOf("package_id", "generation", "file_name", "bytes", "sha256", "status")))
                    assertTrue(names.none { it.contains("blob", ignoreCase = true) || it.contains("bytes_data") })
                }
                close()
            }
        } finally {
            context.deleteDatabase(name)
        }
    }

    @Test fun failedMapMigrationRollsBackAndCanRecover() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val name = "map-migrate-rollback"
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 18).apply { close() }
            val failingTail = allMigrations.sliceArray(0 until 7) + object : Migration(25, 26) {
                override fun migrate(db: SupportSQLiteDatabase) {
                    AnpFuelDatabaseMigrations.MIGRATION_25_26.migrate(db)
                    db.execSQL("INSERT INTO map_expected_missing_table(id) VALUES(1)")
                }
            }
            try {
                helper.runMigrationsAndValidate(name, 26, true, *failingTail).close()
                fail("injected SQL failure must abort migration")
            } catch (_: Exception) {
            }
            // Recovery with the real migration succeeds.
            context.deleteDatabase(name)
            helper.createDatabase(name, 18).apply { close() }
            helper.runMigrationsAndValidate(name, 26, true, *allMigrations).apply {
                query("SELECT count(*) FROM map_generation").use {
                    assertTrue(it.moveToFirst()); assertEquals(0, it.getInt(0))
                }
                close()
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
