package com.anpfuel.app.ui.map

import android.content.Context
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.dp
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.R
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.mapsaved.*
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.saved.RoomSavedStore
import com.anpfuel.domain.map.saved.*
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapStationLocationQuality
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * MAP-25 real UI -> ViewModel -> use cases -> Room on a device (in-memory database, synthetic data, no network).
 * The system file chooser is not driven here; its bytes are produced/consumed by the same use cases.
 */
@RunWith(AndroidJUnit4::class)
class SavedPlacesDeviceTest {
    @get:Rule val compose = createComposeRule()
    private lateinit var context: Context
    private lateinit var db: AnpFuelDatabase
    private lateinit var store: RoomSavedStore
    private lateinit var vm: SavedViewModel
    private var signedIn: String? = null
    private val origin = "https://api.example.test"
    private val stationId = "33333333-3333-4333-8333-333333333333"

    @Before fun prepare() {
        context = ApplicationProvider.getApplicationContext()
        db = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java).build()
        store = RoomSavedStore(db)
        val sessions = object : AuthSessionStore {
            override fun save(session: PortableAuth.Session) = Unit
            override fun load(): PortableAuth.Session? = signedIn?.let { PortableAuth.Session("fam", it, "a", "r", 0, 0) }
            override fun clear() = Unit
        }
        val owners = SavedOwnerResolver(sessions, origin)
        vm = SavedViewModel(
            ListSavedUseCase(store, owners), ToggleStationFavoriteUseCase(store, owners), SaveRoutePlanUseCase(store, owners),
            DeleteSavedUseCase(store, owners), ImportExchangeUseCase(store, owners), ExportExchangeUseCase(store, owners),
            TrackRecordingUseCase(store, owners),
        )
    }

    @After fun finish() { db.close() }

    private fun text(id: Int, vararg args: Any) = context.getString(id, *args)
    private val guest get() = SavedOwner(SavedOwner.GUEST, origin)

    @Test fun stationSheetFavoriteTogglesAndPersistsForTheOwnerOnly() {
        val marker = MapOverlayMarker(stationId, -25.4295, -49.2719, "Posto Sintético", locationQuality = MapStationLocationQuality.REVIEWED)
        val sheetVm = MapSheetViewModel(SheetPriceSource { _, _, _ -> null })
        compose.setContent {
            MapScreen(
                onOpenStation = { _, _ -> }, onOpenSearch = {}, onOpenDownloads = {},
                viewModel = MapScreenViewModel(object : com.anpfuel.data.map.MapPackageStore {
                    override suspend fun insertGeneration(generation: com.anpfuel.data.local.map.MapGenerationEntity, packages: List<com.anpfuel.data.local.map.MapPackageEntity>) = Unit
                    override suspend fun generation(generation: String): com.anpfuel.data.local.map.MapGenerationEntity? = null
                    override suspend fun activeGeneration(): com.anpfuel.data.local.map.MapGenerationEntity? = null
                    override suspend fun packages(generation: String) = emptyList<com.anpfuel.data.local.map.MapPackageEntity>()
                    override suspend fun updatePackage(generation: String, packageId: String, status: String, downloadedBytes: Long, etag: String?, error: String?, updatedAtMillis: Long) = Unit
                    override suspend fun activate(generation: String, activatedAtMillis: Long) = Unit
                    override suspend fun deleteInactive(generation: String) = Unit
                }),
                sheet = sheetVm, saved = vm, guidance = testGuidance(),
                mapContent = { Box(Modifier.size(10.dp)) },
            )
        }
        compose.runOnIdle { sheetVm.select(marker, MapPriceSource.ANP) }
        compose.waitForIdle()
        compose.onNodeWithText(text(R.string.map25_favorite_add)).assertIsDisplayed()
        compose.onNodeWithTag("map-sheet-favorite").performScrollTo().performClick()
        compose.waitUntil(5_000) { runBlocking { store.stationFavorite(guest, stationId) } != null }
        compose.onNodeWithText(text(R.string.map25_favorite_remove)).assertIsDisplayed()
        assertEquals("Posto Sintético", runBlocking { store.stationFavorite(guest, stationId)!!.name })
        // Another account does not see it.
        assertNull(runBlocking { store.stationFavorite(SavedOwner("acct-b", origin), stationId) })
        compose.onNodeWithTag("map-sheet-favorite").performScrollTo().performClick()
        compose.waitUntil(5_000) { runBlocking { store.stationFavorite(guest, stationId) } == null }
        compose.onNodeWithText(text(R.string.map25_favorite_add)).assertIsDisplayed()
    }

    private fun gpx(vararg names: String) = GeoExchange.writeGpx(
        names.mapIndexed { i, n -> SavedPlace(String.format("00000000-0000-4000-8000-%012d", i + 1), SavedPlaceKind.PLACE, n, GeoPoint(i.toLong() + 1, 1), null, SavedSourceKind.MANUAL, null, null, 0) },
        emptyList(),
    )

    @Test fun savedScreenListsImportsRefusesMaliciousFilesAndNeedsConfirmationForDeletedItems() {
        compose.setContent { SavedScreen(onBack = {}, viewModel = vm) }
        compose.onNodeWithText(text(R.string.map25_privacy)).assertIsDisplayed()
        compose.onNodeWithTag("saved-record-start").assertIsDisplayed()
        // A malicious file is refused and nothing is stored.
        val xxe = """<?xml version="1.0"?><!DOCTYPE gpx [<!ENTITY x SYSTEM "file:///data/data/com.anpfuel.app.debug">]><gpx><wpt lat="1" lon="1"><name>&x;</name></wpt></gpx>"""
        compose.runOnIdle { vm.importFile(xxe.toByteArray()) }
        compose.waitUntil(5_000) { compose.onAllNodesWithTag("saved-message").fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.map25_msg_file_refused)).assertIsDisplayed()
        assertTrue(runBlocking { store.places(guest) }.isEmpty())
        // A valid file imports, a second identical import is recognised.
        compose.runOnIdle { vm.importFile(gpx("Casa", "Trabalho")) }
        compose.waitUntil(5_000) { runBlocking { store.places(guest) }.size == 2 }
        compose.onNodeWithText("Casa").assertIsDisplayed()
        compose.runOnIdle { vm.importFile(gpx("Casa", "Trabalho")) }
        compose.waitUntil(5_000) { compose.onAllNodesWithTag("saved-message").fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.map25_msg_import_already)).assertIsDisplayed()
        assertEquals(2, runBlocking { store.places(guest) }.size)
        // Delete through the UI, then a different file naming the deleted id asks for confirmation.
        val casa = runBlocking { store.places(guest) }.first { it.name == "Casa" }
        compose.onNodeWithTag("saved-delete-place-${casa.id}").performScrollTo().performClick()
        compose.waitUntil(5_000) { runBlocking { store.places(guest) }.size == 1 }
        compose.runOnIdle { vm.importFile(gpx("Casa", "Trabalho", "Extra")) }
        compose.waitUntil(5_000) { compose.onAllNodesWithTag("saved-reimport-confirm").fetchSemanticsNodes().isNotEmpty() }
        assertEquals("deleted record must not return without confirmation", 1, runBlocking { store.places(guest) }.size)
        compose.onNodeWithTag("saved-reimport-confirm").performClick()
        compose.waitUntil(5_000) { runBlocking { store.places(guest) }.size == 3 }
    }

    @Test fun accountSwitchShowsOnlyThatOwnersItemsAndExportsOnlyTheirs() {
        runBlocking {
            store.savePlace(guest, SavedPlace("00000000-0000-4000-8000-000000000001", SavedPlaceKind.PLACE, "Do visitante", GeoPoint(1, 1), null, SavedSourceKind.MANUAL, null, null, 0))
            store.savePlace(SavedOwner("acct-a", origin), SavedPlace("00000000-0000-4000-8000-000000000002", SavedPlaceKind.PLACE, "Da conta A", GeoPoint(2, 2), null, SavedSourceKind.MANUAL, null, null, 0))
        }
        compose.setContent { SavedScreen(onBack = {}, viewModel = vm) }
        compose.onNodeWithText("Do visitante").assertIsDisplayed()
        assertEquals(0, compose.onAllNodesWithTag("saved-place-00000000-0000-4000-8000-000000000002").fetchSemanticsNodes().size)
        signedIn = "acct-a"
        compose.runOnIdle { vm.refresh() }
        compose.waitUntil(5_000) { compose.onAllNodesWithTag("saved-place-00000000-0000-4000-8000-000000000002").fetchSemanticsNodes().isNotEmpty() }
        assertEquals(0, compose.onAllNodesWithTag("saved-place-00000000-0000-4000-8000-000000000001").fetchSemanticsNodes().size)
        compose.runOnIdle { vm.prepareExport(ExchangeFormat.GPX) }
        compose.waitUntil(5_000) { vm.ui.value.export != null }
        val exported = vm.ui.value.export!!.bytes.decodeToString()
        assertTrue(exported.contains("Da conta A"))
        assertFalse(exported.contains("Do visitante"))
    }
}
