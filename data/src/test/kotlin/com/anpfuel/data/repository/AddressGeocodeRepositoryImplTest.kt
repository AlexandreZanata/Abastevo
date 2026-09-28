package com.anpfuel.data.repository

import com.anpfuel.data.local.preferences.AddressGeocodeCacheStore
import com.anpfuel.data.remote.AddressGeocodeCacheKeyFormatter
import com.anpfuel.data.remote.NominatimFixtureFiles
import com.anpfuel.data.remote.NominatimRateLimiter
import com.anpfuel.data.remote.NominatimSearchClient
import com.anpfuel.domain.repository.AddressGeocodeOutcome
import com.anpfuel.domain.repository.GeocodeRequest
import com.anpfuel.domain.valueobject.GeoCoordinates
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

class AddressGeocodeRepositoryImplTest {

    private lateinit var server: MockWebServer
    private lateinit var cacheStore: InMemoryAddressGeocodeCacheStore
    private lateinit var repository: AddressGeocodeRepositoryImpl

    private val query = "POSTO CENTRO, RUA XV DE NOVEMBRO, 1000, Curitiba - PR, Brazil"
    private val coordinates = GeoCoordinates.of(-25.4293875, -49.2718364)

    @BeforeEach
    fun setUp() {
        server = MockWebServer()
        server.start()

        cacheStore = InMemoryAddressGeocodeCacheStore()

        val client = NominatimSearchClient(
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

        repository = AddressGeocodeRepositoryImpl(
            addressGeocodeCacheStore = cacheStore,
            nominatimSearchClient = client,
            rateLimiter = NominatimRateLimiter(
                Clock.fixed(Instant.parse("2026-06-21T12:00:00Z"), ZoneOffset.UTC),
            ),
        )
    }

    @AfterEach
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun returnsCachedCoordinatesWithoutNetworkCall() = runTest {
        cacheStore.put(AddressGeocodeCacheKeyFormatter.format(query), coordinates)

        val outcome = repository.geocode(query)

        assertEquals(AddressGeocodeOutcome.Success(coordinates), outcome)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun reusesCacheRegardlessOfCaseAndSpacing() = runTest {
        cacheStore.put(AddressGeocodeCacheKeyFormatter.format(query), coordinates)

        val outcome = repository.geocode("  posto centro,   rua xv de novembro, 1000, curitiba - pr, brazil ")

        assertEquals(AddressGeocodeOutcome.Success(coordinates), outcome)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun fetchesAndCachesCoordinates() = runTest {
        server.enqueue(jsonResponse(NominatimFixtureFiles.readSearchCuritiba()))

        val firstOutcome = repository.geocode(query)
        val secondOutcome = repository.geocode(query)

        assertEquals(AddressGeocodeOutcome.Success(coordinates), firstOutcome)
        assertEquals(AddressGeocodeOutcome.Success(coordinates), secondOutcome)
        assertEquals(1, server.requestCount)
        assertEquals(
            coordinates,
            cacheStore.get(AddressGeocodeCacheKeyFormatter.format(query)),
        )
    }

    @Test
    fun returnsNotFoundWhenNominatimHasNoResult() = runTest {
        server.enqueue(jsonResponse("[]"))

        val outcome = repository.geocode(query)

        assertEquals(AddressGeocodeOutcome.NotFound, outcome)
    }

    @Test
    fun returnsNetworkErrorWhenNominatimFails() = runTest {
        server.enqueue(MockResponse().setResponseCode(500))

        val outcome = repository.geocode(query)

        assertEquals(AddressGeocodeOutcome.NetworkError, outcome)
    }

    @Test
    fun returnsRateLimitedWhenThrottleStaysSaturated() = runTest {
        server.enqueue(jsonResponse(NominatimFixtureFiles.readSearchCuritiba()))
        server.enqueue(jsonResponse(NominatimFixtureFiles.readSearchCuritiba()))

        val firstOutcome = repository.geocode(query)
        val secondOutcome = repository.geocode("POSTO SEGUNDO, RUA A, 200, Curitiba - PR, Brazil")

        assertTrue(firstOutcome is AddressGeocodeOutcome.Success)
        assertEquals(AddressGeocodeOutcome.RateLimited, secondOutcome)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun returnsNotFoundForBlankQuery() = runTest {
        assertEquals(AddressGeocodeOutcome.NotFound, repository.geocode("   "))
        assertEquals(0, server.requestCount)
    }

    @Test
    fun returnsSuccessForStructuredRequestAndCachesResult() = runTest {
        server.enqueue(jsonResponse(NominatimFixtureFiles.readSearchCuritiba()))

        val request = GeocodeRequest.Structured(
            street = "Rua XV de Novembro",
            city = "Curitiba",
            state = "Paraná",
        )
        val outcome = repository.geocode(listOf(request))

        assertEquals(AddressGeocodeOutcome.Success(coordinates), outcome)
        assertEquals(1, server.requestCount)
        val requestRecorded = server.takeRequest()
        assertEquals("Rua XV de Novembro", requestRecorded.requestUrl?.queryParameter("street"))
        assertEquals("Curitiba", requestRecorded.requestUrl?.queryParameter("city"))
        assertEquals("Paraná", requestRecorded.requestUrl?.queryParameter("state"))
    }

    @Test
    fun abortsCascadeOnTransportError() = runTest {
        server.enqueue(MockResponse().setResponseCode(500))

        val requests = listOf(
            GeocodeRequest.Structured(street = "Rua A", city = "Curitiba", state = "Paraná"),
            GeocodeRequest.FreeText("Curitiba"),
        )
        val outcome = repository.geocode(requests)

        assertEquals(AddressGeocodeOutcome.NetworkError, outcome)
        assertEquals(1, server.requestCount)
    }

    private fun jsonResponse(body: String): MockResponse =
        MockResponse()
            .setResponseCode(200)
            .setHeader("Content-Type", "application/json")
            .setBody(body)

    private class InMemoryAddressGeocodeCacheStore : AddressGeocodeCacheStore {
        private val values = mutableMapOf<String, GeoCoordinates>()

        override suspend fun get(cacheKey: String): GeoCoordinates? = values[cacheKey]

        override suspend fun put(cacheKey: String, coordinates: GeoCoordinates) {
            values[cacheKey] = coordinates
        }
    }
}
