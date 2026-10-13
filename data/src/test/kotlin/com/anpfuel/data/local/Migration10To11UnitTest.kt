package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * Schema-11 upgrade guard (unit side): the local-only vehicle photo
 * column lands nullable with no data movement, and no table content
 * is touched.
 */
class Migration10To11UnitTest {

    @Test
    fun `migration adds nullable vehicle photo column only`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_10_11.migrate(db)

        assertTrue(
            statements.any {
                it.contains("ALTER TABLE") &&
                    it.contains("`vehicle`") &&
                    it.contains("`photo_path`") &&
                    it.contains("TEXT") &&
                    it.contains("DEFAULT NULL")
            },
        )
        verify(exactly = 0) { db.execSQL(match { it.contains("DROP TABLE") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("DELETE FROM") }) }
    }
}
