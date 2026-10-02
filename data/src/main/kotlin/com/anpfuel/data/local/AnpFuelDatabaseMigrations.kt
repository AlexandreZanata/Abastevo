package com.anpfuel.data.local

import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase

object AnpFuelDatabaseMigrations {

    val MIGRATION_1_2: Migration = object : Migration(1, 2) {
        override fun migrate(db: SupportSQLiteDatabase) {
            db.execSQL(
                """
                CREATE VIRTUAL TABLE IF NOT EXISTS `municipality_fts` USING FTS4(
                    `municipality` TEXT NOT NULL,
                    `state` TEXT NOT NULL,
                    tokenize=unicode61 `remove_diacritics=2`,
                    content=`average_price`
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_BEFORE_UPDATE
                BEFORE UPDATE ON `average_price`
                BEGIN
                    DELETE FROM `municipality_fts` WHERE `docid`=OLD.`rowid`;
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_BEFORE_DELETE
                BEFORE DELETE ON `average_price`
                BEGIN
                    DELETE FROM `municipality_fts` WHERE `docid`=OLD.`rowid`;
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_AFTER_UPDATE
                AFTER UPDATE ON `average_price`
                BEGIN
                    INSERT INTO `municipality_fts`(`docid`, `municipality`, `state`)
                    VALUES (NEW.`rowid`, NEW.`municipality`, NEW.`state`);
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_AFTER_INSERT
                AFTER INSERT ON `average_price`
                BEGIN
                    INSERT INTO `municipality_fts`(`docid`, `municipality`, `state`)
                    VALUES (NEW.`rowid`, NEW.`municipality`, NEW.`state`);
                END
                """.trimIndent(),
            )
            db.execSQL("INSERT INTO municipality_fts(municipality_fts) VALUES('rebuild')")
        }
    }

