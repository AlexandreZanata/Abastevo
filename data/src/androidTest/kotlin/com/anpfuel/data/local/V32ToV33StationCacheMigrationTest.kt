package com.anpfuel.data.local

import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class V32ToV33StationCacheMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradeKeepsScopedRowAddressAndCursor() {
        val name = "station-cache-32-33"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 32).use { db ->
                db.execSQL(
                    """
                    INSERT INTO server_station_cache
                    (station_id, display_name, location_quality, position, saved_at_millis, address_json, scope_key)
                    VALUES ('123e4567-e89b-12d3-a456-426614174000', '[P25-TEST] Trade', 'unknown',
                            2, 1000, '{"street":"Synthetic Street"}', 'city:3550308')
                    """.trimIndent(),
                )
                db.execSQL("INSERT INTO server_catalog_meta VALUES ('city:3550308', 'cursor-1', 1000)")
            }
            helper.runMigrationsAndValidate(name, 33, true, AnpFuelDatabaseMigrations.MIGRATION_32_33).use { db ->
                db.query(
                    "SELECT display_name, address_json, scope_key, position, legal_name, address_source FROM server_station_cache",
                ).use {
                    assertTrue(it.moveToFirst())
                    assertEquals("[P25-TEST] Trade", it.getString(0))
                    assertEquals("""{"street":"Synthetic Street"}""", it.getString(1))
                    assertEquals("city:3550308", it.getString(2))
                    assertEquals(2, it.getInt(3))
                    assertTrue(it.isNull(4))
                    assertTrue(it.isNull(5))
                    assertFalse(it.moveToNext())
                }
                db.query("SELECT next_cursor FROM server_catalog_meta WHERE key = 'city:3550308'").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("cursor-1", it.getString(0))
                }
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
