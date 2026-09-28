package com.anpfuel.application.port

import kotlinx.coroutines.flow.Flow

/**
 * Observes device network connectivity for offline UI banners (BR-004).
 */
interface NetworkConnectivityGateway {

    fun observeIsConnected(): Flow<Boolean>
}
