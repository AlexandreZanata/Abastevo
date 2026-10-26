package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.mapsaved.*
import com.anpfuel.domain.map.saved.*
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Exactly one message code is shown; the UI maps codes to localized text (never raw exception text). */
data class SavedUiState(
    val loading: Boolean = true,
    val places: List<SavedPlace> = emptyList(),
    val trips: List<TripSummary> = emptyList(),
    val recording: RecordingState? = null,
    val favoriteStationIds: Set<String> = emptySet(),
    val message: String? = null,
    val pendingDeletedImport: ByteArray? = null,
    val export: PendingExport? = null,
)

class PendingExport(val fileName: String, val mime: String, val bytes: ByteArray)

@HiltViewModel
class SavedViewModel @Inject constructor(
    private val list: ListSavedUseCase,
    private val toggle: ToggleStationFavoriteUseCase,
    private val savePlan: SaveRoutePlanUseCase,
    private val delete: DeleteSavedUseCase,
    private val import: ImportExchangeUseCase,
    private val export: ExportExchangeUseCase,
    private val recorder: TrackRecordingUseCase,
) : ViewModel() {
    private val _ui = MutableStateFlow(SavedUiState())
    val ui: StateFlow<SavedUiState> = _ui.asStateFlow()

    private fun guarded(code: String, work: suspend () -> Unit) {
        viewModelScope.launch {
            try {
                work()
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: SavedValidationException) {
                _ui.update { it.copy(message = error.code) }
            } catch (_: Exception) {
                _ui.update { it.copy(message = code) }
            }
        }
    }

    fun refresh() = guarded("load-failed") {
        val overview = list()
        _ui.update {
            it.copy(
                loading = false, places = overview.places, trips = overview.trips, recording = overview.recording,
                favoriteStationIds = overview.places.mapNotNull { p -> p.stationId }.toSet(),
            )
        }
    }

    fun toggleFavorite(marker: com.anpfuel.maps.engine.MapOverlayMarker) = guarded("favorite-failed") {
        val uuid = marker.uuid
        val source = if (marker.charging && !marker.fuel) SavedSourceKind.COMMUNITY else SavedSourceKind.ANP
        val nowFavorite = toggle(uuid, marker.title.take(SavedBounds.MAX_NAME), marker.latitude, marker.longitude, source, uuid)
        refresh()
        _ui.update { it.copy(message = if (nowFavorite) "favorite-saved" else "favorite-removed") }
    }

    fun saveRoutePlan(name: String, points: List<Pair<Double, Double>>, avoid: Set<String>) = guarded("trip-save-failed") {
        savePlan(name, points, avoid)
        refresh()
        _ui.update { it.copy(message = "trip-saved") }
    }

    fun deletePlace(id: String) = guarded("delete-failed") { delete.place(id); refresh() }
    fun deleteTrip(id: String) = guarded("delete-failed") { delete.trip(id); refresh() }

    fun importFile(bytes: ByteArray, confirmDeleted: Boolean = false) = guarded("import-failed") {
        try {
            val report = import(bytes, confirmDeleted)
            refresh()
            _ui.update {
                it.copy(
                    pendingDeletedImport = null,
                    message = when {
                        report.alreadyImported -> "import-already"
                        report.importedPlaces + report.importedTrips == 0 -> "import-nothing-new"
                        else -> "import-done"
                    },
                )
            }
        } catch (error: SavedValidationException) {
            if (error.code == "deleted-records-present") {
                _ui.update { it.copy(pendingDeletedImport = bytes, message = error.code) }
            } else {
                throw error
            }
        }
    }

    fun dismissDeletedImport() = _ui.update { it.copy(pendingDeletedImport = null, message = null) }

    fun prepareExport(format: ExchangeFormat, tripId: String? = null, placeId: String? = null) = guarded("export-failed") {
        val bytes = export(format, tripId, placeId)
        val ext = if (format == ExchangeFormat.GPX) "gpx" else "kml"
        val mime = if (format == ExchangeFormat.GPX) "application/gpx+xml" else "application/vnd.google-earth.kml+xml"
        _ui.update { it.copy(export = PendingExport("abastevo-saved.$ext", mime, bytes)) }
    }

    fun exportConsumed(message: String?) = _ui.update { it.copy(export = null, message = message) }

    /** Discards a recording left unfinished by an interrupted process or a stopped service. */
    fun finishRecording(save: Boolean) = guarded("recording-failed") {
        val saved = recorder.stop(save)
        refresh()
        _ui.update { it.copy(message = if (save && saved != null) "recording-saved" else "recording-discarded") }
    }

    fun clearMessage() = _ui.update { it.copy(message = null) }
}
