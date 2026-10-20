package com.anpfuel.data.local

import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.entity.StationPriceEntity
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * P27 ANP bounded discovery proof on real Room: filtering, ordering
 * with a stable identity tiebreak, pagination windows, totals and the
 * targeted CNPJ read run in the query. JVM suites pin the emitted SQL
 * against real SQLite; this runs the Room-generated implementation on
 * device/emulator.
 */
@RunWith(AndroidJUnit4::class)
class StationPriceDaoPagingTest {
    private lateinit var database: AnpFuelDatabase

    private fun row(
        id: String,
        price: Double,
        tradeName: String? = "Posto Alfa",
        collectedAt: String? = "2026-06-10",
        fuel: String = "GASOLINE_REGULAR",
        municipality: String = "SAO PAULO",
        cnpj: String = "11111111111111",
    ) = StationPriceEntity(
        id = id,
        surveyWeekId = "w1",
        cnpj = cnpj,
        legalName = "RAZAO ALFA LTDA",
        tradeName = tradeName,
        address = "Rua A, 10",
        municipality = municipality,
        state = "SP",
        brand = "BR",
        fuelProduct = fuel,
        price = price,
        collectedAt = collectedAt,
    )

    @Before fun open() {
        database = Room.inMemoryDatabaseBuilder(
            ApplicationProvider.getApplicationContext(), AnpFuelDatabase::class.java,
        ).build()
        runBlocking {
            database.stationPriceDao().insertAll(
                listOf(
                    row("w1|111|G", 5.19),
                    row("w1|222|G", 5.19, tradeName = "Posto Beta"),
                    row("w1|333|G", 5.29, tradeName = "Posto Gama", collectedAt = null),
                    row("w1|444|G", 4.99, tradeName = "Posto 100%", collectedAt = "2026-06-09"),
                    row("w1|555|E", 3.49, fuel = "ETHANOL"),
                ),
            )
        }
    }

    @After fun close() = database.close()

    @Test fun priceWindowIsOrderedWithStableTiebreak() = runBlocking {
        val dao = database.stationPriceDao()
        val first = dao.findPageByPrice("w1", "SP", "SAO PAULO", "GASOLINE_REGULAR", "", 2, 0)
        assertEquals(listOf("w1|444|G", "w1|111|G"), first.map { it.id })
        val second = dao.findPageByPrice("w1", "SP", "SAO PAULO", "GASOLINE_REGULAR", "", 2, 2)
        assertEquals(listOf("w1|222|G", "w1|333|G"), second.map { it.id })
        assertEquals(4, dao.countPage("w1", "SP", "SAO PAULO", "GASOLINE_REGULAR", ""))
    }

    @Test fun searchFiltersCaseInsensitivelyAndWildcardsStayLiteral() = runBlocking {
        val dao = database.stationPriceDao()
        assertEquals(1, dao.findPageByPrice("w1", "SP", "SAO PAULO", "GASOLINE_REGULAR", "beta", 20, 0).size)
        assertEquals(
            "w1|444|G",
            dao.findPageByPrice("w1", "SP", "SAO PAULO", "GASOLINE_REGULAR", "100\\%", 20, 0).single().id,
        )
    }

    @Test fun recencyWindowOrdersNewestFirstWithNullsLast() = runBlocking {
        val dao = database.stationPriceDao()
        val rows = dao.findPageByRecency("w1", "SP", "SAO PAULO", "GASOLINE_REGULAR", "", 20, 0)
        assertEquals(listOf("w1|222|G", "w1|111|G", "w1|444|G", "w1|333|G"), rows.map { it.id })
    }

    @Test fun targetedCnpjReadIsFuelScoped() = runBlocking {
        val dao = database.stationPriceDao()
        assertEquals("w1|111|G", dao.findByCnpj("w1", "SP", "SAO PAULO", "GASOLINE_REGULAR", "11111111111111")?.id)
        assertNull(dao.findByCnpj("w1", "SP", "SAO PAULO", "ETHANOL", "11111111111111"))
    }
}
