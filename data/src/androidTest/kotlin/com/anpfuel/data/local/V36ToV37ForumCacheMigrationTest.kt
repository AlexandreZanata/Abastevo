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
class V36ToV37ForumCacheMigrationTest {

    @get:Rule
    val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test
    fun upgradeCreatesForumCacheTableAndPreservesExistingData() {
        val name = "forum-cache-36-37"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 36).use { db ->
                // Verify DB at v36 can insert into existing table
                db.execSQL(
                    """
                    INSERT INTO survey_week (id, start_date, end_date, summary_imported_at)
                    VALUES ('2026-W45', '2026-11-01', '2026-11-07', 1000)
                    """.trimIndent(),
                )
            }
            helper.runMigrationsAndValidate(name, 37, true, AnpFuelDatabaseMigrations.MIGRATION_36_37).use { db ->
                // Verify existing table preserved
                db.query("SELECT id FROM survey_week WHERE id = '2026-W45'").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("2026-W45", it.getString(0))
                }
                // Verify forum_cache table exists and is operational
                db.execSQL(
                    """
                    INSERT INTO forum_cache (origin, `key`, kind, payload_json, fetched_at_millis, payload_bytes)
                    VALUES ('https://teste.abastevo.com.br', 'categories', 'CATEGORIES', '{}', 1000, 2)
                    """.trimIndent(),
                )
                db.query("SELECT origin, `key`, kind FROM forum_cache WHERE `key` = 'categories'").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("https://teste.abastevo.com.br", it.getString(0))
                    assertEquals("categories", it.getString(1))
                    assertEquals("CATEGORIES", it.getString(2))
                }
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
