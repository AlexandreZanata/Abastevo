package com.anpfuel.app.ui.map

import com.anpfuel.application.usecase.mapsaved.*
import com.anpfuel.domain.map.saved.*
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SavedViewModelTest {
    private val dispatcher = UnconfinedTestDispatcher()
    private val list = mockk<ListSavedUseCase>()
    private val toggle = mockk<ToggleStationFavoriteUseCase>()
    private val plan = mockk<SaveRoutePlanUseCase>()
    private val delete = mockk<DeleteSavedUseCase>()
    private val import = mockk<ImportExchangeUseCase>()
    private val export = mockk<ExportExchangeUseCase>()
    private val recorder = mockk<TrackRecordingUseCase>()
    private val owner = SavedOwner("guest", "o")
    private lateinit var vm: SavedViewModel

    @BeforeEach fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { list() } returns SavedOverview(owner, emptyList(), emptyList(), null)
        vm = SavedViewModel(list, toggle, plan, delete, import, export, recorder)
    }
    @AfterEach fun tearDown() { Dispatchers.resetMain() }

    @Test fun `domain refusals surface their code and generic failures never leak exception text`() {
        coEvery { import(any(), any()) } throws SavedValidationException("xml-doctype-refused")
        vm.importFile(ByteArray(3))
        assertEquals("xml-doctype-refused", vm.ui.value.message)
        coEvery { export(any(), any(), any()) } throws IllegalStateException("secret path /data/user/0/x")
        vm.prepareExport(ExchangeFormat.GPX)
        assertEquals("export-failed", vm.ui.value.message)
        assertNull(vm.ui.value.export)
    }

    @Test fun `deleted records pause the import and keep the bytes only until confirmed or dismissed`() {
        val bytes = byteArrayOf(1, 2, 3)
        coEvery { import(bytes, false) } throws SavedValidationException("deleted-records-present")
        coEvery { import(bytes, true) } returns ImportReport(1, 0, 0, false)
        vm.importFile(bytes)
        assertSame(bytes, vm.ui.value.pendingDeletedImport)
        vm.dismissDeletedImport()
        assertNull(vm.ui.value.pendingDeletedImport)
        coVerify(exactly = 0) { import(bytes, true) }
        vm.importFile(bytes)
        vm.importFile(bytes, confirmDeleted = true)
        assertNull(vm.ui.value.pendingDeletedImport)
        assertEquals("import-done", vm.ui.value.message)
    }

    @Test fun `import outcomes map to distinct messages`() {
        coEvery { import(any(), any()) } returns ImportReport(0, 0, 0, true)
        vm.importFile(ByteArray(1)); assertEquals("import-already", vm.ui.value.message)
        coEvery { import(any(), any()) } returns ImportReport(0, 0, 4, false)
        vm.importFile(ByteArray(1)); assertEquals("import-nothing-new", vm.ui.value.message)
    }

    @Test fun `favorite ids come from the stored places of the current owner`() {
        val place = SavedPlace("11111111-1111-4111-8111-111111111111", SavedPlaceKind.STATION, "P", GeoPoint(1, 1), "33333333-3333-4333-8333-333333333333", SavedSourceKind.ANP, null, null, 0)
        coEvery { list() } returns SavedOverview(owner, listOf(place), emptyList(), null)
        vm.refresh()
        assertEquals(setOf("33333333-3333-4333-8333-333333333333"), vm.ui.value.favoriteStationIds)
    }

    @Test fun `finishing a recording reports saved only when a track was stored`() {
        coEvery { recorder.stop(true) } returns null
        vm.finishRecording(save = true)
        assertEquals("recording-discarded", vm.ui.value.message)
    }
}
