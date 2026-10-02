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
 * P24-T01 — schema-6 upgrade keeps catalog rows and restores an
 * FTS index that old SQLite can open.
 *
 * Any `remove_diacritics` argument needs SQLite 3.20+ (value 2
 * additionally needs ICU) and crashes table creation on API 26
 * (`unknown tokenizer`); the migration rebuilds the derived index
 * with the plain `unicode61` tokenizer plus the pre-normalized
 * ASCII column. Afterwards "SAO" (normalized) and "SÃO" (raw) must
 * both match "SÃO PAULO".
 */
@RunWith(AndroidJUnit4::class)
class V6ToV7DatabaseMigrationTest {

    private val testDb = "v6-fts-tokenizer-migration"

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
    fun migrateFromV6PreservesCatalogAndRestoresFts() {
        // A v6 database can only exist where its `remove_diacritics=2`
        // tokenizer could be created (P24-T01 probe: fails on SQLite
        // 3.18/API 26, works on current releases). Skip the upgrade
        // path where no v6 history can exist; fresh installs there
        // are covered by the v7 creation suites.
        LegacyFtsMigrationSupport.assumeLegacyFtsCreatable()
        helper.createDatabase(testDb, 6).apply {
            execSQL(
                """
                INSERT INTO municipality_catalog (
                    id, ibge_code, state, municipality, normalized_name, anp_alias
                ) VALUES (
                    'sp-3550308', '3550308', 'SP', 'SÃO PAULO', 'SAO PAULO', NULL
                )
                """.trimIndent(),
            )
            close()
        }

        helper.runMigrationsAndValidate(
            testDb,
            7,
            true,
            AnpFuelDatabaseMigrations.MIGRATION_6_7,
        ).apply {
            query("SELECT municipality FROM municipality_catalog").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("SÃO PAULO", cursor.getString(0))
            }
            query(
                "SELECT municipality FROM municipality_fts WHERE municipality_fts MATCH 'SAO*'",
            ).use { cursor ->
                assertTrue(
                    "expected SÃO PAULO in FTS results after 6 to 7",
                    cursor.moveToFirst(),
                )
                assertEquals("SÃO PAULO", cursor.getString(0))
            }
            query(
                "SELECT municipality FROM municipality_fts WHERE municipality_fts MATCH 'SÃO*'",
            ).use { cursor ->
                assertTrue(
                    "expected accented query to match raw column after 6 to 7",
                    cursor.moveToFirst(),
                )
                assertEquals("SÃO PAULO", cursor.getString(0))
            }
            close()
        }
    }
}
