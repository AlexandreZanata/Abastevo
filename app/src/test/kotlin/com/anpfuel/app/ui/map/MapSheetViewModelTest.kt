package com.anpfuel.app.ui.map

import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapStationLocationQuality
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapSheetViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    private fun marker(
        uuid: String = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        fuel: Boolean = true,
        charging: Boolean = false,
    ) = MapOverlayMarker(
        uuid = uuid,
        latitude = -25.42,
        longitude = -49.27,
        title = "Posto",
        fuel = fuel,
        charging = charging,
        locationQuality = MapStationLocationQuality.REVIEWED,
    )

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun tearDown() { Dispatchers.resetMain() }

    @Test fun `anp price shows exact product unit and date`() = runTest {
        val vm = MapSheetViewModel(SheetPriceSource { _, source, _ ->
            if (source == MapPriceSource.ANP) SheetPrice("GASOLINE_REGULAR", "L", "5.99", source, "2026-10-20") else null
        })
        vm.select(marker(), MapPriceSource.ANP)
        advanceTimeBy(200)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("5.99", vm.ui.value.price?.amount)
        assertEquals("L", vm.ui.value.price?.unit)
        assertEquals(false, vm.ui.value.priceAbsent)
    }

    @Test fun `absence is explicit without silent fallback`() = runTest {
        val vm = MapSheetViewModel(SheetPriceSource { _, _, _ -> null })
        vm.select(marker(), MapPriceSource.ANP)
        advanceTimeBy(200)
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(vm.ui.value.priceAbsent)
        assertNull(vm.ui.value.price)
        // Switching source does not paint the old response.
        vm.select(marker(), MapPriceSource.COMMUNITY)
        advanceTimeBy(50)
        // Old ANP price (null) stays null; new load is pending, never a mix.
        assertNull(vm.ui.value.price)
    }

    @Test fun `rapid source switches discard stale responses`() = runTest {
        var calls = 0
        val vm = MapSheetViewModel(SheetPriceSource { _, source, _ ->
            calls++
            kotlinx.coroutines.delay(200)
            SheetPrice("GASOLINE_REGULAR", "L", if (source == MapPriceSource.ANP) "5.99" else "6.10", source, "2026-10-20")
        })
        vm.select(marker(), MapPriceSource.ANP)
        advanceTimeBy(50)
        vm.select(marker(), MapPriceSource.COMMUNITY)
        advanceTimeBy(400)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(MapPriceSource.COMMUNITY, vm.ui.value.price?.source)
        assertEquals("6.10", vm.ui.value.price?.amount)
    }

    @Test fun `charging in anp explains missing tariff and offers switch`() = runTest {
        var lookups = 0
        val vm = MapSheetViewModel(SheetPriceSource { _, _, _ ->
            lookups++
            SheetPrice("TARIFF", "kWh", "1.20", MapPriceSource.COMMUNITY, "2026-10-20")
        })
        vm.select(marker(charging = true, fuel = false), MapPriceSource.ANP)
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(vm.ui.value.priceAbsent)
        assertEquals("anp-no-tariff", vm.ui.value.absentReason)
        assertEquals(0, lookups)
    }

    @Test fun `no-price locations stay discoverable`() = runTest {
        val vm = MapSheetViewModel(SheetPriceSource { _, _, _ -> null })
        vm.select(marker(), MapPriceSource.COMMUNITY)
        advanceTimeBy(200)
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(vm.ui.value.priceAbsent)
        assertEquals(marker().uuid, vm.ui.value.marker?.uuid)
    }

    @Test fun `selecting anp charger cancels in-flight fuel lookup and rejects late response`() = runTest {
        val entered = kotlinx.coroutines.CompletableDeferred<Unit>()
        val release = kotlinx.coroutines.CompletableDeferred<Unit>()
        val vm = MapSheetViewModel(object : SheetPriceSource {
            override suspend fun lookup(request: SheetPriceRequest): SheetPrice? {
                entered.complete(Unit)
                // Simulate a non-cooperative provider that ignores cancellation and completes late.
                kotlinx.coroutines.withContext(kotlinx.coroutines.NonCancellable) {
                    release.await()
                }
                return SheetPrice("GASOLINE_REGULAR", "L", "5.99", MapPriceSource.ANP, "2026-10-20")
            }

            override suspend fun lookup(stationUuid: String, source: MapPriceSource, marker: MapOverlayMarker): SheetPrice? =
                null
        })

        // 1. Select ANP fuel station and wait for lookup to start.
        vm.select(marker(uuid = "11111111-1111-4111-8111-111111111111", fuel = true, charging = false), MapPriceSource.ANP)
        advanceTimeBy(150)
        dispatcher.scheduler.runCurrent()
        assertTrue(entered.isCompleted)

        // 2. Select ANP charger; initial state must be anp-no-tariff.
        val charger = marker(uuid = "22222222-2222-4222-8222-222222222222", fuel = false, charging = true)
        vm.select(charger, MapPriceSource.ANP)
        dispatcher.scheduler.runCurrent()
        assertEquals(charger.uuid, vm.ui.value.marker?.uuid)
        assertTrue(vm.ui.value.priceAbsent)
        assertEquals("anp-no-tariff", vm.ui.value.absentReason)
        assertNull(vm.ui.value.price)

        // 3. Complete the old fuel query with 5.99/L.
        release.complete(Unit)
        dispatcher.scheduler.advanceUntilIdle()

        // 4. Charger must keep anp-no-tariff and never receive the old gasoline price.
        assertEquals(charger.uuid, vm.ui.value.marker?.uuid)
        assertTrue(vm.ui.value.priceAbsent)
        assertEquals("anp-no-tariff", vm.ui.value.absentReason)
        assertNull(vm.ui.value.price)
    }

    @Test fun `clear and debounce window invalidate non-cooperative in-flight lookups`() = runTest {
        val entered = kotlinx.coroutines.CompletableDeferred<Unit>()
        val release = kotlinx.coroutines.CompletableDeferred<Unit>()
        val vm = MapSheetViewModel(object : SheetPriceSource {
            override suspend fun lookup(request: SheetPriceRequest): SheetPrice? {
                entered.complete(Unit)
                kotlinx.coroutines.withContext(kotlinx.coroutines.NonCancellable) {
                    release.await()
                }
                return SheetPrice("GASOLINE_REGULAR", "L", "5.99", request.source, "2026-10-20")
            }

            override suspend fun lookup(stationUuid: String, source: MapPriceSource, marker: MapOverlayMarker): SheetPrice? =
                null
        })

        vm.select(marker(), MapPriceSource.ANP)
        advanceTimeBy(150)
        dispatcher.scheduler.runCurrent()
        assertTrue(entered.isCompleted)

        // Clear while lookup is in flight; late completion must not re-populate sheet.
        vm.clear()
        release.complete(Unit)
        dispatcher.scheduler.advanceUntilIdle()
        assertNull(vm.ui.value.marker)
        assertNull(vm.ui.value.price)
    }

    @Test fun `lookup failure sets load-failed reason distinct from no-price`() = runTest {
        val vm = MapSheetViewModel(SheetPriceSource { _, _, _ ->
            throw java.io.IOException("backend unreachable")
        })
        vm.select(marker(), MapPriceSource.COMMUNITY)
        advanceTimeBy(200)
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(vm.ui.value.priceAbsent)
        assertEquals("load-failed", vm.ui.value.absentReason)
        assertNull(vm.ui.value.price)
    }

    @Test fun `real sheet price source resolves anp community and charging prices without cross-source fallback`() = runTest {
        val prefs = io.mockk.mockk<com.anpfuel.domain.repository.UserPreferencesRepository>()
        val vehicles = io.mockk.mockk<com.anpfuel.domain.repository.VehicleRepository>()
        val detail = io.mockk.mockk<com.anpfuel.application.usecase.directory.GetServerStationDetailUseCase>()
        val stationPrices = io.mockk.mockk<com.anpfuel.domain.repository.StationPriceRepository>()
        val priceTables = io.mockk.mockk<com.anpfuel.domain.repository.PriceTableRepository>()
        val catalog = io.mockk.mockk<com.anpfuel.domain.repository.MunicipalityCatalogRepository>()
        val backendPrices = io.mockk.mockk<com.anpfuel.application.usecase.community.GetCommunityPriceGroupsUseCase>()
        val cityFeed = io.mockk.mockk<com.anpfuel.domain.community.CityCommunityFeedReader>()
        val charging = io.mockk.mockk<com.anpfuel.domain.charging.ChargingObservationGateway>()

        val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
        val week = com.anpfuel.domain.valueobject.SurveyWeek.fromIsoDates("2026-09-27", "2026-10-03")
        val serverStation = com.anpfuel.domain.discovery.ServerStation.create(
            stationId,
            "Posto Real",
            com.anpfuel.domain.discovery.StationLocationQuality.REVIEWED,
            -25.42,
            -49.27,
            "04218406000104",
            "4106902",
            "PR",
            null,
        )

        io.mockk.coEvery { prefs.getPreferences() } returns com.anpfuel.domain.model.UserPreferences(
            preferredState = com.anpfuel.domain.valueobject.BrazilianState.PARANA,
            preferredMunicipality = "Curitiba",
            preferredFuelProduct = com.anpfuel.domain.valueobject.FuelProduct.ETHANOL,
            activeSurveyWeek = week,
        )
        io.mockk.coEvery { vehicles.listAll() } returns emptyList()
        io.mockk.coEvery { detail(stationId) } returns com.anpfuel.application.usecase.directory.ServerStationDetailOutcome.Fresh(serverStation)
        io.mockk.coEvery { priceTables.getImportedPriceSurveys() } returns emptyList()
        io.mockk.coEvery { priceTables.findPriceSurveyByWeek(any()) } returns null
        io.mockk.coEvery { catalog.getCatalogMunicipalities(any()) } returns emptyList()
        io.mockk.coEvery { catalog.findCatalogEntry(any(), any()) } returns null

        val localRow = com.anpfuel.domain.model.StationPrice.create(
            priceSurveyId = com.anpfuel.domain.valueobject.DomainId.forSurveyWeek(week),
            surveyWeek = week,
            station = com.anpfuel.domain.model.RetailStation.create(
                com.anpfuel.domain.valueobject.Cnpj.parse("04218406000104"),
                "Posto Real LTDA",
                "Posto Real",
                "Av Brasil, 100",
                "Curitiba",
                com.anpfuel.domain.valueobject.BrazilianState.PARANA,
                "BR",
            ),
            fuelProduct = com.anpfuel.domain.valueobject.FuelProduct.ETHANOL,
            price = com.anpfuel.domain.valueobject.PriceAmount.of("3.89"),
            collectedAt = java.time.LocalDate.of(2026, 9, 29),
        )
        io.mockk.coEvery {
            stationPrices.findStationPrice(
                com.anpfuel.domain.valueobject.BrazilianState.PARANA,
                "Curitiba",
                com.anpfuel.domain.valueobject.FuelProduct.ETHANOL,
                week,
                "04218406000104",
            )
        } returns localRow

        val realSource = RealSheetPriceSource(
            userPreferences = prefs,
            vehicles = vehicles,
            stationDetail = detail,
            stationPrices = stationPrices,
            priceTables = priceTables,
            catalog = catalog,
            backendPrices = backendPrices,
            cityFeed = cityFeed,
            chargingObservations = charging,
        )

        val fuelMarker = marker(uuid = stationId, fuel = true, charging = false)

        // 1. ANP lookup returns exact station observation for the chosen survey period and product.
        val anpResult = realSource.lookup(
            SheetPriceRequest(
                stationUuid = stationId,
                source = MapPriceSource.ANP,
                marker = fuelMarker,
                fuelProduct = com.anpfuel.domain.valueobject.FuelProduct.ETHANOL,
                surveyPeriod = "2026-09-27/2026-10-03",
            )
        )
        assertEquals("ETHANOL", anpResult?.product)
        assertEquals("L", anpResult?.unit)
        assertEquals("R$ 3,89", anpResult?.amount)
        assertEquals(MapPriceSource.ANP, anpResult?.source)
        assertEquals("2026-09-27..2026-10-03 (2026-09-29)", anpResult?.dateLabel)

        // 2. ANP lookup for a mismatched survey period returns null (never silent cross-period fallback).
        io.mockk.coEvery {
            stationPrices.findStationPrice(any(), any(), com.anpfuel.domain.valueobject.FuelProduct.GASOLINE_REGULAR, any(), any())
        } returns null
        io.mockk.coEvery {
            stationPrices.hasStationData(any(), any(), any())
        } returns false
        val officialSection = com.anpfuel.domain.model.BackendOfficialSection.create(
            source = "ANP",
            amountMilliBrl = 5790L,
            currency = "BRL",
            collectedOn = "2026-09-20",
            surveyWeekStart = "2026-09-14",
            surveyWeekEnd = "2026-09-20",
            revisionId = "123e4567-e89b-12d3-a456-426614174000",
        )
        val officialGroup = com.anpfuel.domain.model.BackendPriceGroup.create(
            stationId = stationId,
            fuelProductWire = "GASOLINE_REGULAR",
            unit = "L",
            conditionKind = "STANDARD",
            official = officialSection,
            community = null,
        )
        io.mockk.coEvery {
            backendPrices(stationId, com.anpfuel.domain.valueobject.FuelProduct.GASOLINE_REGULAR)
        } returns com.anpfuel.application.usecase.community.CommunityPriceGroupsOutcome.Fresh(
            com.anpfuel.domain.model.BackendPriceGroups.create(
                stationId = stationId,
                fuelFilterWire = "GASOLINE_REGULAR",
                groups = listOf(officialGroup),
                fetchedAtMillis = 1000L,
                expiresAtMillis = 2000L,
            )
        )
        val mismatchedPeriodAnp = realSource.lookup(
            SheetPriceRequest(
                stationUuid = stationId,
                source = MapPriceSource.ANP,
                marker = fuelMarker,
                fuelProduct = com.anpfuel.domain.valueobject.FuelProduct.GASOLINE_REGULAR,
                surveyPeriod = "2026-09-27/2026-10-03",
            )
        )
        assertNull(mismatchedPeriodAnp)

        // 3. Community lookup returns validated non-expired projection and never falls back to ANP.
        val futureExpiry = java.time.Instant.now().plusSeconds(3600)
        val receivedAt = java.time.Instant.parse("2026-10-24T15:30:00Z")
        val communitySection = com.anpfuel.domain.model.BackendCommunitySection(
            source = "COMMUNITY",
            availability = "AVAILABLE",
            amountMilliBrl = 6150L,
            currency = "BRL",
            confidence = "HIGH",
            freshness = "FRESH",
            supporters = 3,
            confirmations = 4,
            representativeObservationId = "123e4567-e89b-12d3-a456-426614174001",
            lastReceivedAt = receivedAt,
            expiresAt = futureExpiry,
            algorithmVersion = "v1",
            projectionVersion = 1L,
        )
        val communityGroup = com.anpfuel.domain.model.BackendPriceGroup.create(
            stationId = stationId,
            fuelProductWire = "GASOLINE_REGULAR",
            unit = "L",
            conditionKind = "STANDARD",
            official = officialSection,
            community = communitySection,
        )
        io.mockk.coEvery {
            backendPrices(stationId, com.anpfuel.domain.valueobject.FuelProduct.GASOLINE_REGULAR)
        } returns com.anpfuel.application.usecase.community.CommunityPriceGroupsOutcome.Fresh(
            com.anpfuel.domain.model.BackendPriceGroups.create(
                stationId = stationId,
                fuelFilterWire = "GASOLINE_REGULAR",
                groups = listOf(communityGroup),
                fetchedAtMillis = 1000L,
                expiresAtMillis = 2000L,
            )
        )
        val communityResult = realSource.lookup(
            SheetPriceRequest(
                stationUuid = stationId,
                source = MapPriceSource.COMMUNITY,
                marker = fuelMarker,
                fuelProduct = com.anpfuel.domain.valueobject.FuelProduct.GASOLINE_REGULAR,
            )
        )
        assertEquals("GASOLINE_REGULAR", communityResult?.product)
        assertEquals("L", communityResult?.unit)
        assertEquals("R$ 6,150", communityResult?.amount)
        assertEquals(MapPriceSource.COMMUNITY, communityResult?.source)
        assertEquals("2026-10-24", communityResult?.dateLabel)

        // 4. Community charging lookup reads ChargingObservationGateway and formats kWh price.
        val chargerId = "33333333-3333-4333-8333-333333333333"
        val chargerMarker = marker(uuid = chargerId, fuel = false, charging = true)
        io.mockk.coEvery { charging.history(chargerId, 20) } returns listOf(
            com.anpfuel.domain.charging.CommunityChargingObservation(
                id = "44444444-4444-4444-8444-444444444444",
                connector = com.anpfuel.domain.charging.ChargingConnector.CCS2,
                priceMilliBrlPerKwh = 2450L,
                condition = com.anpfuel.domain.charging.ChargingCondition.STANDARD,
                observedDay = "2026-10-25",
                note = "Fast charger",
                helpful = 2L,
                notHelpful = 0L,
            )
        )
        val chargingResult = realSource.lookup(
            SheetPriceRequest(
                stationUuid = chargerId,
                source = MapPriceSource.COMMUNITY,
                marker = chargerMarker,
            )
        )
        assertEquals("CCS2", chargingResult?.product)
        assertEquals("kWh", chargingResult?.unit)
        assertEquals("R$ 2,450", chargingResult?.amount)
        assertEquals(MapPriceSource.COMMUNITY, chargingResult?.source)
        assertEquals("2026-10-25", chargingResult?.dateLabel)
    }
}
