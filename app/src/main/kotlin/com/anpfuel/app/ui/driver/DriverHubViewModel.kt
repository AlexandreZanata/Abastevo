package com.anpfuel.app.ui.driver

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.premium.GetPremiumStatusUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.PremiumStatusException
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

enum class DriverHubAccess { VERIFIED, OFFLINE, LAPSED, UNAVAILABLE, SIGNED_OUT }

data class DriverHubUiState(
    val loading: Boolean = true,
    val access: DriverHubAccess = DriverHubAccess.UNAVAILABLE,
    val vehicles: List<Vehicle> = emptyList(),
    val vehicleLoadFailed: Boolean = false,
    val paymentsConfigured: Boolean = false,
)

/** DRV-16 owner-scoped hub state; never infers entitlement locally. */
@HiltViewModel
class DriverHubViewModel @Inject constructor(
    private val listVehicles: ListVehiclesUseCase,
    private val premiumStatus: GetPremiumStatusUseCase,
    private val sessions: AuthSessionStore,
) : ViewModel() {
    private val state = MutableStateFlow(DriverHubUiState())
    val uiState = state.asStateFlow()

    fun refresh() {
        viewModelScope.launch {
            state.value = state.value.copy(loading = true, vehicleLoadFailed = false)
            val owner = runCatching { sessions.load()?.accountId }.getOrNull()
            val vehicles = runCatching { listVehicles() }
            val vehicleList = vehicles.getOrElse {
                if (it is CancellationException) throw it
                emptyList()
            }
            if (owner.isNullOrBlank()) {
                state.value = DriverHubUiState(
                    loading = false, access = DriverHubAccess.SIGNED_OUT,
                    vehicles = vehicleList, vehicleLoadFailed = vehicles.isFailure,
                )
                return@launch
            }
            try {
                val snapshot = premiumStatus()
                val access = when {
                    snapshot.accountId != owner -> DriverHubAccess.SIGNED_OUT
                    snapshot.status.canUsePaidCapabilities -> DriverHubAccess.VERIFIED
                    else -> DriverHubAccess.LAPSED
                }
                state.value = DriverHubUiState(
                    loading = false, access = access, vehicles = vehicleList,
                    vehicleLoadFailed = vehicles.isFailure, paymentsConfigured = snapshot.status.paymentsConfigured,
                )
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (error: Exception) {
                val access = when (error) {
                    is PremiumStatusException.SignedOut -> DriverHubAccess.SIGNED_OUT
                    is PremiumStatusException.Denied -> DriverHubAccess.LAPSED
                    is PremiumStatusException.Unavailable, is IOException -> DriverHubAccess.OFFLINE
                    else -> DriverHubAccess.UNAVAILABLE
                }
                state.value = DriverHubUiState(
                    loading = false, access = access, vehicles = vehicleList,
                    vehicleLoadFailed = vehicles.isFailure,
                )
            }
        }
    }
}
