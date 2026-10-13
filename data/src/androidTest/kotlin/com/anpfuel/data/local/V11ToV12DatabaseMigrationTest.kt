package com.anpfuel.data.local

import android.database.sqlite.SQLiteDatabase
import androidx.room.migration.Migration
import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.SupportSQLiteDatabase
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Reconciliation retains the already installed vehicle-photo schema and appends premium state. */
@RunWith(AndroidJUnit4::class)
class V11ToV12DatabaseMigrationTest {
    @get:Rule val helper = MigrationTestHelper(InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName, FrameworkSQLiteOpenHelperFactory())

    private fun seed(db: SupportSQLiteDatabase, withPhoto: Boolean) {
        db.execSQL("""INSERT INTO vehicle(id,display_name,tank_capacity_liters,fuel_product,
            price_source_mode,specific_station_cnpj,price_drop_alert_enabled,sort_order)
            VALUES ('synthetic-car','Synthetic car',50,'GASOLINE_REGULAR','CITY_AVERAGE',NULL,0,0)""")
        if (withPhoto) db.execSQL("UPDATE vehicle SET photo_path='synthetic-private.jpg' WHERE id='synthetic-car'")
        db.execSQL("""INSERT INTO contribution_outbox(command_id,kind,payload,revision,state,attempts,next_eligible_tick,nonce)
            VALUES ('synthetic-legacy','contribution.submit','{}',3,'FAILED',2,123,'')""")
    }

    @Test fun installedElevenRetainsVehiclePhotoAndLegacyOutbox() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val name = "reconcile-v11-v12"
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name,11).apply { seed(this,true); close() }
            helper.runMigrationsAndValidate(name,12,true,AnpFuelDatabaseMigrations.MIGRATION_11_12).apply {
                query("SELECT photo_path,tank_capacity_liters FROM vehicle WHERE id='synthetic-car'").use {
                    assertTrue(it.moveToFirst()); assertEquals("synthetic-private.jpg",it.getString(0)); assertEquals(50.0,it.getDouble(1),0.0)
                }
                query("SELECT revision,attempts FROM contribution_outbox WHERE command_id='synthetic-legacy'").use {
                    assertTrue(it.moveToFirst()); assertEquals(3,it.getInt(0)); assertEquals(2,it.getInt(1))
                }
                query("SELECT count(*) FROM premium_outbox").use { assertTrue(it.moveToFirst()); assertEquals(0,it.getInt(0)) }
                query("SELECT count(*) FROM premium_cursor").use { assertTrue(it.moveToFirst()); assertEquals(0,it.getInt(0)) }
                close()
            }
        } finally { context.deleteDatabase(name) }
    }

    @Test fun tenUpgradesThroughUnchangedPhotoMigration() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val name = "reconcile-v10-v12"
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name,10).apply { seed(this,false); close() }
            helper.runMigrationsAndValidate(name,12,true,AnpFuelDatabaseMigrations.MIGRATION_10_11,
                AnpFuelDatabaseMigrations.MIGRATION_11_12).apply {
                query("SELECT photo_path FROM vehicle WHERE id='synthetic-car'").use { assertTrue(it.moveToFirst()); assertTrue(it.isNull(0)) }
                close()
            }
        } finally { context.deleteDatabase(name) }
    }

    @Test fun failedUpgradeRollsBackAndCanRecover() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val name = "reconcile-v12-recovery"
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name,11).apply { seed(this,true); close() }
            val injected = object : Migration(11,12) {
                override fun migrate(db: SupportSQLiteDatabase) {
                    AnpFuelDatabaseMigrations.MIGRATION_11_12.migrate(db)
                    db.execSQL("INSERT INTO reconcile_expected_missing_table(id) VALUES(1)")
                }
            }
            try {
                helper.runMigrationsAndValidate(name,12,true,injected).close()
                fail("injected SQL failure must abort migration")
            } catch (error: android.database.sqlite.SQLiteException) {
                assertTrue(error.message.orEmpty().contains("reconcile_expected_missing_table"))
            }
            SQLiteDatabase.openDatabase(context.getDatabasePath(name).path,null,SQLiteDatabase.OPEN_READONLY).use { db ->
                assertEquals(11,db.version)
                db.rawQuery("SELECT count(*) FROM sqlite_master WHERE name='premium_outbox'",null).use {
                    assertTrue(it.moveToFirst()); assertEquals(0,it.getInt(0))
                }
            }
            helper.runMigrationsAndValidate(name,12,true,AnpFuelDatabaseMigrations.MIGRATION_11_12).close()
        } finally { context.deleteDatabase(name) }
    }
}
