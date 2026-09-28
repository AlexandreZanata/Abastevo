package com.anpfuel.app.ui.vehicle

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.app.notification.NotificationPermissionHandler
import com.anpfuel.application.error.AppError
import com.anpfuel.application.error.AppErrorResolver
import com.anpfuel.application.usecase.alert.ConfigurePriceDropAlertUseCase
import com.anpfuel.application.usecase.price.GetStationPricesUseCase
import com.anpfuel.application.usecase.price.StationPricesOutcome
import com.anpfuel.application.usecase.vehicle.DeleteVehicleUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.application.usecase.vehicle.SaveVehicleUseCase
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.rule.MaxRegisteredVehiclesRule
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import com.anpfuel.domain.valueobject.VehiclePriceSourceMode
import com.anpfuel.app.mapper.StationPriceUiMapper
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class VehicleStationOptionUiModel(
    val cnpj: Cnpj,
    val displayName: String,
    val priceFormatted: String,
)

data class VehicleFormState(
    val displayName: String = "",
    val tankCapacityInput: String = "",
    val fuelProduct: FuelProduct = FuelProduct.GASOLINE_REGULAR,
    val priceSourceMode: VehiclePriceSourceMode = VehiclePriceSourceMode.CHEAPEST_STATION,
    val selectedStationCnpj: Cnpj? = null,
    val priceDropAlertEnabled: Boolean = false,
    val validationErrorKey: VehicleFormValidationError? = null,
)

enum class VehicleFormValidationError {
    NAME_REQUIRED,
    TANK_CAPACITY_INVALID,
    STATION_REQUIRED,
}

data class VehicleUiState(
    val isLoading: Boolean = true,
    val vehicles: List<Vehicle> = emptyList(),
    val showForm: Boolean = false,
    val editingVehicleId: DomainId? = null,
    val form: VehicleFormState = VehicleFormState(),
    val stationOptions: List<VehicleStationOptionUiModel> = emptyList(),
    val isLoadingStations: Boolean = false,
    val showDeleteConfirm: Boolean = false,
    val vehicleToDelete: DomainId? = null,
    val canAddVehicle: Boolean = true,
    val isSaving: Boolean = false,
    val showNotificationPermissionHint: Boolean = false,
    val error: AppError? = null,
    val errorMessage: String? = null,
)

