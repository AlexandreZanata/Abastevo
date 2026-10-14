package com.anpfuel.data.local

import androidx.room.testing.MigrationTestHelper
import androidx.sqlite.db.framework.FrameworkSQLiteOpenHelperFactory
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class V12ToV13DatabaseMigrationTest {
 @get:Rule val helper=MigrationTestHelper(InstrumentationRegistry.getInstrumentation(),AnpFuelDatabase::class.java.canonicalName,FrameworkSQLiteOpenHelperFactory())
 @Test fun preservesAddressOnlyCatalogAndAddsHonestEmptyAddress() {
   val context=InstrumentationRegistry.getInstrumentation().targetContext
   val name="community-address-v12-v13"
   context.deleteDatabase(name)
   try {
     helper.createDatabase(name,12).apply {
       execSQL("INSERT INTO server_station_cache(station_id,display_name,location_quality,position,saved_at_millis) VALUES('123e4567-e89b-12d3-a456-426614174000','Synthetic station','unknown',0,123)")
       close()
     }
     helper.runMigrationsAndValidate(name,13,true,AnpFuelDatabaseMigrations.MIGRATION_12_13).apply {
       query("SELECT display_name,latitude,longitude,address_json FROM server_station_cache").use {
         assertTrue(it.moveToFirst());assertEquals("Synthetic station",it.getString(0));assertTrue(it.isNull(1));assertTrue(it.isNull(2));assertEquals("{}",it.getString(3))
       };close()
     }
   } finally {context.deleteDatabase(name)}
 }
}
