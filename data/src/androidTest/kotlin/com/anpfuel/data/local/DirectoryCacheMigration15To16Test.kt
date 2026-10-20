package com.anpfuel.data.local

import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.entity.ServerCatalogMetaEntity
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * P27 15→16 scoped-catalog upgrade: device-only proof that the rebuild
 * keeps catalog rows (backfilled from the stored cursor scope), keeps
 * unscoped details, and leaves ANP tables and their rows untouched.
 * Runs on device/emulator; JVM suites only assert the emitted SQL.
 */
@RunWith(AndroidJUnit4::class)
class DirectoryCacheMigration15To16Test {

    @get:Rule
    val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java,
    )

    @Test
    fun migrate15To16_backfillsScopeAndPreservesAnpRows() {
        helper.createDatabase(TEST_DB, 15).apply {
            execSQL(
                "INSERT INTO server_station_cache " +
                    "(station_id, display_name, location_quality, cnpj_normalized, municipality_code, " +
                    "state, position, saved_at_millis, address_json) VALUES " +
                    "('uuid-a', 'Posto Alfa', 'reviewed', '04218406000104', '3550308', 'SP', 0, 1000, '{}')," +
                    "('uuid-c', 'Posto Gama', 'unknown', '12ABC34501DE35', '3550308', 'SP', 2147483647, 1000, '{}')",
            )
            execSQL("INSERT INTO server_catalog_meta VALUES ('page', 'cursor-old', 1000)")
            execSQL("INSERT INTO survey_week (id, start_date, end_date, summary_imported_at) VALUES ('w1', '2026-01-01', '2026-01-07', 1000)")
            execSQL(
                "INSERT INTO station_price " +
                    "(id, survey_week_id, cnpj, address, municipality, state, fuel_product, price) VALUES " +
                    "('anp-1', 'w1', '04218406000104', 'RUA SÃO JOÃO', 'Sao Paulo', 'SP', 'GASOLINE_REGULAR', 5.99)",
            )
            close()
        }

        helper.runMigrationsAndValidate(TEST_DB, 16, true, AnpFuelDatabaseMigrations.MIGRATION_15_16).apply {
            val scopes = mutableMapOf<String, String>()
            query("SELECT station_id, scope_key FROM server_station_cache").use { cursor ->
                while (cursor.moveToNext()) scopes[cursor.getString(0)] = cursor.getString(1)
            }
            assertEquals(ServerCatalogMetaEntity.PAGE_KEY, scopes["uuid-a"])
            assertEquals("", scopes["uuid-c"])
            query("SELECT `key`, next_cursor FROM server_catalog_meta").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("page", cursor.getString(0))
                assertEquals("cursor-old", cursor.getString(1))
            }
            query("SELECT price FROM station_price WHERE id = 'anp-1'").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(5.99, cursor.getDouble(0), 0.0)
            }
            query("SELECT address_search FROM station_price WHERE id = 'anp-1'").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals("rua são joão", cursor.getString(0))
            }
            // Same station coexists in two scopes after the upgrade.
            execSQL(
                "INSERT INTO server_station_cache " +
                    "(station_id, display_name, location_quality, position, saved_at_millis, scope_key) VALUES " +
                    "('uuid-a', 'Posto Alfa', 'reviewed', 0, 2000, 'city:3550308')",
            )
            query("SELECT COUNT(*) FROM server_station_cache WHERE station_id = 'uuid-a'").use { cursor ->
                assertTrue(cursor.moveToFirst())
                assertEquals(2, cursor.getInt(0))
            }
            close()
        }
    }

    companion object {
        private const val TEST_DB = "migration-15-16-test"
    }
}
