package com.anpfuel.data.local.map

import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.AnpFuelDatabaseMigrations
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import androidx.test.ext.junit.runners.AndroidJUnit4

/**
 * MAP-15 native-layout retention: 26→27 adds only `map_generation.data_version`
 * (the SDK numeric version directory). Existing rows survive with the empty
 * default and count as unpublished until re-activated.
 */
@RunWith(AndroidJUnit4::class)
class V26ToV27DatabaseMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradePreservesMapRowsAndDefaultsDataVersion() {
        val name = "map-migrate-26-27"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 26).apply {
                execSQL(
                    "INSERT INTO map_generation(generation, manifest_sha256, active, activated_at_millis) " +
                        "VALUES ('g1', 'abc', 1, 5)",
                )
                execSQL(
                    "INSERT INTO map_package(package_id, generation, role, file_name, bytes, sha256, etag, status, downloaded_bytes, error, updated_at_millis) " +
                        "VALUES ('region', 'g1', 'REGION', 'region.mwm', 10, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'e', 'READY', 10, NULL, 6)",
                )
                close()
            }
            helper.runMigrationsAndValidate(name, 27, true, AnpFuelDatabaseMigrations.MIGRATION_26_27).apply {
                query("SELECT data_version, active FROM map_generation WHERE generation = 'g1'").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("", it.getString(0))
                    assertEquals(1, it.getInt(1))
                }
                query("SELECT file_name, status FROM map_package WHERE generation = 'g1'").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("region.mwm", it.getString(0))
                    assertEquals("READY", it.getString(1))
                }
                close()
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