@HiltViewModel
class VehicleViewModel @Inject constructor(
    private val listVehiclesUseCase: ListVehiclesUseCase,
    private val saveVehicleUseCase: SaveVehicleUseCase,
    private val deleteVehicleUseCase: DeleteVehicleUseCase,
    private val getStationPricesUseCase: GetStationPricesUseCase,
    private val configurePriceDropAlertUseCase: ConfigurePriceDropAlertUseCase,
    private val notificationPermissionHandler: NotificationPermissionHandler,
) : ViewModel() {

    private val _uiState = MutableStateFlow(VehicleUiState())
    val uiState: StateFlow<VehicleUiState> = _uiState.asStateFlow()

    private val _notificationPermissionRequest = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
    val notificationPermissionRequest: SharedFlow<Unit> = _notificationPermissionRequest.asSharedFlow()

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorMessage = null) }
            runCatching {
                val vehicles = listVehiclesUseCase()
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        vehicles = vehicles,
                        canAddVehicle = vehicles.size < MaxRegisteredVehiclesRule.MAX_VEHICLES,
                    )
                }
            }.onFailure { error ->
                val appError = AppErrorResolver.fromThrowable(error)
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        error = appError,
                        errorMessage = error.message,
                    )
                }
            }
        }
    }

    fun startAddVehicle() {
        if (!_uiState.value.canAddVehicle) {
            return
        }
        _uiState.update {
            it.copy(
                showForm = true,
                editingVehicleId = null,
                form = VehicleFormState(),
                stationOptions = emptyList(),
                showNotificationPermissionHint = false,
                error = null,
                errorMessage = null,
            )
        }
    }

    fun startEditVehicle(vehicleId: DomainId, locale: Locale) {
        val vehicle = _uiState.value.vehicles.firstOrNull { it.id == vehicleId } ?: return
        _uiState.update {
            it.copy(
                showForm = true,
                editingVehicleId = vehicle.id,
                form = VehicleFormState(
                    displayName = vehicle.displayName,
                    tankCapacityInput = vehicle.tankCapacity.liters.stripTrailingZeros().toPlainString(),
                    fuelProduct = vehicle.fuelProduct,
                    priceSourceMode = vehicle.priceSource.mode,
                    selectedStationCnpj = vehicle.priceSource.specificStationCnpj,
                    priceDropAlertEnabled = vehicle.priceDropAlertEnabled,
                ),
                showNotificationPermissionHint = vehicle.priceDropAlertEnabled &&
                    !notificationPermissionHandler.hasPostNotificationsPermission(),
                error = null,
                errorMessage = null,
            )
        }
        if (vehicle.priceSource.mode == VehiclePriceSourceMode.SPECIFIC_STATION) {
            loadStationOptions(locale)
        }
    }

    fun dismissForm() {
        _uiState.update {
            it.copy(
                showForm = false,
                editingVehicleId = null,
                form = VehicleFormState(),
                stationOptions = emptyList(),
                showNotificationPermissionHint = false,
            )
        }
    }

    fun onDisplayNameChanged(value: String) {
        _uiState.update {
            it.copy(form = it.form.copy(displayName = value, validationErrorKey = null))
        }
    }

    fun onTankCapacityChanged(value: String) {
        _uiState.update {
            it.copy(form = it.form.copy(tankCapacityInput = value, validationErrorKey = null))
        }
    }

    fun onFuelProductSelected(fuelProduct: FuelProduct, locale: Locale) {
        _uiState.update {
            it.copy(
                form = it.form.copy(
                    fuelProduct = fuelProduct,
                    selectedStationCnpj = null,
                    validationErrorKey = null,
                ),
            )
        }
        if (_uiState.value.form.priceSourceMode == VehiclePriceSourceMode.SPECIFIC_STATION) {
            loadStationOptions(locale)
        }
    }

    fun onPriceSourceModeSelected(mode: VehiclePriceSourceMode, locale: Locale) {
        _uiState.update {
            it.copy(
                form = it.form.copy(
                    priceSourceMode = mode,
                    selectedStationCnpj = if (mode == VehiclePriceSourceMode.CHEAPEST_STATION) {
                        null
                    } else {
                        it.form.selectedStationCnpj
                    },
                    validationErrorKey = null,
                ),
            )
        }
        if (mode == VehiclePriceSourceMode.SPECIFIC_STATION) {
            loadStationOptions(locale)
        } else {
            _uiState.update { state -> state.copy(stationOptions = emptyList()) }
        }
    }

    fun onStationSelected(cnpj: Cnpj) {
        _uiState.update {
            it.copy(
                form = it.form.copy(
                    selectedStationCnpj = cnpj,
                    validationErrorKey = null,
                ),
            )
        }
    }

    fun onPriceDropAlertChanged(enabled: Boolean) {
        if (!enabled) {
            _uiState.update {
                it.copy(
                    form = it.form.copy(priceDropAlertEnabled = false),
                    showNotificationPermissionHint = false,
                )
            }
            return
        }

        if (notificationPermissionHandler.hasPostNotificationsPermission()) {
            _uiState.update {
                it.copy(
                    form = it.form.copy(priceDropAlertEnabled = true),
                    showNotificationPermissionHint = false,
                )
            }
            return
        }

        _uiState.update {
            it.copy(
                form = it.form.copy(priceDropAlertEnabled = true),
                showNotificationPermissionHint = false,
            )
        }
        viewModelScope.launch {
            _notificationPermissionRequest.emit(Unit)
        }
    }

    fun onNotificationPermissionGranted() {
        _uiState.update { it.copy(showNotificationPermissionHint = false) }
    }

    fun onNotificationPermissionDenied() {
        _uiState.update { it.copy(showNotificationPermissionHint = true) }
    }

    fun refreshNotificationPermissionState() {
        val form = _uiState.value.form
        if (form.priceDropAlertEnabled) {
            _uiState.update {
                it.copy(
                    showNotificationPermissionHint =
                        !notificationPermissionHandler.hasPostNotificationsPermission(),
                )
            }
        }
    }

    fun requestDeleteVehicle(vehicleId: DomainId) {
        _uiState.update {
            it.copy(showDeleteConfirm = true, vehicleToDelete = vehicleId)
        }
    }

    fun dismissDeleteConfirm() {
        _uiState.update {
            it.copy(showDeleteConfirm = false, vehicleToDelete = null)
        }
    }

    fun confirmDeleteVehicle() {
        val vehicleId = _uiState.value.vehicleToDelete ?: return
        viewModelScope.launch {
            runCatching {
                deleteVehicleUseCase(vehicleId)
            }.onSuccess {
                dismissDeleteConfirm()
                load()
            }.onFailure { error ->
                val appError = AppErrorResolver.fromThrowable(error)
                _uiState.update {
                    it.copy(
                        showDeleteConfirm = false,
                        vehicleToDelete = null,
                        error = appError,
                        errorMessage = error.message,
                    )
                }
            }
        }
    }

    fun saveVehicle(locale: Locale) {
        val form = _uiState.value.form
        val validationError = validateForm(form)
        if (validationError != null) {
            _uiState.update { it.copy(form = it.form.copy(validationErrorKey = validationError)) }
            return
        }

        val tankCapacity = parseTankCapacity(form.tankCapacityInput)
            ?: run {
                _uiState.update {
                    it.copy(form = it.form.copy(validationErrorKey = VehicleFormValidationError.TANK_CAPACITY_INVALID))
                }
                return
            }

        val priceSource = when (form.priceSourceMode) {
            VehiclePriceSourceMode.CHEAPEST_STATION -> VehiclePriceSource.cheapest()
            VehiclePriceSourceMode.SPECIFIC_STATION -> VehiclePriceSource.specific(form.selectedStationCnpj!!)
        }

        val editingId = _uiState.value.editingVehicleId
        val previousVehicle = editingId?.let { id ->
            _uiState.value.vehicles.firstOrNull { it.id == id }
        }
        val vehicle = if (editingId == null) {
            Vehicle.create(
                displayName = form.displayName,
                tankCapacity = tankCapacity,
                fuelProduct = form.fuelProduct,
                priceSource = priceSource,
                priceDropAlertEnabled = form.priceDropAlertEnabled,
                sortOrder = _uiState.value.vehicles.size,
            )
        } else {
            val existing = _uiState.value.vehicles.first { it.id == editingId }
            existing
                .withDisplayName(form.displayName)
                .withTankCapacity(tankCapacity)
                .withFuelProduct(form.fuelProduct)
                .withPriceSource(priceSource)
                .withPriceDropAlertEnabled(form.priceDropAlertEnabled)
        }

        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null, errorMessage = null) }
            runCatching {
                saveVehicleUseCase(vehicle)
                configurePriceDropAlertUseCase(updated = vehicle, previous = previousVehicle)
            }.onSuccess {
                _uiState.update { it.copy(isSaving = false) }
                dismissForm()
                load()
            }.onFailure { error ->
                val appError = if (error is DomainException &&
                    error.message?.contains("Cannot register more than") == true
                ) {
                    null
                } else {
                    AppErrorResolver.fromThrowable(error)
                }
                _uiState.update {
                    it.copy(
                        isSaving = false,
                        error = appError,
                        errorMessage = error.message,
                        canAddVehicle = it.vehicles.size < MaxRegisteredVehiclesRule.MAX_VEHICLES,
                    )
                }
            }
        }
    }

    private fun loadStationOptions(locale: Locale) {
        val fuelProduct = _uiState.value.form.fuelProduct
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingStations = true) }
            runCatching {
                when (val outcome = getStationPricesUseCase(fuelProduct = fuelProduct)) {
                    is StationPricesOutcome.Success -> {
                        val options = outcome.stations.map { stationPrice ->
                            val uiModel = StationPriceUiMapper.toUiModel(
                                stationPrice = stationPrice,
                                locale = locale,
                                preferredState = outcome.state,
                                preferredMunicipality = outcome.municipality,
                            )
                            VehicleStationOptionUiModel(
                                cnpj = stationPrice.station.cnpj,
                                displayName = uiModel.displayName,
                                priceFormatted = uiModel.priceFormatted,
                            )
                        }
                        _uiState.update {
                            it.copy(
                                isLoadingStations = false,
                                stationOptions = options,
                            )
                        }
                    }
                    is StationPricesOutcome.StationDetailMissing -> {
                        _uiState.update {
                            it.copy(
                                isLoadingStations = false,
                                stationOptions = emptyList(),
                                error = outcome.error,
                            )
                        }
                    }
                }
            }.onFailure { error ->
                _uiState.update {
                    it.copy(
                        isLoadingStations = false,
                        error = AppErrorResolver.fromThrowable(error),
                        errorMessage = error.message,
                    )
                }
            }
        }
    }

    private fun validateForm(form: VehicleFormState): VehicleFormValidationError? {
        if (form.displayName.isBlank()) {
            return VehicleFormValidationError.NAME_REQUIRED
        }
        if (parseTankCapacity(form.tankCapacityInput) == null) {
            return VehicleFormValidationError.TANK_CAPACITY_INVALID
        }
        if (form.priceSourceMode == VehiclePriceSourceMode.SPECIFIC_STATION &&
            form.selectedStationCnpj == null
        ) {
            return VehicleFormValidationError.STATION_REQUIRED
        }
        return null
    }

    private fun parseTankCapacity(input: String): TankCapacity? =
        runCatching {
            TankCapacity.of(input.trim().replace(',', '.').toDouble())
        }.getOrNull()
}
