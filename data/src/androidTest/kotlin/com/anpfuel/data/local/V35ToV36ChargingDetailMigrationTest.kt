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
class V35ToV36ChargingDetailMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradeKeepsNationalRowWithoutInventingDetailFields() {
        val name = "charging-detail-35-36"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 35).use { db ->
                db.execSQL(
                    """
                    INSERT INTO charging_snapshot_place
                    (place_id, generation, profile, source_revision, origin, license, name, country,
                     public_access, reported_points, groups_json, tombstone, saved_at_millis, catalog_confidence)
                    VALUES ('11111111-1111-4111-8111-111111111111', 4, 'charging-br-national-v1', 'synthetic',
                            'https://example.test', 'ODbL-1.0', '', 'BR', 'PUBLIC', 0, '[]', 0, 1000, 'SINGLE_SOURCE')
                    """.trimIndent(),
                )
            }
            helper.runMigrationsAndValidate(name, 36, true, AnpFuelDatabaseMigrations.MIGRATION_35_36).use { db ->
                db.query("SELECT name, catalog_confidence, operator, address_line FROM charging_snapshot_place").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("", it.getString(0))
                    assertEquals("SINGLE_SOURCE", it.getString(1))
                    assertTrue(it.isNull(2))
                    assertTrue(it.isNull(3))
                }
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
