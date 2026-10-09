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
class V8ToV9PhotoOutboxMigrationTest {
    @get:Rule val helper = MigrationTestHelper(InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName, FrameworkSQLiteOpenHelperFactory())
    @Test fun legacyPayloadRemainsUnscopedAndPricesSurviveMigration() {
        val name = "v8-v9-photo-outbox-test"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name,8).apply {
                execSQL("""INSERT INTO contribution_outbox(command_id,kind,payload,revision,state,attempts,next_eligible_tick,nonce)
                    VALUES ('legacy','contribution.submit','{"amount_milli_brl":5890}',3,'FAILED',2,123,'')""")
                close()
            }
            helper.runMigrationsAndValidate(name,9,true,AnpFuelDatabaseMigrations.MIGRATION_8_9).apply {
                query("SELECT payload,revision,attempts,observation_id,remote_status FROM contribution_outbox WHERE command_id='legacy'").use {
                    assertTrue(it.moveToFirst()); assertEquals("{\"amount_milli_brl\":5890}",it.getString(0))
                    assertEquals(3,it.getInt(1)); assertEquals(2,it.getInt(2)); assertTrue(it.isNull(3)); assertTrue(it.isNull(4))
                }
                query("SELECT count(*) FROM photo_upload_sessions").use { assertTrue(it.moveToFirst()); assertEquals(0,it.getInt(0)) }
                close()
            }
        } finally { context.deleteDatabase(name) }
    }
}
