package com.anpfuel.application

import app.cash.turbine.test
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class ExampleApplicationTest {

    @Test
    fun mockkAndTurbineAreWired() = runTest {
        val port = mockk<SamplePort>()
        every { port.execute() } returns "ok"

        assertEquals("ok", port.execute())
        verify { port.execute() }

        flowOf("event").test {
            assertEquals("event", awaitItem())
            awaitComplete()
        }
    }

    private interface SamplePort {
        fun execute(): String
    }
}
