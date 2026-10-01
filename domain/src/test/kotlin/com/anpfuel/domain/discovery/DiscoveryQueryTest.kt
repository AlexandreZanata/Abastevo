package com.anpfuel.domain.discovery

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class DiscoveryQueryTest {

    @Test
    fun `valid query trims city and search and applies defaults`() {
        val query = DiscoveryQuery.create(
            state = BrazilianState.SAO_PAULO,
            municipality = "  São Paulo ",
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            search = "  posto centro ",
        )

        assertEquals("São Paulo", query.municipality)
        assertEquals("posto centro", query.search)
        assertEquals(DiscoverySort.PRICE_ASC, query.sort)
        assertEquals(0, query.page)
        assertEquals(DiscoveryQuery.DEFAULT_PAGE_SIZE, query.pageSize)
        assertEquals(0, query.offset())
    }

    @Test
    fun `blank city is rejected`() {
        assertThrows(DomainException::class.java) {
            DiscoveryQuery.create(
                state = BrazilianState.SAO_PAULO,
                municipality = "   ",
                fuelProduct = FuelProduct.ETHANOL,
            )
        }
    }

    @Test
    fun `short city and short search are rejected`() {
        assertThrows(DomainException::class.java) {
            DiscoveryQuery.create(
                state = BrazilianState.RIO_DE_JANEIRO,
                municipality = "A",
                fuelProduct = FuelProduct.ETHANOL,
            )
        }
        assertThrows(DomainException::class.java) {
            DiscoveryQuery.create(
                state = BrazilianState.RIO_DE_JANEIRO,
                municipality = "Niterói",
                fuelProduct = FuelProduct.ETHANOL,
                search = "x",
            )
        }
    }

    @Test
    fun `blank search becomes null`() {
        val query = DiscoveryQuery.create(
            state = BrazilianState.MINAS_GERAIS,
            municipality = "Belo Horizonte",
            fuelProduct = FuelProduct.DIESEL_S10,
            search = "   ",
        )

        assertNull(query.search)
    }

    @Test
    fun `pagination bounds are enforced and offset is stable`() {
        assertThrows(DomainException::class.java) {
            DiscoveryQuery.create(
                state = BrazilianState.SAO_PAULO,
                municipality = "Campinas",
                fuelProduct = FuelProduct.CNG,
                page = -1,
            )
        }
        assertThrows(DomainException::class.java) {
            DiscoveryQuery.create(
                state = BrazilianState.SAO_PAULO,
                municipality = "Campinas",
                fuelProduct = FuelProduct.CNG,
                pageSize = 0,
            )
        }
        assertThrows(DomainException::class.java) {
            DiscoveryQuery.create(
                state = BrazilianState.SAO_PAULO,
                municipality = "Campinas",
                fuelProduct = FuelProduct.CNG,
                pageSize = DiscoveryQuery.MAX_PAGE_SIZE + 1,
            )
        }

        val query = DiscoveryQuery.create(
            state = BrazilianState.SAO_PAULO,
            municipality = "Campinas",
            fuelProduct = FuelProduct.CNG,
            sort = DiscoverySort.RECENCY_DESC,
            page = 2,
            pageSize = 15,
        )
        assertEquals(30, query.offset())
    }

    @Test
    fun `missing state or fuel is rejected`() {
        assertThrows(DomainException::class.java) {
            DiscoveryQuery.create(
                state = null,
                municipality = "Osasco",
                fuelProduct = FuelProduct.ETHANOL,
            )
        }
        assertThrows(DomainException::class.java) {
            DiscoveryQuery.create(
                state = BrazilianState.SAO_PAULO,
                municipality = "Osasco",
                fuelProduct = null,
            )
        }
    }
}
