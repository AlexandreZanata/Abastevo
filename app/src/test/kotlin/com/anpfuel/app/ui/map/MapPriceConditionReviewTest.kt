package com.anpfuel.app.ui.map

import com.anpfuel.application.usecase.community.CommunityPriceGroupsOutcome
import com.anpfuel.application.usecase.community.GetCommunityPriceGroupsUseCase
import com.anpfuel.application.usecase.directory.GetServerStationDetailUseCase
import com.anpfuel.application.usecase.directory.ServerStationDetailOutcome
import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.data.map.MapDistribution
import com.anpfuel.data.map.MapDistributionConfig
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.data.map.PACKAGE_READY
import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationGateway
import com.anpfuel.domain.charging.CommunityChargingObservation
import com.anpfuel.domain.community.CityCommunityFeedReader
import com.anpfuel.domain.community.CommunityFeedItem
import com.anpfuel.domain.community.CommunityFeedPage
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.model.BackendCommunitySection
import com.anpfuel.domain.model.BackendOfficialSection
import com.anpfuel.domain.model.BackendPriceGroup
import com.anpfuel.domain.model.BackendPriceGroups
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapStationLocationQuality
import io.mockk.coEvery
import io.mockk.mockk
import java.io.File
import java.nio.file.Files
import java.security.MessageDigest
import java.time.Instant
import java.util.concurrent.atomic.AtomicInteger
import kotlin.coroutines.CoroutineContext
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import okhttp3.OkHttpClient
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MapPriceConditionReviewTest {

    private val stationId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
    private val now: Instant = Instant.parse("2026-10-25T12:00:00Z")

    private fun marker(
        id: String = stationId,
        fuel: Boolean = true,
        charging: Boolean = false,
    ) = MapOverlayMarker(
        uuid = id,
        latitude = -25.42,
        longitude = -49.27,
        title = "Station",
        fuel = fuel,
        charging = charging,
        locationQuality = MapStationLocationQuality.REVIEWED,
    )

    private fun communitySection(
        amountMilliBrl: Long? = 5990L,
        availability: String = "AVAILABLE",
        receivedAt: Instant? = now.minusSeconds(60),
        expiresAt: Instant? = now.plusSeconds(3600),
    ) = BackendCommunitySection(
        source = "COMMUNITY",
        availability = availability,
        amountMilliBrl = amountMilliBrl,
        currency = "BRL",
        confidence = if (availability == "AVAILABLE") "HIGH" else "",
        freshness = if (availability == "AVAILABLE") "FRESH" else "UNKNOWN",
        supporters = 3,
        confirmations = 3,
        representativeObservationId = null,
        lastReceivedAt = receivedAt,
        expiresAt = expiresAt,
        algorithmVersion = "v1",
        projectionVersion = 1L,
    )

    @Test
    fun loyaltyPriceMustCarryItsConditionOrBeAbsent() = runBlocking {
        val prefs = mockk<UserPreferencesRepository>()
        coEvery { prefs.getPreferences() } returns UserPreferences(preferredFuelProduct = FuelProduct.GASOLINE_REGULAR)
        val backend = mockk<GetCommunityPriceGroupsUseCase>()
        val comm = communitySection(amountMilliBrl = 5990L)
        val group = BackendPriceGroup.create(stationId, "GASOLINE_REGULAR", "L", "LOYALTY", null, comm)
        coEvery { backend(stationId, FuelProduct.GASOLINE_REGULAR) } returns CommunityPriceGroupsOutcome.Fresh(
            BackendPriceGroups.create(stationId, "GASOLINE_REGULAR", listOf(group), 0L, Long.MAX_VALUE),
        )
        val detail = mockk<GetServerStationDetailUseCase>()
        coEvery { detail(any()) } returns ServerStationDetailOutcome.Disabled
        val source = RealSheetPriceSource(prefs, mockk(), detail, mockk(), mockk(), mockk(), backend, mockk(), mockk())
        source.nowProvider = { now }

        val price = source.lookup(
            SheetPriceRequest(stationId, MapPriceSource.COMMUNITY, marker(), FuelProduct.GASOLINE_REGULAR),
        )
        assertNotNull(price)
        assertTrue(
            price == null || price.toString().contains("LOYALTY"),
            "Conditional price was exposed with no condition information: $price",
        )
        assertEquals("LOYALTY", price!!.conditionKind)
        assertTrue(price.isConditional)
        assertTrue(price.conditionSummary.contains("LOYALTY"))
    }

    @Test
    fun multipleGroupsPreferStandardAndUnavailableStandardYieldsToAvailableConditional() = runBlocking {
        val prefs = mockk<UserPreferencesRepository>()
        coEvery { prefs.getPreferences() } returns UserPreferences(preferredFuelProduct = FuelProduct.GASOLINE_REGULAR)
        val detail = mockk<GetServerStationDetailUseCase>()
        coEvery { detail(any()) } returns ServerStationDetailOutcome.Disabled
        val backend = mockk<GetCommunityPriceGroupsUseCase>()

        // Case 1: Both LOYALTY (cheaper, 5790) and STANDARD (6150) are AVAILABLE -> STANDARD wins.
        val loyaltyGroup = BackendPriceGroup.create(
            stationId, "GASOLINE_REGULAR", "L", "LOYALTY", null, communitySection(5790L),
        )
        val standardGroup = BackendPriceGroup.create(
            stationId, "GASOLINE_REGULAR", "L", "STANDARD", null, communitySection(6150L),
        )
        coEvery { backend(stationId, FuelProduct.GASOLINE_REGULAR) } returns CommunityPriceGroupsOutcome.Fresh(
            BackendPriceGroups.create(
                stationId,
                "GASOLINE_REGULAR",
                listOf(loyaltyGroup, standardGroup),
                0L,
                Long.MAX_VALUE,
            ),
        )
        val source = RealSheetPriceSource(prefs, mockk(), detail, mockk(), mockk(), mockk(), backend, mockk(), mockk())
        source.nowProvider = { now }

        val standardResult = source.lookup(
            SheetPriceRequest(stationId, MapPriceSource.COMMUNITY, marker(), FuelProduct.GASOLINE_REGULAR),
        )
        assertNotNull(standardResult)
        assertEquals("STANDARD", standardResult!!.conditionKind)
        assertFalse(standardResult.isConditional)
        assertEquals("R$ 6,150", standardResult.amount)

        // Case 2: STANDARD is UNKNOWN/unavailable while APP (5850) and OTHER (5500) are AVAILABLE.
        // Unavailable STANDARD must not shadow APP, and APP must beat OTHER while keeping conditionKind="APP".
        val unavailableStandard = BackendPriceGroup.create(
            stationId,
            "GASOLINE_REGULAR",
            "L",
            "STANDARD",
            null,
            communitySection(amountMilliBrl = null, availability = "UNKNOWN", receivedAt = null, expiresAt = null),
        )
        val otherGroup = BackendPriceGroup.create(
            stationId, "GASOLINE_REGULAR", "L", "OTHER", null, communitySection(5500L),
        )
        val appGroup = BackendPriceGroup.create(
            stationId, "GASOLINE_REGULAR", "L", "APP", null, communitySection(5850L),
        )
        coEvery { backend(stationId, FuelProduct.GASOLINE_REGULAR) } returns CommunityPriceGroupsOutcome.Fresh(
            BackendPriceGroups.create(
                stationId,
                "GASOLINE_REGULAR",
                listOf(unavailableStandard, otherGroup, appGroup),
                0L,
                Long.MAX_VALUE,
            ),
        )

        val conditionalResult = source.lookup(
            SheetPriceRequest(stationId, MapPriceSource.COMMUNITY, marker(), FuelProduct.GASOLINE_REGULAR),
        )
        assertNotNull(conditionalResult)
        assertEquals("APP", conditionalResult!!.conditionKind)
        assertTrue(conditionalResult.isConditional)
        assertEquals("R$ 5,850", conditionalResult.amount)
        assertTrue(conditionalResult.conditionSummary.contains("APP"))
    }

    @Test
    fun incompatibleUnitsAndDivergentProductsOrPeriodsAreRejected() = runBlocking {
        val activeWeek = SurveyWeek.fromIsoDates("2026-09-27", "2026-10-03")
        val prefs = mockk<UserPreferencesRepository>()
        coEvery { prefs.getPreferences() } returns UserPreferences(
            preferredState = BrazilianState.PARANA,
            preferredMunicipality = "Curitiba",
            preferredFuelProduct = FuelProduct.GASOLINE_REGULAR,
            activeSurveyWeek = activeWeek,
        )
        val vehicles = mockk<VehicleRepository>()
        coEvery { vehicles.listAll() } returns emptyList()
        val detail = mockk<GetServerStationDetailUseCase>()
        val serverStation = ServerStation.create(
            stationId,
            "Posto Curitiba",
            StationLocationQuality.REVIEWED,
            -25.42,
            -49.27,
            "04218406000104",
            "4106902",
            "PR",
            null,
        )
        coEvery { detail(stationId) } returns ServerStationDetailOutcome.Fresh(serverStation)
        val stationPrices = mockk<StationPriceRepository>()
        coEvery { stationPrices.findStationPrice(any(), any(), any(), any(), any()) } returns null
        coEvery { stationPrices.hasStationData(any(), any(), any()) } returns false
        val priceTables = mockk<PriceTableRepository>()
        coEvery { priceTables.getImportedPriceSurveys() } returns emptyList()
        coEvery { priceTables.findPriceSurveyByWeek(any()) } returns null
        val catalog = mockk<MunicipalityCatalogRepository>()
        coEvery { catalog.getCatalogMunicipalities(any()) } returns emptyList()
        coEvery { catalog.findCatalogEntry(any(), any()) } returns null
        val backend = mockk<GetCommunityPriceGroupsUseCase>()
        val cityFeed = mockk<CityCommunityFeedReader>()
        val charging = mockk<ChargingObservationGateway>()

        val source = RealSheetPriceSource(
            prefs, vehicles, detail, stationPrices, priceTables, catalog, backend, cityFeed, charging,
        )
        source.nowProvider = { now }

        // 1. Incompatible unit on GASOLINE_REGULAR (M3 instead of L) in Community groups must be rejected.
        val wrongUnitCommunity = BackendPriceGroup.create(
            stationId, "GASOLINE_REGULAR", "M3", "STANDARD", null, communitySection(5990L),
        )
        // Also include a divergent product group (ETHANOL with L) in the response for GASOLINE_REGULAR.
        val divergentProductGroup = BackendPriceGroup.create(
            stationId, "ETHANOL", "L", "STANDARD", null, communitySection(3990L),
        )
        coEvery { backend(stationId, FuelProduct.GASOLINE_REGULAR) } returns CommunityPriceGroupsOutcome.Fresh(
            BackendPriceGroups.create(
                stationId,
                "GASOLINE_REGULAR",
                listOf(wrongUnitCommunity, divergentProductGroup),
                0L,
                Long.MAX_VALUE,
            ),
        )
        // Feed fallback returns an ETHANOL item when GASOLINE_REGULAR was queried -> must be rejected.
        coEvery { cityFeed.read(any(), any()) } returns CommunityFeedPage(
            items = listOf(
                CommunityFeedItem(
                    stationId = stationId,
                    stationName = "Posto Curitiba",
                    fuel = FuelProduct.ETHANOL,
                    amountMilliBrl = 3990L,
                    updatedAt = now.minusSeconds(120),
                    expiresAt = now.plusSeconds(3600),
                    supporters = 2,
                    confirmations = 2,
                    confidence = "HIGH",
                    version = 1L,
                ),
            ),
            nextCursor = null,
            generatedAt = now,
        )

        val rejectedCommunity = source.lookup(
            SheetPriceRequest(stationId, MapPriceSource.COMMUNITY, marker(), FuelProduct.GASOLINE_REGULAR),
        )
        assertNull(rejectedCommunity)

        // 2. Incompatible unit on CNG (L instead of M3) in ANP official section must be rejected.
        val officialSection = BackendOfficialSection.create(
            source = "ANP",
            amountMilliBrl = 4890L,
            currency = "BRL",
            collectedOn = "2026-09-29",
            surveyWeekStart = "2026-09-27",
            surveyWeekEnd = "2026-10-03",
            revisionId = "rev-1",
        )
        val wrongUnitAnp = BackendPriceGroup.create(
            stationId, "CNG", "L", "STANDARD", officialSection, null,
        )
        coEvery { backend(stationId, FuelProduct.CNG) } returns CommunityPriceGroupsOutcome.Fresh(
            BackendPriceGroups.create(stationId, "CNG", listOf(wrongUnitAnp), 0L, Long.MAX_VALUE),
        )
        val rejectedAnpUnit = source.lookup(
            SheetPriceRequest(
                stationUuid = stationId,
                source = MapPriceSource.ANP,
                marker = marker(),
                fuelProduct = FuelProduct.CNG,
                surveyPeriod = "2026-09-27..2026-10-03",
            ),
        )
        assertNull(rejectedAnpUnit)

        // 3. Malformed non-blank surveyPeriod must NOT fall back to prefs.activeSurveyWeek.
        val validAnpGroup = BackendPriceGroup.create(
            stationId, "GASOLINE_REGULAR", "L", "STANDARD", officialSection, null,
        )
        coEvery { backend(stationId, FuelProduct.GASOLINE_REGULAR) } returns CommunityPriceGroupsOutcome.Fresh(
            BackendPriceGroups.create(stationId, "GASOLINE_REGULAR", listOf(validAnpGroup), 0L, Long.MAX_VALUE),
        )
        val malformedPeriodResult = source.lookup(
            SheetPriceRequest(
                stationUuid = stationId,
                source = MapPriceSource.ANP,
                marker = marker(),
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                surveyPeriod = "malformed-period",
            ),
        )
        assertNull(malformedPeriodResult)

        // 4. Combined fuel+charging station with no fuel price must NOT return a charging kWh price when fuel was requested.
        coEvery { backend(stationId, FuelProduct.GASOLINE_REGULAR) } returns CommunityPriceGroupsOutcome.Fresh(
            BackendPriceGroups.create(stationId, "GASOLINE_REGULAR", emptyList(), 0L, Long.MAX_VALUE),
        )
        coEvery { cityFeed.read(any(), any()) } returns CommunityFeedPage(emptyList(), null, now)
        coEvery { charging.history(stationId, 20) } returns listOf(
            CommunityChargingObservation(
                id = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                connector = ChargingConnector.CCS2,
                priceMilliBrlPerKwh = 2400L,
                condition = ChargingCondition.STANDARD,
                observedDay = "2026-10-25",
                note = "Fast",
                helpful = 1L,
                notHelpful = 0L,
            ),
        )
        val combinedMarker = marker(fuel = true, charging = true)
        val fuelOnCombinedStation = source.lookup(
            SheetPriceRequest(
                stationUuid = stationId,
                source = MapPriceSource.COMMUNITY,
                marker = combinedMarker,
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
            ),
        )
        assertNull(fuelOnCombinedStation)
    }

    @Test
    fun chargingObservationsPreserveConditionAndEnforceConnectorCompatibility() = runBlocking {
        val prefs = mockk<UserPreferencesRepository>()
        coEvery { prefs.getPreferences() } returns UserPreferences(preferredFuelProduct = FuelProduct.GASOLINE_REGULAR)
        val charging = mockk<ChargingObservationGateway>()
        val source = RealSheetPriceSource(
            prefs, mockk(), mockk(), mockk(), mockk(), mockk(), mockk(), mockk(), charging,
        )
        source.nowProvider = { now }
        val chargerMarker = marker(fuel = false, charging = true)

        // 1. Conditional-only charging observation (MEMBER) preserves conditionKind="MEMBER" and never converts to STANDARD.
        coEvery { charging.history(stationId, 20) } returns listOf(
            CommunityChargingObservation(
                id = "11111111-1111-4111-8111-111111111111",
                connector = ChargingConnector.CCS2,
                priceMilliBrlPerKwh = 1950L,
                condition = ChargingCondition.MEMBER,
                observedDay = "2026-10-25",
                note = "Member tariff",
                helpful = 2L,
                notHelpful = 0L,
            ),
        )
        val memberResult = source.lookup(
            SheetPriceRequest(stationId, MapPriceSource.COMMUNITY, chargerMarker),
        )
        assertNotNull(memberResult)
        assertEquals("CCS2", memberResult!!.product)
        assertEquals("kWh", memberResult.unit)
        assertEquals("R$ 1,950", memberResult.amount)
        assertEquals("MEMBER", memberResult.conditionKind)
        assertTrue(memberResult.isConditional)

        // 2. Connector filter compatibility: requesting TYPE2 when only CCS2 exists returns null;
        // when TYPE2 (OFF_PEAK) and CCS2 (STANDARD) exist, requesting TYPE2 returns the TYPE2 observation with OFF_PEAK.
        val missingType2 = source.lookup(
            SheetPriceRequest(
                stationUuid = stationId,
                source = MapPriceSource.COMMUNITY,
                marker = chargerMarker,
                chargingConnector = ChargingConnector.TYPE2,
            ),
        )
        assertNull(missingType2)

        coEvery { charging.history(stationId, 20) } returns listOf(
            CommunityChargingObservation(
                id = "11111111-1111-4111-8111-111111111111",
                connector = ChargingConnector.CCS2,
                priceMilliBrlPerKwh = 2500L,
                condition = ChargingCondition.STANDARD,
                observedDay = "2026-10-25",
                note = "CCS2 standard",
                helpful = 3L,
                notHelpful = 0L,
            ),
            CommunityChargingObservation(
                id = "22222222-2222-4222-8222-222222222222",
                connector = ChargingConnector.TYPE2,
                priceMilliBrlPerKwh = 1750L,
                condition = ChargingCondition.OFF_PEAK,
                observedDay = "2026-10-25",
                note = "Type2 off-peak",
                helpful = 1L,
                notHelpful = 0L,
            ),
            CommunityChargingObservation(
                id = "33333333-3333-4333-8333-333333333333",
                connector = ChargingConnector.TYPE2,
                priceMilliBrlPerKwh = 1500L,
                condition = ChargingCondition.UNKNOWN,
                observedDay = "2026-10-25",
                note = "Unknown condition must be ignored",
                helpful = 1L,
                notHelpful = 0L,
            ),
        )
        val type2Result = source.lookup(
            SheetPriceRequest(
                stationUuid = stationId,
                source = MapPriceSource.COMMUNITY,
                marker = chargerMarker,
                chargingConnector = ChargingConnector.TYPE2,
            ),
        )
        assertNotNull(type2Result)
        assertEquals("TYPE2", type2Result!!.product)
        assertEquals("R$ 1,750", type2Result.amount)
        assertEquals("OFF_PEAK", type2Result.conditionKind)
        assertTrue(type2Result.isConditional)
    }

    @OptIn(ExperimentalCoroutinesApi::class)
    @Test
    fun viewModelAvailabilityAndRefreshOffloadRepositoryDiskChecksToIoDispatcher() = runTest {
        val mainDispatcher = StandardTestDispatcher(testScheduler)
        Dispatchers.setMain(mainDispatcher)
        val root = Files.createTempDirectory("vm-io-proof").toFile()
        try {
            val ioDispatches = AtomicInteger(0)
            val trackingIoDispatcher = object : CoroutineDispatcher() {
                override fun dispatch(context: CoroutineContext, block: Runnable) {
                    ioDispatches.incrementAndGet()
                    mainDispatcher.dispatch(context, block)
                }
            }
            val bytes = ByteArray(64 * 1024) { (it and 0xff).toByte() }
            val sha = MessageDigest.getInstance("SHA-256").digest(bytes)
                .joinToString("") { "%02x".format(it) }
            val store = object : MapPackageStore {
                var active: MapGenerationEntity? = MapGenerationEntity("g1", "m", true, 1L, "2601")
                val pkgs = listOf(
                    MapPackageEntity("p1", "g1", "REGION", "2601/rect.mwm", bytes.size.toLong(), sha, null, PACKAGE_READY, bytes.size.toLong(), null, 1L),
                )
                override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
                override suspend fun generation(generation: String) = active?.takeIf { it.generation == generation }
                override suspend fun activeGeneration() = active
                override suspend fun packages(generation: String) = if (generation == "g1") pkgs else emptyList()
                override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
                override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
                override suspend fun deleteInactive(generation: String) = Unit
            }
            val nativeFile = File(root, "2601/rect.mwm").apply {
                parentFile?.mkdirs()
                writeBytes(bytes)
            }
            assertTrue(nativeFile.isFile)

            val repo = MapPackageRepository(
                store = store,
                filesRoot = File(root, "maps"),
                http = OkHttpClient(),
                nativeRoot = root,
                ioDispatcher = trackingIoDispatcher,
            )

            // 1. MapScreenViewModel.refreshAvailability() dispatches publishedState() to ioDispatcher
            // and reuses the verified stamp on repeated resume checks without re-hashing full file bytes.
            val screenVm = MapScreenViewModel(store, repo)
            testScheduler.advanceUntilIdle()
            assertFalse(screenVm.ui.value.noPackage)
            assertTrue(ioDispatches.get() >= 1)
            assertEquals(1, repo.fullSha256ComputationCount())

            screenVm.refreshAvailability()
            testScheduler.advanceUntilIdle()
            assertEquals(1, repo.fullSha256ComputationCount())

            // 2. MapDownloadsViewModel.refresh() also offloads publishedState() to ioDispatcher.
            val beforeDownloadsIo = ioDispatches.get()
            val cfg = MapDistributionConfig("https://example.test", "test-region", mapOf("k" to ByteArray(32)), false)
            val dist = object : MapDistribution(OkHttpClient(), cfg) {
                override suspend fun fetchEnvelope(): ByteArray = error("unused")
            }
            val downloadsVm = MapDownloadsViewModel(cfg, dist, repo, store, object : MapWorkControl {
                override fun enqueue(generation: String, requireUnmetered: Boolean) = Unit
                override fun cancel(generation: String) = Unit
            }, object : com.anpfuel.domain.repository.UserPreferencesRepository {
                override suspend fun getPreferences() = com.anpfuel.domain.model.UserPreferences()
                override suspend fun savePreferences(preferences: com.anpfuel.domain.model.UserPreferences) = Unit
            })
            testScheduler.advanceUntilIdle()
            assertTrue(downloadsVm.ui.value.published)
            assertTrue(ioDispatches.get() > beforeDownloadsIo)
            assertEquals(1, repo.fullSha256ComputationCount())
        } finally {
            Dispatchers.resetMain()
            root.deleteRecursively()
        }
    }
}
