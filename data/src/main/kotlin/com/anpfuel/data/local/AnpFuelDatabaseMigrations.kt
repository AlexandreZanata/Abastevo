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
}
