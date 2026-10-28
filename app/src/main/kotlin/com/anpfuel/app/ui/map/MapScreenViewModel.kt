package com.anpfuel.app.ui.map

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.FuelProduct
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

enum class MapPriceSource { ANP, COMMUNITY }

data class MapScreenUi(
    val source: MapPriceSource = MapPriceSource.ANP,
    val selectedFuelProduct: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    val selectedSurveyPeriod: String? = null,
    val loading: Boolean = false,
    val error: String? = null,
    val noPackage: Boolean = true,
    /** Region of the city the user opened; null outside Brazil or unknown. */
    val region: MapRegionUi? = null,
    /** "Baixar mapa de <região> (N MB)" offer; null when not applicable. */
    val regionSuggestion: MapRegionSuggestion? = null,
)

/** [bytes] is null until the signed region manifest was fetched (Wi-Fi or on tap). */
data class MapRegionSuggestion(val region: MapRegionUi, val bytes: Long?)

/**
 * MAP-17 shell state plus MAP-15 availability: the banner shows only when no
 * verified generation is active/published. Source and fuel product switching
 * stay explicit and reuse existing user/vehicle/week preferences.
 */
@HiltViewModel
class MapScreenViewModel @Inject constructor(
    private val store: MapPackageStore,
    private val repository: MapPackageRepository?,
    private val userPreferences: UserPreferencesRepository?,
    private val vehicles: VehicleRepository?,
    private val regions: MapRegionServices?,
) : ViewModel() {

    constructor(
        store: MapPackageStore,
        repository: MapPackageRepository? = null,
        regions: MapRegionServices? = null,
    ) : this(store, repository, null, null, regions)

    private val _ui = MutableStateFlow(MapScreenUi())
    val ui: StateFlow<MapScreenUi> = _ui.asStateFlow()
    private var fuelUserOverridden = false
    private var periodUserOverridden = false
    private var cityPoint: Pair<Double, Double>? = null
    private var regionJob: Job? = null

    init {
        refreshAvailability()
    }

    fun selectSource(source: MapPriceSource) {
        _ui.update { it.copy(source = source, error = null) }
    }

    fun selectFuelProduct(product: FuelProduct) {
        fuelUserOverridden = true
        _ui.update { it.copy(selectedFuelProduct = product) }
    }

    fun selectSurveyPeriod(period: String?) {
        periodUserOverridden = true
        _ui.update { it.copy(selectedSurveyPeriod = period) }
    }

    fun retry() {
        _ui.update { it.copy(loading = true, error = null) }
        refreshAvailability()
    }

    fun refreshAvailability() {
        viewModelScope.launch {
            try {
                val active = store.activeGeneration()
                val hasVerifiedPackage = if (repository != null) {
                    val published = repository.publishedState()
                    active != null && published != null && published.complete && published.generation == active.generation
                } else {
                    active != null
                }
                val prefs = userPreferences?.getPreferences()
                val preferredFuel = if (!fuelUserOverridden && prefs != null) {
                    prefs.preferredFuelProduct
                        ?: runCatching { vehicles?.listAll()?.firstNotNullOfOrNull { it.fuelProduct } }
                            .onFailure { if (it is CancellationException) throw it }
                            .getOrNull()
                        ?: _ui.value.selectedFuelProduct
                } else {
                    _ui.value.selectedFuelProduct
                }
                val preferredPeriod = if (!periodUserOverridden && prefs != null) {
                    prefs.activeSurveyWeek?.let { "${it.startDate}..${it.endDate}" }
                } else {
                    _ui.value.selectedSurveyPeriod
                }
                _ui.update {
                    it.copy(
                        loading = false,
                        noPackage = !hasVerifiedPackage,
                        selectedFuelProduct = preferredFuel,
                        selectedSurveyPeriod = preferredPeriod,
                    )
                }
                // A finished download (or a switch) changes what to suggest.
                cityPoint?.let { (lat, lon) -> evaluateRegion(lat, lon) }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                _ui.update { it.copy(loading = false, error = "availability-failed") }
            }
        }
    }

    /**
     * The map screen reports the coordinate of the city the user opened
     * (see [com.anpfuel.domain.map.chooseCityRegionPoint]); the region of
     * that point becomes the download target, and a suggestion is offered
     * when that region has no active offline package and was not dismissed.
     */
    fun onCityPoint(latitude: Double?, longitude: Double?) {
        if (regions == null) return
        if (latitude == null || longitude == null || !latitude.isFinite() || !longitude.isFinite()) {
            cityPoint = null
            regionJob?.cancel()
            regions.select(null)
            _ui.update { it.copy(region = null, regionSuggestion = null) }
            return
        }
        if (cityPoint == latitude to longitude) return
        cityPoint = latitude to longitude
        evaluateRegion(latitude, longitude)
    }

    private fun evaluateRegion(latitude: Double, longitude: Double) {
        val services = regions ?: return
        regionJob?.cancel()
        regionJob = viewModelScope.launch {
            try {
                val region = services.regionFor(latitude, longitude)?.let(MapRegionUi::of)
                services.select(region?.slug)
                if (region == null) {
                    _ui.update { it.copy(region = null, regionSuggestion = null) }
                    return@launch
                }
                val offer = services.downloadsConfigured &&
                    !services.isDismissed(region.slug) &&
                    services.activeRegionSlug() != region.slug
                if (!offer) {
                    _ui.update { it.copy(region = region, regionSuggestion = null) }
                    return@launch
                }
                val known = _ui.value.regionSuggestion?.takeIf { it.region == region }?.bytes
                _ui.update { it.copy(region = region, regionSuggestion = MapRegionSuggestion(region, known)) }
                if (known == null && services.canFetchManifestAutomatically()) {
                    val bytes = try {
                        services.regionDownloadBytes(region.slug)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (_: Exception) {
                        null // Size stays unknown; the offer itself remains valid.
                    }
                    if (bytes != null) {
                        _ui.update { state ->
                            if (state.regionSuggestion?.region == region) {
                                state.copy(regionSuggestion = MapRegionSuggestion(region, bytes))
                            } else {
                                state
                            }
                        }
                    }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                _ui.update { it.copy(regionSuggestion = null) }
            }
        }
    }

    /** "Baixar": the downloads screen starts this region right away. */
    fun acceptRegionSuggestion(): Boolean {
        val suggestion = _ui.value.regionSuggestion ?: return false
        regions?.requestDownload(suggestion.region.slug) ?: return false
        return true
    }

    /** "Agora não": persisted per region. */
    fun dismissRegionSuggestion() {
        val suggestion = _ui.value.regionSuggestion ?: return
        regions?.dismiss(suggestion.region.slug)
        _ui.update { it.copy(regionSuggestion = null) }
    }
}
