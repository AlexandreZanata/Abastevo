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
)

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
) : ViewModel() {

    constructor(
        store: MapPackageStore,
        repository: MapPackageRepository? = null,
    ) : this(store, repository, null, null)

    private val _ui = MutableStateFlow(MapScreenUi())
    val ui: StateFlow<MapScreenUi> = _ui.asStateFlow()
    private var fuelUserOverridden = false
    private var periodUserOverridden = false

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
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                _ui.update { it.copy(loading = false, error = "availability-failed") }
            }
        }
    }
}
