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

    val MIGRATION_7_8: Migration = object : Migration(7, 8) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // P27-T03: additive canonical catalog cache. No existing
            // table is touched; the page set is replaced per fresh
            // traversal so stale scopes never linger.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `server_station_cache` (
                    `station_id` TEXT NOT NULL,
                    `display_name` TEXT NOT NULL,
                    `location_quality` TEXT NOT NULL,
                    `latitude` REAL,
                    `longitude` REAL,
                    `cnpj_normalized` TEXT,
                    `municipality_code` TEXT,
                    `state` TEXT,
                    `current_revision_id` TEXT,
                    `position` INTEGER NOT NULL,
                    `saved_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`station_id`)
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `server_catalog_meta` (
                    `key` TEXT NOT NULL,
                    `next_cursor` TEXT,
                    `saved_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`key`)
                )
                """.trimIndent(),
            )
        }
    }
    val MIGRATION_8_9: Migration = object : Migration(8, 9) {
        override fun migrate(db: SupportSQLiteDatabase) {
            db.execSQL("ALTER TABLE contribution_outbox ADD COLUMN observation_id TEXT DEFAULT NULL")
            db.execSQL("ALTER TABLE contribution_outbox ADD COLUMN remote_status TEXT DEFAULT NULL")
            db.execSQL("ALTER TABLE contribution_outbox ADD COLUMN failure_reason TEXT DEFAULT NULL")
            db.execSQL("""CREATE TABLE IF NOT EXISTS photo_upload_sessions (
                capture_id TEXT NOT NULL PRIMARY KEY, session_id TEXT NOT NULL,
                photo_id TEXT NOT NULL, owner_scope TEXT NOT NULL, origin TEXT NOT NULL,
                captured_at_millis INTEGER NOT NULL, expires_at_millis INTEGER NOT NULL,
                sha256 TEXT NOT NULL, evidence_id TEXT)""".trimIndent())
        }
    }
    val MIGRATION_9_10: Migration = object : Migration(9, 10) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // SEC06-C dispatch/owner indexes (names match Room's
            // auto-generated index names for the entity declarations).
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_contribution_outbox_state_next_eligible_tick` ON `contribution_outbox` (`state`, `next_eligible_tick`)")
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_contribution_outbox_remote_status` ON `contribution_outbox` (`remote_status`)")
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_photo_upload_sessions_owner_scope` ON `photo_upload_sessions` (`owner_scope`)")
        }
    }
    val MIGRATION_10_11: Migration = object : Migration(10, 11) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Local-only vehicle photo file name; null keeps every
            // existing row untouched, no data movement.
            db.execSQL("ALTER TABLE `vehicle` ADD COLUMN `photo_path` TEXT DEFAULT NULL")
        }
    }

    val MIGRATION_11_12: Migration = object : Migration(11, 12) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // PRM09-T01: additive premium outbox + cursor. No existing
            // table is touched; free vehicles and outbox rows survive.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `premium_outbox` (
                    `command_id` TEXT NOT NULL,
                    `owner_account` TEXT NOT NULL,
                    `origin` TEXT NOT NULL,
                    `dataset` TEXT NOT NULL,
                    `record_id` TEXT NOT NULL,
                    `expected_revision` INTEGER NOT NULL,
                    `payload_json` TEXT NOT NULL,
                    `delete_flag` INTEGER NOT NULL,
                    `void_flag` INTEGER NOT NULL,
                    `state` TEXT NOT NULL,
                    `attempts` INTEGER NOT NULL,
                    `next_eligible_tick` INTEGER NOT NULL,
                    `nonce` TEXT NOT NULL,
                    PRIMARY KEY(`command_id`)
                )
                """.trimIndent(),
            )
            db.execSQL(
                "CREATE INDEX IF NOT EXISTS `index_premium_outbox_owner_account_state_next_eligible_tick` " +
                    "ON `premium_outbox` (`owner_account`, `state`, `next_eligible_tick`)",
            )
            db.execSQL(
                "CREATE INDEX IF NOT EXISTS `index_premium_outbox_owner_account_origin_dataset` " +
                    "ON `premium_outbox` (`owner_account`, `origin`, `dataset`)",
            )
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `premium_cursor` (
                    `owner_account` TEXT NOT NULL,
                    `origin` TEXT NOT NULL,
                    `dataset` TEXT NOT NULL,
                    `cursor` TEXT NOT NULL,
                    `epoch` INTEGER NOT NULL,
                    `updated_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`owner_account`, `origin`, `dataset`)
                )
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_12_13: Migration = object : Migration(12, 13) {
        override fun migrate(db: SupportSQLiteDatabase) {
            db.execSQL("ALTER TABLE `server_station_cache` ADD COLUMN `address_json` TEXT NOT NULL DEFAULT '{}'")
        }
    }

    val MIGRATION_13_14: Migration = object : Migration(13,14) {
        override fun migrate(db:SupportSQLiteDatabase) {
            db.execSQL("CREATE TABLE IF NOT EXISTS `quality_owner_gate` (`owner_key` TEXT NOT NULL, `owner_local` TEXT NOT NULL, `origin` TEXT NOT NULL, `blocked` INTEGER NOT NULL, `last_seen` INTEGER NOT NULL, `expires_at` INTEGER NOT NULL, PRIMARY KEY(`owner_key`))")
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_quality_owner_gate_owner_local` ON `quality_owner_gate` (`owner_local`)")
            db.execSQL("CREATE TABLE IF NOT EXISTS `quality_report_outbox` (`owner_key` TEXT NOT NULL, `command_id` TEXT NOT NULL, `payload_hash` TEXT NOT NULL, `sealed_command` TEXT NOT NULL, `created_at` INTEGER NOT NULL, `expires_at` INTEGER NOT NULL, `state` TEXT NOT NULL, `attempts` INTEGER NOT NULL, `retry_at` INTEGER NOT NULL, `claim_id` TEXT NOT NULL, `lease_until` INTEGER NOT NULL, PRIMARY KEY(`owner_key`))")
            db.execSQL("CREATE UNIQUE INDEX IF NOT EXISTS `index_quality_report_outbox_command_id` ON `quality_report_outbox` (`command_id`)")
        }
    }

    val MIGRATION_14_15: Migration = object : Migration(14,15) {
        override fun migrate(db:SupportSQLiteDatabase) {
            db.execSQL("ALTER TABLE `quality_owner_gate` ADD COLUMN `key_erased` INTEGER NOT NULL DEFAULT 0")
        }
    }

}
