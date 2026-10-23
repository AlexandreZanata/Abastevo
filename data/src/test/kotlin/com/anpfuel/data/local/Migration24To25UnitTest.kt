package com.anpfuel.data.local

import androidx.sqlite.db.SupportSQLiteDatabase
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class Migration24To25UnitTest {
    @Test fun `migration only creates owner-scoped erasure table`() {
        val db = mockk<SupportSQLiteDatabase>(relaxed = true)
        val statements = mutableListOf<String>()
        val sql = slot<String>()
        every { db.execSQL(capture(sql)) } answers { statements.add(sql.captured); Unit }
        AnpFuelDatabaseMigrations.MIGRATION_24_25.migrate(db)
        assertEquals(1, statements.size)
        val statement = statements.single()
        assertTrue(statement.contains("CREATE TABLE IF NOT EXISTS `ledger_erased`"))
        assertTrue(statement.contains("PRIMARY KEY(`owner_account`, `origin`, `dataset`, `record_id`)"))
        assertFalse(statement.contains("DROP"))
        assertFalse(statement.contains("DELETE"))
    }
}
