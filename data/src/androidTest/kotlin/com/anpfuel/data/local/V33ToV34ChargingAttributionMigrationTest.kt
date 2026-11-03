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
class V33ToV34ChargingAttributionMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradeKeepsLegacyPlaceAndGenerationWithoutInventingSources() {
        val name = "charging-attribution-33-34"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 33).use { db ->
                db.execSQL(
                    """
                    INSERT INTO charging_snapshot_place
                    (place_id, entity_key, external_id, generation, profile, source_revision, origin, license,
                     name, country, latitude, longitude, public_access, reported_points, groups_json, tombstone,
                     saved_at_millis)
                    VALUES ('11111111-1111-4111-8111-111111111111', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', '7',
                            4, 'ocm-br-20261021-v1', 'rev-1', 'https://example.test', 'legacy',
                            'Synthetic place', 'BR', '-23.5', '-46.6', 'PUBLIC', 1, '[{"id":"group"}]', 0, 1000)
                    """.trimIndent(),
                )
                db.execSQL(
                    """
                    INSERT INTO charging_snapshot_meta
                    (key, generation, profile, source_revision, origin, license, schema_version,
                     base_generation, place_count, group_count, cursor, state, saved_at_millis)
                    VALUES ('pack:ocm-br-20261021-v1', 4, 'ocm-br-20261021-v1', 'rev-1', 'https://example.test',
                            'legacy', 1, 3, 1, 1, NULL, 'COMPLETE', 1000)
                    """.trimIndent(),
                )
            }
            helper.runMigrationsAndValidate(name, 34, true, AnpFuelDatabaseMigrations.MIGRATION_33_34).use { db ->
                db.query("SELECT place_id, generation, groups_json, sources_json FROM charging_snapshot_place").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("11111111-1111-4111-8111-111111111111", it.getString(0))
                    assertEquals(4L, it.getLong(1))
                    assertEquals("""[{"id":"group"}]""", it.getString(2))
                    assertTrue(it.isNull(3))
                }
                db.query("SELECT source_revision, place_count, state FROM charging_snapshot_meta").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("rev-1", it.getString(0))
                    assertEquals(1L, it.getLong(1))
                    assertEquals("COMPLETE", it.getString(2))
                }
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