    val MIGRATION_2_3: Migration = object : Migration(2, 3) {
        override fun migrate(db: SupportSQLiteDatabase) {
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `municipality_catalog` (
                    `id` TEXT NOT NULL,
                    `ibge_code` TEXT NOT NULL,
                    `state` TEXT NOT NULL,
                    `municipality` TEXT NOT NULL,
                    `normalized_name` TEXT NOT NULL,
                    `anp_alias` TEXT,
                    PRIMARY KEY(`id`)
                )
                """.trimIndent(),
            )
            db.execSQL(
                "CREATE UNIQUE INDEX IF NOT EXISTS `index_municipality_catalog_state_normalized_name` " +
                    "ON `municipality_catalog` (`state`, `normalized_name`)",
            )
            db.execSQL(
                "CREATE UNIQUE INDEX IF NOT EXISTS `index_municipality_catalog_ibge_code` " +
                    "ON `municipality_catalog` (`ibge_code`)",
            )

            dropAveragePriceFtsTriggers(db)
            db.execSQL("DROP TABLE IF EXISTS `municipality_fts`")
            createCatalogFtsTable(db)
            createCatalogFtsTriggers(db)
        }

        private fun dropAveragePriceFtsTriggers(db: SupportSQLiteDatabase) {
            db.execSQL("DROP TRIGGER IF EXISTS room_fts_content_sync_municipality_fts_BEFORE_UPDATE")
            db.execSQL("DROP TRIGGER IF EXISTS room_fts_content_sync_municipality_fts_BEFORE_DELETE")
            db.execSQL("DROP TRIGGER IF EXISTS room_fts_content_sync_municipality_fts_AFTER_UPDATE")
            db.execSQL("DROP TRIGGER IF EXISTS room_fts_content_sync_municipality_fts_AFTER_INSERT")
        }

        private fun createCatalogFtsTable(db: SupportSQLiteDatabase) {
            db.execSQL(
                """
                CREATE VIRTUAL TABLE IF NOT EXISTS `municipality_fts` USING FTS4(
                    `municipality` TEXT NOT NULL,
                    `state` TEXT NOT NULL,
                    tokenize=unicode61 `remove_diacritics=2`,
                    content=`municipality_catalog`
                )
                """.trimIndent(),
            )
        }

        private fun createCatalogFtsTriggers(db: SupportSQLiteDatabase) {
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_BEFORE_UPDATE
                BEFORE UPDATE ON `municipality_catalog`
                BEGIN
                    DELETE FROM `municipality_fts` WHERE `docid`=OLD.`rowid`;
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_BEFORE_DELETE
                BEFORE DELETE ON `municipality_catalog`
                BEGIN
                    DELETE FROM `municipality_fts` WHERE `docid`=OLD.`rowid`;
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_AFTER_UPDATE
                AFTER UPDATE ON `municipality_catalog`
                BEGIN
                    INSERT INTO `municipality_fts`(`docid`, `municipality`, `state`)
                    VALUES (NEW.`rowid`, NEW.`municipality`, NEW.`state`);
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_AFTER_INSERT
                AFTER INSERT ON `municipality_catalog`
                BEGIN
                    INSERT INTO `municipality_fts`(`docid`, `municipality`, `state`)
                    VALUES (NEW.`rowid`, NEW.`municipality`, NEW.`state`);
                END
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_3_4: Migration = object : Migration(3, 4) {
        override fun migrate(db: SupportSQLiteDatabase) {
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `vehicle` (
                    `id` TEXT NOT NULL,
                    `display_name` TEXT NOT NULL,
                    `tank_capacity_liters` REAL NOT NULL,
                    `fuel_product` TEXT NOT NULL,
                    `price_source_mode` TEXT NOT NULL,
                    `specific_station_cnpj` TEXT,
                    `price_drop_alert_enabled` INTEGER NOT NULL DEFAULT 0,
                    `sort_order` INTEGER NOT NULL DEFAULT 0,
                    PRIMARY KEY(`id`)
                )
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_4_5: Migration = object : Migration(4, 5) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // P10-T08: no secondary index on purpose. Every cache read
            // is by primary `key` (or full-table expiry sweep), so an
            // undeclared station_id index only drifts migrated installs
            // away from the entity schema and fails Room validation
            // on device. Fresh installs never had it.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `backend_price_cache` (
                    `key` TEXT NOT NULL,
                    `station_id` TEXT NOT NULL,
                    `fuel_filter` TEXT NOT NULL,
                    `payload_json` TEXT NOT NULL,
                    `source` TEXT NOT NULL,
                    `version` TEXT NOT NULL,
                    `fetched_at_millis` INTEGER NOT NULL,
                    `expires_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`key`)
                )
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_5_6: Migration = object : Migration(5, 6) {
        override fun migrate(db: SupportSQLiteDatabase) {
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `contribution_outbox` (
                    `command_id` TEXT NOT NULL,
                    `kind` TEXT NOT NULL,
                    `payload` TEXT NOT NULL,
                    `revision` INTEGER NOT NULL,
                    `state` TEXT NOT NULL,
                    `attempts` INTEGER NOT NULL,
                    `next_eligible_tick` INTEGER NOT NULL,
                    `nonce` TEXT NOT NULL,
                    PRIMARY KEY(`command_id`)
                )
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_6_7: Migration = object : Migration(6, 7) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // P24-T01: FTS4 `unicode61 remove_diacritics=*` needs
            // SQLite 3.20+ (value 2 additionally needs ICU) and
            // crashes table creation on API 26 (`unknown
            // tokenizer`). Rebuild the derived index with the plain
            // `unicode61` tokenizer plus the pre-normalized ASCII
            // `normalized_name` column, so "SAO*" (normalized) and
            // "SÃO*" (raw) both match on every API level. The index
            // is fully derived from `municipality_catalog` (rebuilt
            // below); catalog rows are never touched.
            db.execSQL("DROP TRIGGER IF EXISTS room_fts_content_sync_municipality_fts_BEFORE_UPDATE")
            db.execSQL("DROP TRIGGER IF EXISTS room_fts_content_sync_municipality_fts_BEFORE_DELETE")
            db.execSQL("DROP TRIGGER IF EXISTS room_fts_content_sync_municipality_fts_AFTER_UPDATE")
            db.execSQL("DROP TRIGGER IF EXISTS room_fts_content_sync_municipality_fts_AFTER_INSERT")
            db.execSQL("DROP TABLE IF EXISTS `municipality_fts`")
            db.execSQL(
                """
                CREATE VIRTUAL TABLE IF NOT EXISTS `municipality_fts` USING FTS4(
                    `municipality` TEXT NOT NULL,
                    `state` TEXT NOT NULL,
                    `normalized_name` TEXT NOT NULL,
                    tokenize=unicode61,
                    content=`municipality_catalog`
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_BEFORE_UPDATE
                BEFORE UPDATE ON `municipality_catalog`
                BEGIN
                    DELETE FROM `municipality_fts` WHERE `docid`=OLD.`rowid`;
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_BEFORE_DELETE
                BEFORE DELETE ON `municipality_catalog`
                BEGIN
                    DELETE FROM `municipality_fts` WHERE `docid`=OLD.`rowid`;
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_AFTER_UPDATE
                AFTER UPDATE ON `municipality_catalog`
                BEGIN
                    INSERT INTO `municipality_fts`(`docid`, `municipality`, `state`, `normalized_name`)
                    VALUES (NEW.`rowid`, NEW.`municipality`, NEW.`state`, NEW.`normalized_name`);
                END
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TRIGGER IF NOT EXISTS room_fts_content_sync_municipality_fts_AFTER_INSERT
                AFTER INSERT ON `municipality_catalog`
                BEGIN
                    INSERT INTO `municipality_fts`(`docid`, `municipality`, `state`, `normalized_name`)
                    VALUES (NEW.`rowid`, NEW.`municipality`, NEW.`state`, NEW.`normalized_name`);
                END
                """.trimIndent(),
            )
            db.execSQL("INSERT INTO municipality_fts(municipality_fts) VALUES('rebuild')")
        }
    }
}
