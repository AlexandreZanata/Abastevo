package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P24-T01 schema-6 upgrade guard (unit side).
 *
 * The device-side `V6ToV7DatabaseMigrationTest` proves catalog rows
 * survive and FTS searches on old SQLite; here we prove
 * MIGRATION_6_7 rebuilds the derived FTS index with the plain
 * `unicode61` tokenizer plus the pre-normalized ASCII column (any
 * `remove_diacritics` argument crashes API 26), reissues the sync
 * triggers plus a rebuild, and never touches the
 * `municipality_catalog` content.
 */
class Migration6To7UnitTest {

    @Test
    fun `migration rebuilds fts with portable tokenizer`() {
        val db: SupportSQLiteDatabase = mockk(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers {
            statements += sql.captured
            Unit
        }

        AnpFuelDatabaseMigrations.MIGRATION_6_7.migrate(db)

        assertTrue(statements.any { it.contains("DROP TABLE IF EXISTS `municipality_fts`") })
        assertTrue(
            statements.any {
                it.contains("CREATE VIRTUAL TABLE IF NOT EXISTS `municipality_fts`") &&
                    it.contains("`normalized_name` TEXT NOT NULL") &&
                    it.contains("tokenize=unicode61,")
            },
        )
        assertTrue(
            statements.any {
                it.contains("INSERT INTO municipality_fts(municipality_fts) VALUES('rebuild')")
            },
        )
        assertTrue(statements.none { it.contains("remove_diacritics") })
        verify(exactly = 0) { db.execSQL(match { it.contains("DROP TABLE IF EXISTS `municipality_catalog`") }) }
    }
}
