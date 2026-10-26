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

    val MIGRATION_16_17: Migration = object : Migration(16, 17) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // OPT community snapshot: additive private cache for validated
            // first-page/summary payloads. No existing table is touched;
            // stored rows keep their original dates and are filtered at
            // read time, so validity can only shrink, never extend.
            // No secondary index on purpose: every read is by primary
            // `key` (or a full-table hygiene sweep), matching the entity
            // schema exactly on migrated installs.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `community_feed_cache` (
                    `key` TEXT NOT NULL,
                    `kind` TEXT NOT NULL,
                    `origin` TEXT NOT NULL,
                    `state` TEXT NOT NULL,
                    `municipality_code` TEXT NOT NULL,
                    `fuel` TEXT,
                    `sort` TEXT,
                    `payload_json` TEXT NOT NULL,
                    `generated_at_millis` INTEGER NOT NULL,
                    `fetched_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`key`)
                )
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_15_16: Migration = object : Migration(15, 16) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Kotlin Unicode folding preserves the original case-insensitive name/address semantics;
            // SQLite NOCASE/LIKE only fold ASCII. Walk the cursor without loading the table into memory.
            db.execSQL("ALTER TABLE `station_price` ADD COLUMN `legal_name_search` TEXT NOT NULL DEFAULT ''")
            db.execSQL("ALTER TABLE `station_price` ADD COLUMN `trade_name_search` TEXT NOT NULL DEFAULT ''")
            db.execSQL("ALTER TABLE `station_price` ADD COLUMN `brand_search` TEXT NOT NULL DEFAULT ''")
            db.execSQL("ALTER TABLE `station_price` ADD COLUMN `address_search` TEXT NOT NULL DEFAULT ''")
            db.query("SELECT id, legal_name, trade_name, brand, address FROM station_price").use { rows ->
                while (rows.moveToNext()) {
                    db.execSQL(
                        "UPDATE station_price SET legal_name_search=?, trade_name_search=?, brand_search=?, address_search=? WHERE id=?",
                        arrayOf(
                            com.anpfuel.data.local.entity.foldStationSearch(rows.getString(1)),
                            com.anpfuel.data.local.entity.foldStationSearch(rows.getString(2)),
                            com.anpfuel.data.local.entity.foldStationSearch(rows.getString(3)),
                            com.anpfuel.data.local.entity.foldStationSearch(rows.getString(4)), rows.getString(0),
                        ),
                    )
                }
            }
            db.execSQL("CREATE INDEX IF NOT EXISTS `idx_station_discovery_price` ON `station_price` (`survey_week_id`, `state`, `municipality`, `fuel_product`, `price`, `id`)")
            // Scoped page participation: the primary key becomes
            // (station_id, scope_key) so the same station can belong to
            // the national page and a city page with independent
            // positions, while unscoped detail records survive page
            // replacement. Surviving page rows inherit the stored cursor
            // scope; detail rows keep the unscoped identity scope. ANP
            // and every other table are untouched: no data movement
            // outside server_station_cache, no drops elsewhere.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `server_station_cache_new` (
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
                    `address_json` TEXT NOT NULL DEFAULT '{}',
                    `scope_key` TEXT NOT NULL DEFAULT '',
                    PRIMARY KEY(`station_id`, `scope_key`)
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                INSERT INTO `server_station_cache_new` (
                    `station_id`, `display_name`, `location_quality`, `latitude`, `longitude`,
                    `cnpj_normalized`, `municipality_code`, `state`, `current_revision_id`,
                    `position`, `saved_at_millis`, `address_json`, `scope_key`
                )
                SELECT `station_id`, `display_name`, `location_quality`, `latitude`, `longitude`,
                    `cnpj_normalized`, `municipality_code`, `state`, `current_revision_id`,
                    `position`, `saved_at_millis`, `address_json`,
                    CASE WHEN `position` != 2147483647
                        THEN COALESCE((SELECT `key` FROM `server_catalog_meta` LIMIT 1), 'page')
                        ELSE '' END
                FROM `server_station_cache`
                """.trimIndent(),
            )
            db.execSQL("DROP TABLE `server_station_cache`")
            db.execSQL("ALTER TABLE `server_station_cache_new` RENAME TO `server_station_cache`")
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_server_station_cache_scope_key_position` ON `server_station_cache` (`scope_key`, `position`)")
        }
    }

    val MIGRATION_17_18: Migration = object : Migration(17, 18) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Additive MEX-05 prefix index; canonical v17 community snapshots survive.
            db.execSQL("CREATE INDEX IF NOT EXISTS `idx_municipality_catalog_normalized` ON `municipality_catalog` (`normalized_name`)")
        }
    }

    val MIGRATION_18_19: Migration = object : Migration(18, 19) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Additive EE-18 public EV snapshot tables. No existing table is
            // altered; every ANP/history/fuel row and every private/outbox
            // row survives the upgrade untouched.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `charging_snapshot_place` (
                    `place_id` TEXT NOT NULL,
                    `generation` INTEGER NOT NULL,
                    `profile` TEXT NOT NULL,
                    `source_revision` TEXT NOT NULL,
                    `origin` TEXT NOT NULL,
                    `license` TEXT NOT NULL,
                    `name` TEXT NOT NULL,
                    `country` TEXT NOT NULL,
                    `latitude` TEXT,
                    `longitude` TEXT,
                    `public_access` TEXT NOT NULL,
                    `reported_points` INTEGER NOT NULL,
                    `groups_json` TEXT NOT NULL,
                    `tombstone` INTEGER NOT NULL,
                    `saved_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`place_id`, `generation`)
                )
                """.trimIndent(),
            )
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_charging_snapshot_place_generation` ON `charging_snapshot_place` (`generation`)")
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_charging_snapshot_place_profile_generation` ON `charging_snapshot_place` (`profile`, `generation`)")
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `charging_snapshot_meta` (
                    `key` TEXT NOT NULL,
                    `generation` INTEGER NOT NULL,
                    `profile` TEXT NOT NULL,
                    `source_revision` TEXT NOT NULL,
                    `origin` TEXT NOT NULL,
                    `license` TEXT NOT NULL,
                    `schema_version` INTEGER NOT NULL,
                    `base_generation` INTEGER NOT NULL,
                    `place_count` INTEGER NOT NULL,
                    `group_count` INTEGER NOT NULL,
                    `cursor` TEXT,
                    `state` TEXT NOT NULL,
                    `saved_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`key`)
                )
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_19_20: Migration = object : Migration(19, 20) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // EE-19 energy-capable vehicles: propulsion plus optional
            // battery/connector, with tank and fuel relaxed for BEV.
            // Rebuild preserves every row (legacy rows read back as ICE
            // with no battery, never backfilled) and no row is dropped.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `vehicle_new` (
                    `id` TEXT NOT NULL,
                    `display_name` TEXT NOT NULL,
                    `tank_capacity_liters` REAL,
                    `fuel_product` TEXT,
                    `price_source_mode` TEXT NOT NULL,
                    `specific_station_cnpj` TEXT,
                    `price_drop_alert_enabled` INTEGER NOT NULL,
                    `sort_order` INTEGER NOT NULL,
                    `propulsion` TEXT NOT NULL DEFAULT 'ICE',
                    `battery_wh` INTEGER,
                    `charge_connector` TEXT,
                    `photo_path` TEXT,
                    PRIMARY KEY(`id`)
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                INSERT INTO `vehicle_new` (`id`, `display_name`, `tank_capacity_liters`, `fuel_product`, `price_source_mode`, `specific_station_cnpj`, `price_drop_alert_enabled`, `sort_order`, `propulsion`, `battery_wh`, `charge_connector`, `photo_path`)
                SELECT `id`, `display_name`, `tank_capacity_liters`, `fuel_product`, `price_source_mode`, `specific_station_cnpj`, `price_drop_alert_enabled`, `sort_order`, 'ICE', NULL, NULL, `photo_path` FROM `vehicle`
                """.trimIndent(),
            )
            db.execSQL("DROP TABLE `vehicle`")
            db.execSQL("ALTER TABLE `vehicle_new` RENAME TO `vehicle`")
        }
    }

    val MIGRATION_20_21: Migration = object : Migration(20, 21) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Additive MEX-08 owner/origin capability lease table. No
            // existing table is altered; every vehicle and private row
            // survives untouched.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `vehicle_capability_lease` (
                    `owner_account` TEXT NOT NULL,
                    `origin` TEXT NOT NULL,
                    `policy_version` INTEGER NOT NULL,
                    `capability` TEXT NOT NULL,
                    `verified_at_millis` INTEGER NOT NULL,
                    `paid_through_millis` INTEGER NOT NULL,
                    `revoked` INTEGER NOT NULL,
                    `saved_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`owner_account`, `origin`)
                )
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_21_22: Migration = object : Migration(21, 22) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Additive MEX-11 durable ledger draft table. No existing
            // table is altered; every vehicle, snapshot and private row
            // survives untouched.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `ledger_draft` (
                    `draft_id` TEXT NOT NULL,
                    `owner_account` TEXT NOT NULL,
                    `origin` TEXT NOT NULL,
                    `dataset` TEXT NOT NULL,
                    `record_id` TEXT NOT NULL,
                    `expected_revision` INTEGER NOT NULL,
                    `body_json` TEXT NOT NULL,
                    `updated_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`draft_id`)
                )
                """.trimIndent(),
            )
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_ledger_draft_owner_record` ON `ledger_draft` (`owner_account`, `origin`, `dataset`, `record_id`)")
        }
    }

    val MIGRATION_22_23: Migration = object : Migration(22, 23) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Additive EE-20 local-only session context on drafts. No
            // existing table is altered; every draft and private row
            // survives untouched (context defaults to absent).
            db.execSQL("ALTER TABLE `ledger_draft` ADD COLUMN `context` TEXT DEFAULT NULL")
        }
    }

    val MIGRATION_23_24: Migration = object : Migration(23, 24) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Additive EE-21 snapshot identity columns (source entity key
            // and external ID travel with the pack instead of being
            // re-derived). No rows are touched; existing nulls stay null
            // and only live rows decode (tombstones keep null identity).
            db.execSQL("ALTER TABLE `charging_snapshot_place` ADD COLUMN `entity_key` TEXT DEFAULT NULL")
            db.execSQL("ALTER TABLE `charging_snapshot_place` ADD COLUMN `external_id` TEXT DEFAULT NULL")
        }
    }

    val MIGRATION_24_25: Migration = object : Migration(24, 25) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Additive MEX-15 local erasure log (tombstone memory for
            // restore-old-after-erase). No rows are touched; restore
            // consults before adopting any server row.
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `ledger_erased` (
                    `owner_account` TEXT NOT NULL,
                    `origin` TEXT NOT NULL,
                    `dataset` TEXT NOT NULL,
                    `record_id` TEXT NOT NULL,
                    `erased_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`owner_account`, `origin`, `dataset`, `record_id`)
                )
                """.trimIndent(),
            )
        }
    }

    val MIGRATION_25_26: Migration = object : Migration(25, 26) {
        override fun migrate(db: SupportSQLiteDatabase) {
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `map_generation` (
                    `generation` TEXT NOT NULL,
                    `manifest_sha256` TEXT NOT NULL,
                    `active` INTEGER NOT NULL,
                    `activated_at_millis` INTEGER,
                    PRIMARY KEY(`generation`)
                )
                """.trimIndent(),
            )
            db.execSQL(
                """
                CREATE TABLE IF NOT EXISTS `map_package` (
                    `package_id` TEXT NOT NULL,
                    `generation` TEXT NOT NULL,
                    `role` TEXT NOT NULL,
                    `file_name` TEXT NOT NULL,
                    `bytes` INTEGER NOT NULL,
                    `sha256` TEXT NOT NULL,
                    `etag` TEXT,
                    `status` TEXT NOT NULL,
                    `downloaded_bytes` INTEGER NOT NULL,
                    `error` TEXT,
                    `updated_at_millis` INTEGER NOT NULL,
                    PRIMARY KEY(`generation`, `package_id`),
                    FOREIGN KEY(`generation`) REFERENCES `map_generation`(`generation`) ON UPDATE NO ACTION ON DELETE CASCADE
                )
                """.trimIndent(),
            )
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_map_package_generation` ON `map_package` (`generation`)")
            db.execSQL("CREATE INDEX IF NOT EXISTS `index_map_package_status` ON `map_package` (`status`)")
        }
    }

    val MIGRATION_26_27: Migration = object : Migration(26, 27) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Append-only: records the native data version
            // (`engine.format_version`, numeric version-directory name) that
            // activation publishes. Pre-existing rows default to empty and
            // are treated as unpublished until re-activated.
            db.execSQL("ALTER TABLE `map_generation` ADD COLUMN `data_version` TEXT NOT NULL DEFAULT ''")
        }
    }

    val MIGRATION_27_28: Migration = object : Migration(27, 28) {
        override fun migrate(db: SupportSQLiteDatabase) {
            // Append-only: MAP-25 private, owner-scoped favorites, saved trips,
            // id-only tombstones and import receipts. No existing table is touched.
            db.execSQL(
                "CREATE TABLE `map_saved_place` (`place_id` TEXT NOT NULL, `owner_account` TEXT NOT NULL, `origin` TEXT NOT NULL, `kind` TEXT NOT NULL, `name` TEXT NOT NULL, `lat_micro` INTEGER NOT NULL, `lon_micro` INTEGER NOT NULL, `station_id` TEXT, `source_kind` TEXT NOT NULL, `source_id` TEXT, `note` TEXT, `created_at_millis` INTEGER NOT NULL, PRIMARY KEY(`owner_account`, `origin`, `place_id`))",
            )
            db.execSQL(
                "CREATE UNIQUE INDEX IF NOT EXISTS `index_map_saved_place_station` ON `map_saved_place` (`owner_account`, `origin`, `station_id`)",
            )
            db.execSQL(
                "CREATE TABLE `map_saved_trip` (`trip_id` TEXT NOT NULL, `owner_account` TEXT NOT NULL, `origin` TEXT NOT NULL, `state` TEXT NOT NULL, `kind` TEXT NOT NULL, `name` TEXT NOT NULL, `avoid` TEXT NOT NULL, `source_id` TEXT, `note` TEXT, `point_count` INTEGER NOT NULL, `created_at_millis` INTEGER NOT NULL, PRIMARY KEY(`owner_account`, `origin`, `trip_id`))",
            )
            db.execSQL(
                "CREATE INDEX IF NOT EXISTS `index_map_saved_trip_owner` ON `map_saved_trip` (`owner_account`, `origin`, `state`)",
            )
            db.execSQL(
                "CREATE TABLE `map_saved_trip_point` (`owner_account` TEXT NOT NULL, `origin` TEXT NOT NULL, `trip_id` TEXT NOT NULL, `seq` INTEGER NOT NULL, `lat_micro` INTEGER NOT NULL, `lon_micro` INTEGER NOT NULL, `elevation_cm` INTEGER, `time_millis` INTEGER, PRIMARY KEY(`owner_account`, `origin`, `trip_id`, `seq`), FOREIGN KEY(`owner_account`, `origin`, `trip_id`) REFERENCES `map_saved_trip`(`owner_account`, `origin`, `trip_id`) ON UPDATE NO ACTION ON DELETE CASCADE )",
            )
            db.execSQL(
                "CREATE TABLE `map_saved_tombstone` (`owner_account` TEXT NOT NULL, `origin` TEXT NOT NULL, `record_id` TEXT NOT NULL, `deleted_at_millis` INTEGER NOT NULL, PRIMARY KEY(`owner_account`, `origin`, `record_id`))",
            )
            db.execSQL(
                "CREATE TABLE `map_saved_import` (`owner_account` TEXT NOT NULL, `origin` TEXT NOT NULL, `content_sha256` TEXT NOT NULL, `imported_at_millis` INTEGER NOT NULL, PRIMARY KEY(`owner_account`, `origin`, `content_sha256`))",
            )
        }
    }
}
