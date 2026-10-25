package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.maps.engine.MapOverlayMarker
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
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
    val stale: Boolean = false,
)

data class SheetPriceRequest(
    val stationUuid: String,
    val source: MapPriceSource,
    val marker: MapOverlayMarker,
    val fuelProduct: FuelProduct? = null,
    val surveyPeriod: String? = null,
)

data class MapSheetUi(
    val marker: MapOverlayMarker? = null,
    val source: MapPriceSource = MapPriceSource.ANP,
    val product: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    val surveyPeriod: String? = null,
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
 * Rapid source/station/period switches and clear() invalidate generation and
 * cancel in-flight jobs before any branch or debounce. No silent fallback,
 * no float money, no N+1 (single lookup per selection).
 */
@HiltViewModel
class MapSheetViewModel @Inject constructor(
    private val prices: SheetPriceSource,
) : ViewModel() {
    private val _ui = MutableStateFlow(MapSheetUi())
    val ui: StateFlow<MapSheetUi> = _ui.asStateFlow()
    private var job: Job? = null
    private var generation = 0L

    fun select(
        marker: MapOverlayMarker,
        source: MapPriceSource,
        product: FuelProduct? = null,
        surveyPeriod: String? = null,
    ) {
        // B-BR-MAP-SHEET-RACE-01: invalidate generation and cancel in-flight
        // lookup immediately before any early-return branch or debounce.
        val gen = ++generation
        job?.cancel()
        job = null
        val resolvedProduct = product ?: _ui.value.product
        val resolvedPeriod = surveyPeriod ?: _ui.value.surveyPeriod
        // Charging in ANP mode never invents a tariff.
        if (source == MapPriceSource.ANP && !marker.fuel && marker.charging) {
            _ui.update {
                it.copy(
                    marker = marker,
                    source = source,
                    product = resolvedProduct,
                    surveyPeriod = resolvedPeriod,
                    price = null,
                    priceAbsent = true,
                    absentReason = "anp-no-tariff",
                    loading = false,
                    generation = gen,
                )
            }
            return
        }
        _ui.update {
            it.copy(
                marker = marker,
                source = source,
                product = resolvedProduct,
                surveyPeriod = resolvedPeriod,
                loading = true,
                price = null,
                priceAbsent = false,
                absentReason = null,
                generation = gen,
            )
        }
        job = viewModelScope.launch {
            delay(100)
            if (gen != generation) return@launch
            try {
                val result = prices.lookup(
                    SheetPriceRequest(
                        stationUuid = marker.uuid,
                        source = source,
                        marker = marker,
                        fuelProduct = product,
                        surveyPeriod = surveyPeriod,
                    ),
                )
                if (gen != generation) return@launch
                if (result == null) {
                    _ui.update { it.copy(loading = false, priceAbsent = true, absentReason = "no-price") }
                } else {
                    _ui.update { it.copy(loading = false, price = result, priceAbsent = false, absentReason = null) }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                if (gen != generation) return@launch
                _ui.update { it.copy(loading = false, priceAbsent = true, absentReason = "load-failed") }
            }
        }
    }

    fun clear() {
        val gen = ++generation
        job?.cancel()
        job = null
        _ui.update { MapSheetUi(generation = gen) }
    }
}

/** Single lookup per selection; null means explicit absence (never fallback). */
fun interface SheetPriceSource {
    suspend fun lookup(stationUuid: String, source: MapPriceSource, marker: MapOverlayMarker): SheetPrice?

    suspend fun lookup(request: SheetPriceRequest): SheetPrice? =
        lookup(request.stationUuid, request.source, request.marker)
}
