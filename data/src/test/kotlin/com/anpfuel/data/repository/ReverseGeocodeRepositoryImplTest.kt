package com.anpfuel.data.repository

import com.anpfuel.data.local.preferences.GeocodeCacheStore
import com.anpfuel.data.remote.GeocodeCacheKeyFormatter
import com.anpfuel.data.remote.NominatimFixtureFiles
import com.anpfuel.data.remote.NominatimRateLimiter
import com.anpfuel.data.remote.NominatimReverseGeocodeClient
import com.anpfuel.domain.model.ReverseGeocodeResult
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.ReverseGeocodeOutcome
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DeviceLocation
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import io.mockk.coEvery
import io.mockk.mockk
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class ReverseGeocodeRepositoryImplTest {

    private lateinit var server: MockWebServer
    private lateinit var cacheStore: InMemoryGeocodeCacheStore
    private lateinit var rateLimiter: NominatimRateLimiter
    private lateinit var municipalityCatalogRepository: MunicipalityCatalogRepository
    private lateinit var repository: ReverseGeocodeRepositoryImpl

    private val location = DeviceLocation.of(-25.4284, -49.2733)

    @BeforeEach
    fun setUp() {
        server = MockWebServer()
        server.start()

        cacheStore = InMemoryGeocodeCacheStore()
        rateLimiter = NominatimRateLimiter(Clock.fixed(Instant.parse("2026-06-21T12:00:00Z"), ZoneOffset.UTC))
        municipalityCatalogRepository = mockk()

        val client = NominatimReverseGeocodeClient(
            okHttpClient = OkHttpClient.Builder()
                .addInterceptor { chain ->
                    val original = chain.request().url
                    val rewritten = original.newBuilder()
                        .scheme(server.url("/").scheme)
                        .host(server.hostName)
                        .port(server.port)
                        .build()
                    chain.proceed(chain.request().newBuilder().url(rewritten).build())
                }
                .build(),
        )

        repository = ReverseGeocodeRepositoryImpl(
            geocodeCacheStore = cacheStore,
            nominatimClient = client,
            rateLimiter = rateLimiter,
            municipalityCatalogRepository = municipalityCatalogRepository,
        )
    }

    @AfterEach
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun returnsCachedResultWithoutNetworkCall() = runTest {
        val cacheKey = GeocodeCacheKeyFormatter.format(location)
        val cached = ReverseGeocodeResult(
            state = BrazilianState.PARANA,
            municipality = "CURITIBA",
        )
        cacheStore.put(cacheKey, cached)
        coEvery {
            municipalityCatalogRepository.findCatalogEntry(BrazilianState.PARANA, "CURITIBA")
        } returns MunicipalityCatalogEntry(
            state = BrazilianState.PARANA,
            municipality = "CURITIBA",
        )

        val outcome = repository.reverseGeocode(location)

        assertTrue(outcome is ReverseGeocodeOutcome.Success)
        assertEquals(cached, (outcome as ReverseGeocodeOutcome.Success).result)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun fetchesAndCachesWhenCatalogEntryMatches() = runTest {
        coEvery {
            municipalityCatalogRepository.findCatalogEntry(BrazilianState.PARANA, any())
        } returns MunicipalityCatalogEntry(
            state = BrazilianState.PARANA,
            municipality = "CURITIBA",
        )

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(NominatimFixtureFiles.readReverseCuritiba()),
        )

        val firstOutcome = repository.reverseGeocode(location)
        val secondOutcome = repository.reverseGeocode(location)

        assertTrue(firstOutcome is ReverseGeocodeOutcome.Success)
        assertEquals("CURITIBA", (firstOutcome as ReverseGeocodeOutcome.Success).result.municipality)
        assertTrue(secondOutcome is ReverseGeocodeOutcome.Success)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun returnsMunicipalityNotInCatalogWhenUnknownCity() = runTest {
        coEvery {
            municipalityCatalogRepository.findCatalogEntry(BrazilianState.PARANA, "Curitiba")
        } returns null

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(NominatimFixtureFiles.readReverseCuritiba()),
        )

        val outcome = repository.reverseGeocode(location)

        assertEquals(ReverseGeocodeOutcome.MunicipalityNotInCatalog, outcome)
    }

    @Test
    fun returnsRateLimitedWhenRequestsAreTooFast() = runTest {
        val secondLocation = DeviceLocation.of(-23.5505, -46.6333)

        coEvery {
            municipalityCatalogRepository.findCatalogEntry(any(), any())
        } returns MunicipalityCatalogEntry(
            state = BrazilianState.PARANA,
            municipality = "CURITIBA",
        )

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(NominatimFixtureFiles.readReverseCuritiba()),
        )
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(NominatimFixtureFiles.readReverseCuritiba()),
        )

        val firstOutcome = repository.reverseGeocode(location)
        val secondOutcome = repository.reverseGeocode(secondLocation)

        assertTrue(firstOutcome is ReverseGeocodeOutcome.Success)
        assertEquals(ReverseGeocodeOutcome.RateLimited, secondOutcome)
        assertEquals(1, server.requestCount)
    }

    private class InMemoryGeocodeCacheStore : GeocodeCacheStore {
        private val values = mutableMapOf<String, ReverseGeocodeResult>()

        override suspend fun get(cacheKey: String): ReverseGeocodeResult? = values[cacheKey]

        override suspend fun put(cacheKey: String, result: ReverseGeocodeResult) {
            values[cacheKey] = result
        }
    }
}
