package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * PRM09-T01 schema-12 upgrade guard: additive premium tables and indexes
 * only; no existing table is touched and free vehicle rows survive.
 */
class Migration11To12UnitTest {

    @Test
    fun `migration creates premium outbox and cursor only`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_11_12.migrate(db)

        assertTrue(
            statements.any {
                it.contains("CREATE TABLE") && it.contains("`premium_outbox`") &&
                    it.contains("`owner_account`") && it.contains("`origin`") &&
                    it.contains("`command_id`")
            },
        )
        assertTrue(
            statements.any {
                it.contains("index_premium_outbox_owner_account_state_next_eligible_tick") &&
                    it.contains("`owner_account`")
            },
        )
        assertTrue(
            statements.any {
                it.contains("index_premium_outbox_owner_account_origin_dataset") &&
                    it.contains("`dataset`")
            },
        )
        assertTrue(
            statements.any {
                it.contains("CREATE TABLE") && it.contains("`premium_cursor`") &&
                    it.contains("PRIMARY KEY(`owner_account`, `origin`, `dataset`)")
            },
        )
        verify(exactly = 0) { db.execSQL(match { it.contains("DROP TABLE") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("DELETE FROM") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("ALTER TABLE `vehicle`") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("ALTER TABLE vehicle") }) }
    }
}
