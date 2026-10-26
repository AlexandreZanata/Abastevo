package com.anpfuel.application.driver

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.driver.*
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class DriverShiftUseCaseTest {
    private class Authority : DriverShiftAuthority {
        private val lock = Mutex()
        val rows = mutableMapOf<String, DriverShift>()
        var beforeTransform: () -> Unit = {}
        override suspend fun transact(owner: String, transform: (DriverShift?) -> DriverShift): DriverShift = lock.withLock {
            beforeTransform()
            transform(rows[owner]).also { rows[owner] = it }
        }
    }
    private val sessions = mockk<AuthSessionStore>()
    private val policy = DriverClockPolicy(60000, 1000)
    private fun session(owner: String) = PortableAuth.Session("family", owner, "synthetic", "synthetic", 0, 0)
    @Test fun `one local owner cannot concurrently open two vehicle or origin shifts`() = runTest {
        every { sessions.load() } returns session("a")
        val authority = Authority()
        val useCase = DriverShiftUseCase(authority, sessions, "origin")
        val results = (1..20).map { index -> async(Dispatchers.Default) {
            runCatching { useCase.invoke(DriverShift("shift-$index", "a", "origin", "vehicle-$index", "UTC"),
                DriverShiftCommand("start-$index", DriverShiftAction.START, 0, DriverClock("boot", 0, 100000)), policy) }
        } }.awaitAll()
        assertEquals(1, results.count { it.isSuccess })
        assertEquals(1, authority.rows.size)
        val foreignOrigin = DriverShiftUseCase(authority, sessions, "other-origin")
        assertTrue(runCatching { foreignOrigin.invoke(DriverShift("different", "a", "other-origin", "car", "UTC"),
            DriverShiftCommand("other", DriverShiftAction.START, 0, DriverClock("boot", 0, 100000)), policy) }.isFailure)
        every { sessions.load() } returns session("b")
        val b = useCase.invoke(DriverShift("own-b", "b", "origin", "car-b", "UTC"),
            DriverShiftCommand("start-b", DriverShiftAction.START, 0, DriverClock("boot", 0, 100000)), policy)
        assertEquals("b", b.owner)
        assertEquals(2, authority.rows.size)
    }
    @Test fun `owner changes while authority load suspends cannot mutate or expose old shift`() = runTest {
        every { sessions.load() } returns session("a")
        val authority = Authority()
        authority.beforeTransform = { every { sessions.load() } returns session("b") }
        val failed = runCatching { DriverShiftUseCase(authority, sessions, "origin").invoke(
            DriverShift("own-a", "a", "origin", "car", "UTC"), DriverShiftCommand("start", DriverShiftAction.START, 0, DriverClock("boot", 0, 100000)), policy) }
        assertTrue(failed.isFailure)
        assertTrue(authority.rows.isEmpty())
    }
    @Test fun `a foreign authority row fails closed even when its shift id matches`() = runTest {
        every { sessions.load() } returns session("a")
        val authority = Authority()
        authority.rows["a"] = DriverShift("same-id", "b", "origin", "foreign-car", "UTC")
        val original = authority.rows["a"]
        val outcome = runCatching { DriverShiftUseCase(authority, sessions, "origin").invoke(
            DriverShift("same-id", "a", "origin", "own-car", "UTC"),
            DriverShiftCommand("start", DriverShiftAction.START, 0, DriverClock("boot", 0, 100000)), policy) }
        assertTrue(outcome.isFailure)
        assertEquals(original, authority.rows["a"])
    }

}
