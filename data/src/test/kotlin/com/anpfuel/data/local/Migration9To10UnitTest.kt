package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * SEC06-C schema-10 upgrade guard (unit side): dispatch and owner
 * indexes land with Room's auto-generated names (validation compares
 * names), and no table content is touched.
 */
class Migration9To10UnitTest {

    @Test
    fun `migration creates dispatch and owner indexes only`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_9_10.migrate(db)

        assertTrue(
            statements.any {
                it.contains("index_contribution_outbox_state_next_eligible_tick") &&
                    it.contains("`state`") && it.contains("`next_eligible_tick`")
            },
        )
        assertTrue(
            statements.any {
                it.contains("index_contribution_outbox_remote_status") &&
                    it.contains("`remote_status`")
            },
        )
        assertTrue(
            statements.any {
                it.contains("index_photo_upload_sessions_owner_scope") &&
                    it.contains("`owner_scope`")
            },
        )
        verify(exactly = 0) { db.execSQL(match { it.contains("DROP TABLE") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("DELETE FROM") }) }
    }
}
