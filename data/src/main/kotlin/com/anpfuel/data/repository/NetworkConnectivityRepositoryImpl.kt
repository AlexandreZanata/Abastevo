package com.anpfuel.data.repository

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import com.anpfuel.application.port.NetworkConnectivityGateway
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map

@Singleton
class NetworkConnectivityRepositoryImpl @Inject constructor(
    @ApplicationContext context: Context,
) : NetworkConnectivityGateway {

    private val connectivityManager =
        context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager

    override fun observeIsConnected(): Flow<Boolean> = observeDefaultNetwork()
        .map { it?.hasValidatedInternet() == true }
        .distinctUntilChanged()

    fun observeUsesMobileData(): Flow<Boolean> = observeDefaultNetwork()
        .map {
            it != null && it.hasValidatedInternet() &&
                it.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) &&
                !it.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)
        }
        .distinctUntilChanged()

    private fun observeDefaultNetwork(): Flow<NetworkCapabilities?> = callbackFlow {
        val callback = object : ConnectivityManager.NetworkCallback() {
            private var defaultNetwork: Network? = null

            override fun onAvailable(network: Network) {
                defaultNetwork = network
            }

            override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) {
                // Use the delivered snapshot; querying ConnectivityManager here races this update.
                if (network == defaultNetwork) trySend(capabilities)
            }

            override fun onLost(network: Network) {
                if (network == defaultNetwork) {
                    defaultNetwork = null
                    trySend(null)
                }
            }
        }

        connectivityManager.registerDefaultNetworkCallback(callback)
        trySend(connectivityManager.activeNetwork?.let(connectivityManager::getNetworkCapabilities))

        awaitClose {
            connectivityManager.unregisterNetworkCallback(callback)
        }
    }
}

private fun NetworkCapabilities.hasValidatedInternet(): Boolean =
    hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
        hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)
