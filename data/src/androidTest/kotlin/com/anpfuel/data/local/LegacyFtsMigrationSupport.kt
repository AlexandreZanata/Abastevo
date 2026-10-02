package com.anpfuel.data.local

import org.junit.Assume.assumeTrue

/**
 * P24-T01 — shared gate for historical migration tests.
 *
 * Every schema before v7 creates the FTS index with
 * `remove_diacritics=2`, which needs SQLite 3.20+ (value 2 needs
 * even newer ICU-backed builds) and throws `unknown tokenizer` on
 * API 26. Those historical databases could never exist on such
 * devices (no version ever opened there), so the upgrade path is
 * vacuous below the probe: skip it and let fresh-install suites
 * prove the floor. Where the probe passes, the test runs fully.
 */
internal object LegacyFtsMigrationSupport {

    fun assumeLegacyFtsCreatable() {
        val supported = runCatching {
            android.database.sqlite.SQLiteDatabase.create(null).apply {
                execSQL(
                    "CREATE VIRTUAL TABLE probe_fts USING " +
                        "FTS4(x, tokenize=unicode61 `remove_diacritics=2`)",
                )
                close()
            }
        }.isSuccess
        assumeTrue(
            "historical schemas require remove_diacritics=2 support on this SQLite",
            supported,
        )
    }
}
