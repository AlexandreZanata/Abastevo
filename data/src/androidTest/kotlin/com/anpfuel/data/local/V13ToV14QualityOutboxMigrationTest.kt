package com.anpfuel.data.local

import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Compiled here; execution remains part of the deferred native batch. */
@RunWith(AndroidJUnit4::class)
class V13ToV14QualityOutboxMigrationTest {
    @get:Rule val helper=MigrationTestHelper(InstrumentationRegistry.getInstrumentation(),AnpFuelDatabase::class.java)
    @Test fun priorCatalogSurvivesAndPrivateQueueStartsEmpty() {
        val context=InstrumentationRegistry.getInstrumentation().targetContext
        val name="quality-v13-v14"
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name,13).apply {
                execSQL("INSERT INTO survey_week(id,start_date,end_date,summary_imported_at) VALUES('synthetic-week','2026-10-01','2026-10-07',123)");close()
            }
            helper.runMigrationsAndValidate(name,14,true,AnpFuelDatabaseMigrations.MIGRATION_13_14).apply {
                query("SELECT count(*) FROM survey_week").use {assertTrue(it.moveToFirst());assertEquals(1,it.getInt(0))}
                query("SELECT count(*) FROM quality_report_outbox").use {assertTrue(it.moveToFirst());assertEquals(0,it.getInt(0))}
                close()
            }
        }finally {context.deleteDatabase(name)}
    }
}
