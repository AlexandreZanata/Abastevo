package com.anpfuel.data.local

import androidx.room.Room
import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.entity.CommunityFeedCacheEntity
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Real SQLite upgrade/reopen and generated DAO acceptance in a dedicated test DB. */
@RunWith(AndroidJUnit4::class)
class CommunityCacheMigration16To17Test {
    private val context = InstrumentationRegistry.getInstrumentation().targetContext

    @get:Rule
    val helper = MigrationTestHelper(
        InstrumentationRegistry.getInstrumentation(),
        AnpFuelDatabase::class.java,
    )

    @Test
    fun upgradePreservesAnpRowsAndSnapshotsSurviveRoomReopen() = runBlocking {
        val name = "opt-community-migration-16-17"
        helper.createDatabase(name, 16).apply {
            execSQL("INSERT INTO survey_week (id, start_date, end_date, summary_imported_at) VALUES ('opt-week', '2026-01-01', '2026-01-07', 1000)")
            execSQL("INSERT INTO station_price (id, survey_week_id, cnpj, address, municipality, state, fuel_product, price) VALUES ('opt-price', 'opt-week', '04218406000104', 'Synthetic street', 'Synthetic city', 'SP', 'GASOLINE_REGULAR', 5.99)")
            close()
        }
        helper.runMigrationsAndValidate(name, 17, true, AnpFuelDatabaseMigrations.MIGRATION_16_17).apply {
            query("SELECT price FROM station_price WHERE id = 'opt-price'").use {
                assertTrue(it.moveToFirst())
                assertEquals(5.99, it.getDouble(0), 0.0)
            }
            close()
        }
        try {
            val first = Room.databaseBuilder(context, AnpFuelDatabase::class.java, name)
                .addMigrations(AnpFuelDatabaseMigrations.MIGRATION_17_18).build()
            try {
                first.communityFeedCacheDao().upsert(row("feed|origin-a|city-a", 1000))
                first.communityFeedCacheDao().upsert(row("feed|origin-b|city-a", 2000))
            } finally { first.close() }
            val reopened = Room.databaseBuilder(context, AnpFuelDatabase::class.java, name)
                .addMigrations(AnpFuelDatabaseMigrations.MIGRATION_17_18).build()
            try {
                val dao = reopened.communityFeedCacheDao()
                assertEquals("{}", dao.findByKey("feed|origin-a|city-a")?.payloadJson)
                assertEquals(2000L, dao.findByKey("feed|origin-b|city-a")?.fetchedAtMillis)
                assertNull(dao.findByKey("feed|origin-a|city-b"))
                dao.deleteOlderThan(1500)
                assertNull(dao.findByKey("feed|origin-a|city-a"))
                assertNotNull(dao.findByKey("feed|origin-b|city-a"))
                repeat(70) { dao.upsert(row("scope-$it", 3000L + it)) }
                dao.trimToNewest()
                assertEquals(64, dao.count())
                assertNull(dao.findByKey("scope-5"))
                assertNotNull(dao.findByKey("scope-6"))
                assertNotNull(dao.findByKey("scope-69"))
            } finally { reopened.close() }
        } finally {
            context.deleteDatabase(name)
        }
    }

    private fun row(key: String, fetched: Long) = CommunityFeedCacheEntity(
        key, "FEED", "https://synthetic.invalid", "SP", "3550308", "ETHANOL", "RECENT", "{}", 1000, fetched,
    )
}
