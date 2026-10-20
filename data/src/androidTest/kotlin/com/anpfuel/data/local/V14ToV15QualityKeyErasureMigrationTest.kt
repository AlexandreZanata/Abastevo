package com.anpfuel.data.local

import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class V14ToV15QualityKeyErasureMigrationTest {
    @get:Rule val helper=MigrationTestHelper(InstrumentationRegistry.getInstrumentation(),AnpFuelDatabase::class.java)
    @Test fun priorGateIsUnconfirmedAndCiphertextSurvivesUpgrade() {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val name="quality-v14-v15"
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name,14).apply {
                execSQL("INSERT INTO quality_owner_gate VALUES ('synthetic-owner','synthetic-local','https://synthetic.invalid',0,100,1000)")
                execSQL("INSERT INTO quality_report_outbox VALUES ('synthetic-owner','synthetic-command','synthetic-hash','synthetic-ciphertext',100,900,'QUEUED',0,100,'',0)");close()
            }
            helper.runMigrationsAndValidate(name,15,true,AnpFuelDatabaseMigrations.MIGRATION_14_15).apply {
                query("SELECT key_erased FROM quality_owner_gate").use {assertTrue(it.moveToFirst());assertEquals(0,it.getInt(0))}
                query("SELECT sealed_command FROM quality_report_outbox").use {assertTrue(it.moveToFirst());assertEquals("synthetic-ciphertext",it.getString(0))};close()
            }
        }finally {context.deleteDatabase(name)}
    }
}
