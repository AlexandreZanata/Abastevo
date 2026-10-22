package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * MEX-05 normalized-prefix index upgrade guard: exactly one additive
 * CREATE INDEX on municipality_catalog(normalized_name); no table rewrite,
 * no data movement, no other table touched.
 */
class Migration17To18UnitTest {

    @Test
    fun `migration adds only the normalized prefix index`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_17_18.migrate(db)

        assertEquals(1, statements.size)
        assertTrue(
            statements.single().contains("CREATE INDEX IF NOT EXISTS") &&
                statements.single().contains("`idx_municipality_catalog_normalized`") &&
                statements.single().contains("ON `municipality_catalog` (`normalized_name`)"),
        )
        verify(exactly = 0) { db.execSQL(match { it.contains("DROP TABLE") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("ALTER TABLE") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("DELETE FROM") }) }
        verify(exactly = 0) { db.execSQL(match { it.contains("UPDATE ") }) }
    }
}
