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
class V30ToV31WorkMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradeKeepsShiftAndAddsWorkTables() {
        val name = "driver-work-migrate-30-31"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 30).apply {
                execSQL("INSERT INTO driver_shift(owner_account,origin,shift_id,vehicle_id,timezone,state,version,active_millis,manually_reviewed_millis,created_at_millis,updated_at_millis) VALUES('o','android','s','v','UTC','CLOSED',1,0,0,1,1)")
                close()
            }
            helper.runMigrationsAndValidate(name, 31, true, AnpFuelDatabaseMigrations.MIGRATION_30_31).apply {
                query("SELECT shift_id,start_odometer_metres FROM driver_shift").use {
                    assertTrue(it.moveToFirst()); assertEquals("s", it.getString(0)); assertTrue(it.isNull(1))
                }
                listOf("driver_payout", "driver_cost_setting", "driver_gps_observation").forEach { table ->
                    query("SELECT count(*) FROM $table").use { assertTrue(it.moveToFirst()); assertEquals(0, it.getInt(0)) }
                }
                close()
            }
        } finally { context.deleteDatabase(name) }
    }
}
