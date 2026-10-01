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
 * P10-T05 — schema-5 upgrade preserves cache/vehicles/history and adds the
 * additive contribution outbox. ANP screens keep working unchanged.
 */
@RunWith(AndroidJUnit4::class)
class V5ToV6DatabaseMigrationTest {

    private val testDb = "v5-user-data-migration"

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
    fun migrateFromV5PreservesCacheAndAddsOutbox() {
        helper.createDatabase(testDb, 5).apply {
            execSQL(
                """
                INSERT INTO backend_price_cache (
                    key, station_id, fuel_filter, payload_json, source, version,
                    fetched_at_millis, expires_at_millis
                ) VALUES (
                    'station|ALL', 'd6c74c23-63db-4c24-a2e5-408cb23bad26', 'ALL',
                    '{"items": []}', 'backend', 'v1', 1000000, 1060000
                )
                """.trimIndent(),
            )
            close()
        }

        helper.runMigrationsAndValidate(
            testDb,
            6,
            true,
            AnpFuelDatabaseMigrations.MIGRATION_5_6,
        ).apply {
            query("SELECT COUNT(*) FROM backend_price_cache").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(1, cursor.getInt(0))
            }
            query(
                """
                SELECT name FROM sqlite_master
                WHERE type = 'table' AND name = 'contribution_outbox'
                """.trimIndent(),
            ).use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("contribution_outbox", cursor.getString(0))
            }
            close()
        }
    }
}
