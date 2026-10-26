package com.anpfuel.data.local

import androidx.room.Room
import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Canonical v17 cache survives the additive prefix-index upgrade and Room reopen. */
@RunWith(AndroidJUnit4::class)
class CatalogMigration17To18Test {
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    @get:Rule val helper = MigrationTestHelper(InstrumentationRegistry.getInstrumentation(), AnpFuelDatabase::class.java)

    @Test fun upgradeRetainsSnapshotsCatalogAndAnpWithIndexedLookup() {
        val name = "mex-catalog-migration-17-18"
        try {
            helper.createDatabase(name, 17).apply {
                execSQL("INSERT INTO municipality_catalog (id, ibge_code, state, municipality, normalized_name) VALUES ('synthetic-city', '3550308', 'SP', 'Synthetic city', 'synthetic city')")
                execSQL("INSERT INTO survey_week (id, start_date, end_date, summary_imported_at) VALUES ('mex-week', '2026-01-01', '2026-01-07', 1000)")
                execSQL("INSERT INTO community_feed_cache VALUES ('scope', 'FEED', 'https://synthetic.invalid', 'SP', '3550308', 'ETHANOL', 'RECENT', '{}', 1000, 2000)")
                close()
            }
            helper.runMigrationsAndValidate(name, 18, true, AnpFuelDatabaseMigrations.MIGRATION_17_18).close()
            val reopened = Room.databaseBuilder(context, AnpFuelDatabase::class.java, name)
                .addMigrations(
                    AnpFuelDatabaseMigrations.MIGRATION_18_19,
                    AnpFuelDatabaseMigrations.MIGRATION_19_20,
                    AnpFuelDatabaseMigrations.MIGRATION_20_21,
                    AnpFuelDatabaseMigrations.MIGRATION_21_22,
                    AnpFuelDatabaseMigrations.MIGRATION_22_23,
                    AnpFuelDatabaseMigrations.MIGRATION_23_24,
                    AnpFuelDatabaseMigrations.MIGRATION_24_25,
                    AnpFuelDatabaseMigrations.MIGRATION_25_26,
                    AnpFuelDatabaseMigrations.MIGRATION_26_27,
                    AnpFuelDatabaseMigrations.MIGRATION_27_28,
                ).build()
            try {
                val db = reopened.openHelper.readableDatabase
                db.query("SELECT payload_json FROM community_feed_cache WHERE `key` = 'scope'").use {
                    assertTrue(it.moveToFirst()); assertEquals("{}", it.getString(0))
                }
                db.query("SELECT COUNT(*) FROM survey_week WHERE id = 'mex-week'").use {
                    assertTrue(it.moveToFirst()); assertEquals(1, it.getInt(0))
                }
                db.query("SELECT ibge_code FROM municipality_catalog WHERE normalized_name >= 'synthetic' AND normalized_name < 'synthetid'").use {
                    assertTrue(it.moveToFirst()); assertEquals("3550308", it.getString(0))
                }
                db.query("EXPLAIN QUERY PLAN SELECT id FROM municipality_catalog WHERE normalized_name >= 'synthetic' AND normalized_name < 'synthetid'").use {
                    assertTrue(it.moveToFirst()); assertTrue(it.getString(3).contains("idx_municipality_catalog_normalized"))
                }
            } finally { reopened.close() }
        } finally { context.deleteDatabase(name) }
    }
}
