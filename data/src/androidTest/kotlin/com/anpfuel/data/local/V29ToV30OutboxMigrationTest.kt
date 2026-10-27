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
class V29ToV30OutboxMigrationTest {
    @get:Rule val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java.canonicalName,
        FrameworkSQLiteOpenHelperFactory(),
    )

    @Test fun upgradeKeepsQueuedCommandClaimable() {
        val name = "driver-outbox-migrate-29-30"
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        context.deleteDatabase(name)
        try {
            helper.createDatabase(name, 29).apply {
                execSQL(
                    "INSERT INTO driver_local_command(owner_account, origin, operation_id, kind, body_sha256, expected_revision, state, attempts, next_eligible_millis, last_error, updated_at_millis) " +
                        "VALUES ('owner-a', 'app', 'op1', 'EARNING_SAVE', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 0, 'PENDING', 0, 10, NULL, 10)",
                )
                close()
            }
            helper.runMigrationsAndValidate(name, 30, true, AnpFuelDatabaseMigrations.MIGRATION_29_30).apply {
                query("SELECT operation_id, state, lease_until_millis FROM driver_local_command WHERE operation_id='op1'").use {
                    assertTrue(it.moveToFirst())
                    assertEquals("PENDING", it.getString(1))
                    assertEquals(0L, it.getLong(2))
                }
                close()
            }
        } finally {
            context.deleteDatabase(name)
        }
    }
}
