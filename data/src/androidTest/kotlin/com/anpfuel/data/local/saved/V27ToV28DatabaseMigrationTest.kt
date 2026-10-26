package com.anpfuel.data.local.saved

import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.AnpFuelDatabaseMigrations
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** MAP-25: 27→28 only adds the private saved tables; existing rows survive and the new tables are empty. */
@RunWith(AndroidJUnit4::class)
class V27ToV28DatabaseMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradePreservesExistingRowsAndCreatesEmptyOwnerScopedTables() {
        val name = "map25-migrate-27-28"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 27).apply {
                execSQL("INSERT INTO map_generation(generation, manifest_sha256, active, activated_at_millis, data_version) VALUES ('g1', 'abc', 1, 5, '261004')")
                close()
            }
            helper.runMigrationsAndValidate(name, 28, true, AnpFuelDatabaseMigrations.MIGRATION_27_28).apply {
                query("SELECT data_version, active FROM map_generation WHERE generation = 'g1'").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("261004", it.getString(0))
                    assertEquals(1, it.getInt(1))
                }
                for (table in listOf("map_saved_place", "map_saved_trip", "map_saved_trip_point", "map_saved_tombstone", "map_saved_import")) {
                    query("SELECT COUNT(*) FROM $table").use { assertTrue(it.moveToFirst()); assertEquals(table, 0, it.getInt(0)) }
                }
                // The same record id may exist for two owners (composite key), never twice for one.
                execSQL("INSERT INTO map_saved_place VALUES ('p','a','o','PLACE','n',1,1,NULL,'MANUAL',NULL,NULL,1)")
                execSQL("INSERT INTO map_saved_place VALUES ('p','b','o','PLACE','n',1,1,NULL,'MANUAL',NULL,NULL,1)")
                var duplicate = false
                try { execSQL("INSERT INTO map_saved_place VALUES ('p','a','o','PLACE','n',1,1,NULL,'MANUAL',NULL,NULL,1)") } catch (_: Exception) { duplicate = true }
                assertTrue(duplicate)
                close()
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
