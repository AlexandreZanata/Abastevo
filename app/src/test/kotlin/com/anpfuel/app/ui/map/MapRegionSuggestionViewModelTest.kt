package com.anpfuel.app.ui.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.data.map.MapPackageStore
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapRegionSuggestionViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    private val emptyStore = object : MapPackageStore {
        override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
        override suspend fun generation(generation: String) = null
        override suspend fun activeGeneration() = null
        override suspend fun packages(generation: String) = emptyList<MapPackageEntity>()
        override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
        override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
        override suspend fun deleteInactive(generation: String) = Unit
    }

    private class FakeRegions(
        var configured: Boolean = true,
        var unmetered: Boolean = true,
        var active: String? = null,
        var bytes: suspend (String) -> Long = { 53_816_563L + 53_519_450L + 8_489_900L },
    ) : MapRegionServices {
        val dismissed = mutableSetOf<String>()
        val selected = mutableListOf<String?>()
        val requested = mutableListOf<String>()
        val fetched = mutableListOf<String>()
        override val downloadsConfigured: Boolean get() = configured
        override suspend fun regionFor(latitude: Double, longitude: Double): String? = when {
            latitude in -18.0..-7.0 && longitude in -61.0..-50.0 -> "Brazil_Mato Grosso"
            latitude in -26.0..-25.0 && longitude in -50.0..-48.0 -> "Brazil_Parana_East"
            else -> null
        }
        override fun isDismissed(slug: String) = slug in dismissed
        override fun dismiss(slug: String) { dismissed += slug }
        override fun select(slug: String?) { selected += slug }
        override fun requestDownload(slug: String) { requested += slug }
        override fun canFetchManifestAutomatically() = unmetered
        override suspend fun regionDownloadBytes(slug: String): Long {
            fetched += slug
            return bytes(slug)
        }
        override suspend fun activeRegionSlug() = active
    }

    @BeforeEach fun setup() = Dispatchers.setMain(dispatcher)

    @AfterEach fun tearDown() = Dispatchers.resetMain()

    private fun vm(regions: FakeRegions) = MapScreenViewModel(emptyStore, regions = regions)

    @Test fun `opened city without offline package suggests its region with the manifest size`() = runTest {
        val regions = FakeRegions()
        val vm = vm(regions)
        vm.onCityPoint(-12.545, -55.711)
        dispatcher.scheduler.advanceUntilIdle()
        val suggestion = vm.ui.value.regionSuggestion
        assertEquals("Mato Grosso", suggestion?.region?.name)
        assertEquals("brazil-mato-grosso", suggestion?.region?.slug)
        assertEquals(115_825_913L, suggestion?.bytes)
        assertEquals(listOf("brazil-mato-grosso"), regions.fetched)
        assertEquals("brazil-mato-grosso", regions.selected.last())
    }

    @Test fun `metered network shows the offer without fetching the manifest`() = runTest {
        val regions = FakeRegions(unmetered = false)
        val vm = vm(regions)
        vm.onCityPoint(-12.545, -55.711)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("Mato Grosso", vm.ui.value.regionSuggestion?.region?.name)
        assertNull(vm.ui.value.regionSuggestion?.bytes)
        assertTrue(regions.fetched.isEmpty())
    }

    @Test fun `manifest failure keeps the offer without a size`() = runTest {
        val regions = FakeRegions(bytes = { throw IOException("offline") })
        val vm = vm(regions)
        vm.onCityPoint(-12.545, -55.711)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("brazil-mato-grosso", vm.ui.value.regionSuggestion?.region?.slug)
        assertNull(vm.ui.value.regionSuggestion?.bytes)
    }

    @Test fun `active package of the same region suppresses the offer, another region offers the switch`() = runTest {
        val regions = FakeRegions(active = "brazil-mato-grosso")
        val vm = vm(regions)
        vm.onCityPoint(-12.545, -55.711)
        dispatcher.scheduler.advanceUntilIdle()
        assertNull(vm.ui.value.regionSuggestion)
        assertEquals("brazil-mato-grosso", vm.ui.value.region?.slug)
        vm.onCityPoint(-25.4284, -49.2733)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("brazil-parana-east", vm.ui.value.regionSuggestion?.region?.slug)
    }

    @Test fun `outside Brazil or unconfigured never offers`() = runTest {
        val regions = FakeRegions()
        val vm = vm(regions)
        vm.onCityPoint(-34.6, -58.4)
        dispatcher.scheduler.advanceUntilIdle()
        assertNull(vm.ui.value.region)
        assertNull(vm.ui.value.regionSuggestion)
        assertNull(regions.selected.last())
        val unconfigured = FakeRegions(configured = false)
        val vm2 = vm(unconfigured)
        vm2.onCityPoint(-12.545, -55.711)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("brazil-mato-grosso", vm2.ui.value.region?.slug)
        assertNull(vm2.ui.value.regionSuggestion)
        vm2.onCityPoint(null, null)
        dispatcher.scheduler.advanceUntilIdle()
        assertNull(vm2.ui.value.region)
    }

    @Test fun `not now persists per region and download requests that region only`() = runTest {
        val regions = FakeRegions()
        val vm = vm(regions)
        vm.onCityPoint(-12.545, -55.711)
        dispatcher.scheduler.advanceUntilIdle()
        vm.dismissRegionSuggestion()
        assertNull(vm.ui.value.regionSuggestion)
        assertEquals(setOf("brazil-mato-grosso"), regions.dismissed)
        // A new screen instance honours the persisted dismissal.
        val again = vm(regions)
        again.onCityPoint(-12.545, -55.711)
        dispatcher.scheduler.advanceUntilIdle()
        assertNull(again.ui.value.regionSuggestion)
        // Another region still gets its own offer.
        again.onCityPoint(-25.4284, -49.2733)
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(again.acceptRegionSuggestion())
        assertEquals(listOf("brazil-parana-east"), regions.requested)
    }

    @Test fun `without region services the screen keeps the legacy behaviour`() = runTest {
        val vm = MapScreenViewModel(emptyStore)
        vm.onCityPoint(-12.545, -55.711)
        dispatcher.scheduler.advanceUntilIdle()
        assertNull(vm.ui.value.region)
        assertFalse(vm.acceptRegionSuggestion())
    }

    @Test fun `selection hands the region to the downloads screen once`() {
        val selection = MapRegionSelection()
        selection.select("brazil-bahia")
        assertFalse(selection.consumeDownloadRequest("brazil-bahia"))
        selection.requestDownload("brazil-mato-grosso")
        assertEquals("brazil-mato-grosso", selection.selected.value)
        assertFalse(selection.consumeDownloadRequest("brazil-bahia"))
        assertTrue(selection.consumeDownloadRequest("brazil-mato-grosso"))
        assertFalse(selection.consumeDownloadRequest("brazil-mato-grosso"))
    }
}
