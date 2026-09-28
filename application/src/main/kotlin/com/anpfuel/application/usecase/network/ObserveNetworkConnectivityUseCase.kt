package com.anpfuel.application.usecase.network

import com.anpfuel.application.port.NetworkConnectivityGateway
import kotlinx.coroutines.flow.Flow

class ObserveNetworkConnectivityUseCase(
    private val networkConnectivityGateway: NetworkConnectivityGateway,
) {

    operator fun invoke(): Flow<Boolean> =
        networkConnectivityGateway.observeIsConnected()
}
