package com.anpfuel.data.local

import androidx.room.Room
import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class V34ToV35ChargingProfileMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradeKeepsLegacyRowsAndIsolatesEqualNationalIdentity() = runBlocking {
        val name = "charging-profile-34-35"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val legacy = "ocm-br-20261021-v1"
        val national = "charging-br-national-v1"
        val placeId = "11111111-1111-4111-8111-111111111111"
        val generation = 4L
        val sources = """[{"source":"OSM","external_id":"node:7"}]"""
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 34).use { db ->
                db.execSQL(
                    """
                    INSERT INTO charging_snapshot_place
                    (place_id, entity_key, external_id, generation, profile, source_revision, origin, license,
                     name, country, latitude, longitude, public_access, reported_points, groups_json, tombstone,
                     saved_at_millis, sources_json)
                    VALUES (?, ?, '7', ?, ?, 'rev-1', 'https://example.test', 'legacy',
                            'Synthetic place', 'BR', '-23.5', '-46.6', 'PUBLIC', 1, '[]', 0, 1000, ?)
                    """.trimIndent(),
                    arrayOf(placeId, "a".repeat(64), generation, legacy, sources),
                )
            }
            helper.runMigrationsAndValidate(name, 35, true, AnpFuelDatabaseMigrations.MIGRATION_34_35).close()
            val db = Room.databaseBuilder(context, AnpFuelDatabase::class.java, name).build()
            try {
                val dao = db.chargingSnapshotDao()
                val legacyRow = requireNotNull(dao.findPlace(placeId, legacy, generation))
                assertEquals("Synthetic place", legacyRow.name)
                assertEquals(sources, legacyRow.sourcesJson)
                assertNull(legacyRow.catalogConfidence)
                assertNull(legacyRow.linkedStationId)

                val nationalRow = legacyRow.copy(
                    profile = national, name = "", catalogConfidence = "SINGLE_SOURCE", linkedStationId = placeId,
                )
                dao.upsertPlaces(listOf(nationalRow))
                assertEquals(nationalRow, dao.findPlace(placeId, national, generation))
                assertEquals(legacyRow, dao.findPlace(placeId, legacy, generation))
                assertEquals(1L, dao.countPlaces(legacy, generation))
                assertEquals(1L, dao.countPlaces(national, generation))

                dao.markTombstones(listOf(placeId), national, generation)
                assertNull(dao.findPlace(placeId, national, generation))
                assertNotNull(dao.findPlace(placeId, legacy, generation))
                assertEquals(1L, dao.countPlaces(legacy, generation))
                assertEquals(0L, dao.countPlaces(national, generation))
            } finally {
                db.close()
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
