package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.repository.LedgerConsentGateway
import com.anpfuel.domain.repository.LedgerConsentStatus
import com.anpfuel.domain.repository.LedgerSyncDenied
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/**
 * The server consent mask gates every ledger upload (403 without it), so an
 * opt-in counts only once the server confirms the requested datasets, and a
 * revocation only once they are gone from the mask.
 */
class SetLedgerConsentUseCaseTest {
    private val gateway = mockk<LedgerConsentGateway>()
    private val sessions = mockk<AuthSessionStore>()
    private val session = PortableAuth.Session("fam-1", "acct-1", "at", "rt", Long.MAX_VALUE, Long.MAX_VALUE)
    private val useCase = SetLedgerConsentUseCase(gateway, sessions)

    @BeforeEach fun setUp() {
        every { sessions.load() } returns session
    }

    @Test fun `opt-in sends each requested dataset once in canonical order for the live session`() = runTest {
        val requested = listOf(LedgerDataset.REFUEL, LedgerDataset.EXPENSE)
        coEvery { gateway.setConsent("acct-1", "fam-1", "at", requested, true) } returns
            LedgerConsentStatus(3, listOf(LedgerDataset.REFUEL, LedgerDataset.EXPENSE, LedgerDataset.CHARGE))

        val status = useCase(listOf(LedgerDataset.EXPENSE, LedgerDataset.REFUEL, LedgerDataset.EXPENSE), enabled = true)

        assertEquals(3L, status.revision)
        coVerify(exactly = 1) { gateway.setConsent("acct-1", "fam-1", "at", requested, true) }
    }

    @Test fun `opt-in is refused when the server mask lacks a requested dataset`() = runTest {
        coEvery { gateway.setConsent(any(), any(), any(), any(), true) } returns
            LedgerConsentStatus(3, listOf(LedgerDataset.REFUEL))
        assertThrows(LedgerSyncDenied::class.java) {
            runBlocking { useCase(listOf(LedgerDataset.REFUEL, LedgerDataset.CHARGE), enabled = true) }
        }
    }

    @Test fun `revocation keeps other datasets but must clear the requested ones`() = runTest {
        coEvery { gateway.setConsent(any(), any(), any(), listOf(LedgerDataset.CHARGE), false) } returns
            LedgerConsentStatus(4, listOf(LedgerDataset.REFUEL))
        assertEquals(4L, useCase(listOf(LedgerDataset.CHARGE), enabled = false).revision)

        coEvery { gateway.setConsent(any(), any(), any(), listOf(LedgerDataset.REFUEL), false) } returns
            LedgerConsentStatus(5, listOf(LedgerDataset.REFUEL))
        assertThrows(LedgerSyncDenied::class.java) {
            runBlocking { useCase(listOf(LedgerDataset.REFUEL), enabled = false) }
        }
    }

    @Test fun `no session or no dataset never reaches the server`() = runTest {
        assertThrows(IllegalArgumentException::class.java) { runBlocking { useCase(emptyList(), enabled = true) } }
        every { sessions.load() } returns null
        assertThrows(LedgerSyncDenied::class.java) { runBlocking { useCase(listOf(LedgerDataset.REFUEL), enabled = true) } }
        coVerify(exactly = 0) { gateway.setConsent(any(), any(), any(), any(), any()) }
    }
}
