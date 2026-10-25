package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.maps.engine.MapOverlayMarker
import com.anpfuel.maps.engine.MapStationType
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class SheetPrice(
    val product: String,
    val unit: String,
    val amount: String,
    val source: MapPriceSource,
    val dateLabel: String,
)

data class MapSheetUi(
    val marker: MapOverlayMarker? = null,
    val source: MapPriceSource = MapPriceSource.ANP,
    val price: SheetPrice? = null,
    val priceAbsent: Boolean = false,
    val absentReason: String? = null,
    val loading: Boolean = false,
    val generation: Long = 0,
)

/**
 * MAP-19 bottom sheet: exact source/date/product/unit prices with explicit
 * absence. ANP shows station-specific observations; Community shows the
 * validated projection; charging in ANP explains the missing electric tariff.
 * Rapid source switches discard stale responses by generation. No silent
 * fallback, no float money, no N+1 (single lookup per selection).
 */
@HiltViewModel
class MapSheetViewModel @Inject constructor(
    private val prices: SheetPriceSource,
) : ViewModel() {
    private val _ui = MutableStateFlow(MapSheetUi())
    val ui: StateFlow<MapSheetUi> = _ui.asStateFlow()
    private var job: Job? = null
    private var generation = 0L

    fun select(marker: MapOverlayMarker, source: MapPriceSource) {
        // Charging in ANP mode never invents a tariff.
        if (source == MapPriceSource.ANP && !marker.fuel && marker.charging) {
            _ui.update {
                it.copy(
                    marker = marker,
                    source = source,
                    price = null,
                    priceAbsent = true,
                    absentReason = "anp-no-tariff",
                    loading = false,
                )
            }
            return
        }
        job?.cancel()
        job = viewModelScope.launch {
            delay(100)
            val gen = ++generation
            _ui.update { it.copy(marker = marker, source = source, loading = true, price = null, priceAbsent = false, absentReason = null, generation = gen) }
            try {
                val result = prices.lookup(marker.uuid, source, marker)
                if (gen != generation) return@launch
                if (result == null) {
                    _ui.update { it.copy(loading = false, priceAbsent = true, absentReason = "no-price") }
                } else {
                    _ui.update { it.copy(loading = false, price = result, priceAbsent = false, absentReason = null) }
                }
            } catch (e: kotlinx.coroutines.CancellationException) {
                throw e
            } catch (_: Exception) {
                if (gen != generation) return@launch
                _ui.update { it.copy(loading = false, priceAbsent = true, absentReason = "load-failed") }
            }
        }
    }

    fun clear() {
        job?.cancel()
        _ui.update { MapSheetUi() }
    }
}

/** Single lookup per selection; null means explicit absence (never fallback). */
fun interface SheetPriceSource {
    suspend fun lookup(stationUuid: String, source: MapPriceSource, marker: MapOverlayMarker): SheetPrice?
}
