package com.anpfuel.app.ui.map

import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import com.anpfuel.data.map.MapPackageStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MapScreenViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    private fun store(active: MapGenerationEntity? = null) = object : MapPackageStore {
        override suspend fun insertGeneration(generation: MapGenerationEntity, packages: List<MapPackageEntity>) = Unit
        override suspend fun generation(generation: String) = null
        override suspend fun activeGeneration() = active
        override suspend fun packages(generation: String) = emptyList<MapPackageEntity>()
        override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
        override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
        override suspend fun deleteInactive(generation: String) = Unit
    }

    @BeforeEach fun setup() {
        Dispatchers.setMain(dispatcher)
    }

    @AfterEach fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test fun `default source is ANP with no package banner`() = runTest {
        val vm = MapScreenViewModel(store())
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(MapPriceSource.ANP, vm.ui.value.source)
        assertEquals(true, vm.ui.value.noPackage)
        assertNull(vm.ui.value.error)
    }

    @Test fun `active generation clears the banner`() = runTest {
        val vm = MapScreenViewModel(store(MapGenerationEntity("g1", "hash", true, 1L)))
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(false, vm.ui.value.noPackage)
    }

    @Test fun `source switch is explicit`() = runTest {
        val vm = MapScreenViewModel(store())
        vm.selectSource(MapPriceSource.COMMUNITY)
        assertEquals(MapPriceSource.COMMUNITY, vm.ui.value.source)
        vm.selectSource(MapPriceSource.ANP)
        assertEquals(MapPriceSource.ANP, vm.ui.value.source)
    }

    @Test fun `retry clears error`() = runTest {
        val vm = MapScreenViewModel(store())
        dispatcher.scheduler.advanceUntilIdle()
        vm.retry()
        dispatcher.scheduler.advanceUntilIdle()
        assertNull(vm.ui.value.error)
    }
}
