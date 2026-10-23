package com.anpfuel.application.usecase.premium

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.PremiumState
import com.anpfuel.domain.premium.PremiumStatus
import com.anpfuel.domain.premium.PremiumStatusException
import com.anpfuel.domain.repository.PremiumStatusGateway
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class GetPremiumStatusUseCaseTest {
    private val gateway = mockk<PremiumStatusGateway>()
    private val sessions = mockk<AuthSessionStore>()
    private val session = PortableAuth.Session("fam", "acct", "token", "refresh", Long.MAX_VALUE, Long.MAX_VALUE)
    private val status = PremiumStatus(
        policyVersion = 1, state = PremiumState.ACTIVE, reason = "verified",
        capabilities = setOf("cloud.sync"), validUntil = null, retainedDataAccess = true,
        paymentsConfigured = false, managementUrl = PremiumStatus.MANAGEMENT_URL,
    )

    @Test fun `signed out refuses before the private wire call`() = runTest {
        every { sessions.load() } returns null
        assertThrows(PremiumStatusException.SignedOut::class.java) {
            kotlinx.coroutines.runBlocking { GetPremiumStatusUseCase(gateway, sessions).invoke() }
        }
        coVerify(exactly = 0) { gateway.status(any(), any()) }
    }

    @Test fun `returns owner-bound server status`() = runTest {
        every { sessions.load() } returns session
        coEvery { gateway.status("fam", "token") } returns status
        val snapshot = GetPremiumStatusUseCase(gateway, sessions).invoke()
        assertEquals("acct", snapshot.accountId)
        assertEquals(PremiumState.ACTIVE, snapshot.status.state)
    }
}
