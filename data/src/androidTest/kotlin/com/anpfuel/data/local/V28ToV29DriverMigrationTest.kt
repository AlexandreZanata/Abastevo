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
class V28ToV29DriverMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradePreservesLedgerVehicleAndMapRows() {
        val name = "driver-migrate-28-29"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 28).apply {
                execSQL(
                    "INSERT INTO ledger_draft(draft_id, owner_account, origin, dataset, record_id, expected_revision, body_json, updated_at_millis, context) " +
                        "VALUES ('d1', 'owner-a', 'app', 'refuel', 'r1', 1, '{}', 10, NULL)",
                )
                execSQL(
                    "INSERT INTO vehicle(id, display_name, price_source_mode, price_drop_alert_enabled, sort_order, propulsion) " +
                        "VALUES ('v1', 'Car', 'STATION', 0, 0, 'COMBUSTION')",
                )
                execSQL(
                    "INSERT INTO map_saved_place(place_id, owner_account, origin, kind, name, lat_micro, lon_micro, station_id, source_kind, source_id, note, created_at_millis) " +
                        "VALUES ('p1', 'owner-a', 'app', 'FAVORITE', 'Place', 1, 2, NULL, 'MANUAL', NULL, NULL, 13)",
                )
                close()
            }
            helper.runMigrationsAndValidate(name, 29, true, AnpFuelDatabaseMigrations.MIGRATION_28_29).apply {
                query("SELECT draft_id, owner_account, body_json FROM ledger_draft").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("d1", it.getString(0))
                    assertEquals("owner-a", it.getString(1))
                    assertEquals("{}", it.getString(2))
                }
                query("SELECT id, display_name FROM vehicle").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("v1", it.getString(0))
                    assertEquals("Car", it.getString(1))
                }
                query("SELECT place_id FROM map_saved_place").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("p1", it.getString(0))
                }
                query("SELECT count(*) FROM driver_draft").use {
                    assertTrue(it.moveToFirst())
                    assertEquals(0, it.getInt(0))
                }
                close()
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
