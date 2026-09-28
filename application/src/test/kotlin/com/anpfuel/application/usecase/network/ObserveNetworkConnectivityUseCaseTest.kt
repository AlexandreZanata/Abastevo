package com.anpfuel.application.usecase.network

import com.anpfuel.application.port.NetworkConnectivityGateway
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class ObserveNetworkConnectivityUseCaseTest {

    private val networkConnectivityGateway = mockk<NetworkConnectivityGateway>()
    private lateinit var useCase: ObserveNetworkConnectivityUseCase

    @BeforeEach
    fun setUp() {
        useCase = ObserveNetworkConnectivityUseCase(networkConnectivityGateway)
    }

    @Test
    fun emitsGatewayConnectivityUpdates() = runTest {
        every { networkConnectivityGateway.observeIsConnected() } returns flowOf(true, false)

        val emissions = mutableListOf<Boolean>()
        useCase().collect { emissions.add(it) }

        assertTrue(emissions[0])
        assertFalse(emissions[1])
        verify(exactly = 1) { networkConnectivityGateway.observeIsConnected() }
    }
}
