package com.anpfuel.application.usecase.privacy

import com.anpfuel.application.port.NetworkConnectivityGateway
import com.anpfuel.application.port.TrainingConsentLocalStore
import com.anpfuel.domain.repository.TrainingConsentException
import com.anpfuel.domain.repository.TrainingConsentGateway
import com.anpfuel.domain.repository.TrainingConsentReceipt
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import com.anpfuel.domain.repository.TrainingConsentStatus
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.Test

/**
 * CS11A: training-consent use case (B-BR-CS06).
 *
 * Guest performs no IO and claims nothing; grants refuse offline
 * without queueing (fail-closed ON); revokes queue offline as a
 * pending revocation the UI surfaces until the next sync; revisions
 * increase per decision for server-side ordering; rejections carry
 * fixed kinds with sanitized messages.
 */
class TrainingConsentUseCaseTest {

    private class FakeLocal : TrainingConsentLocalStore {
        var toggle: Boolean = false
        var revision: Long = 0
        var pending: Boolean = false

        override fun isToggleOn(): Boolean = toggle

        override fun setToggleOn(enabled: Boolean) {
            toggle = enabled
        }

        override fun nextRevision(): Long {
            revision += 1
            return revision
        }

        override fun hasPendingRevocation(): Boolean = pending

        override fun setPendingRevocation(pending: Boolean) {
            this.pending = pending
        }
    }

    private class FakeGateway(
        var receipt: TrainingConsentReceipt = TrainingConsentReceipt("GRANTED", 1, "training-terms-v1", false),
        var status: TrainingConsentStatus = TrainingConsentStatus(false, null, null, null, 0, false),
        var failure: TrainingConsentException? = null,
    ) : TrainingConsentGateway {
        val grants = mutableListOf<Long>()
        val revokes = mutableListOf<Long>()
        var statusCalls = 0

        override suspend fun grantConsent(termsVersion: String, termsDigest: String, revision: Long): TrainingConsentReceipt {
            failure?.let { throw it }
            grants += revision
            return receipt.copy(revision = revision)
        }

        override suspend fun revokeConsent(revision: Long): TrainingConsentReceipt {
            failure?.let { throw it }
            revokes += revision
            return receipt.copy(state = "REVOKED", revision = revision)
        }

        override suspend fun consentStatus(): TrainingConsentStatus {
            failure?.let { throw it }
            statusCalls += 1
            return status
        }
    }

    private class FakeNetwork(connected: Boolean) : NetworkConnectivityGateway {
        private val flow = MutableStateFlow(connected)

        override fun observeIsConnected(): Flow<Boolean> = flow
    }

    private fun useCase(
        local: FakeLocal = FakeLocal(),
        gateway: FakeGateway = FakeGateway(),
        connected: Boolean = true,
    ): Triple<TrainingConsentUseCase, FakeLocal, FakeGateway> {
        val useCase = TrainingConsentUseCase(local, gateway, FakeNetwork(connected))
        return Triple(useCase, local, gateway)
    }

    private fun caller() = TrainingConsentCaller("account-1")

    @Test
    fun `guest performs no io`() = runTest {
        val (useCase, local, gateway) = useCase()
        assertEquals(TrainingConsentOutcome.Guest, useCase.status(null))
        assertEquals(TrainingConsentOutcome.Guest, useCase.grant(null, "training-terms-v1", "digest-v1"))
        assertEquals(TrainingConsentOutcome.Guest, useCase.revoke(null))
        assertEquals(0, gateway.statusCalls)
        assertTrue(gateway.grants.isEmpty())
        assertTrue(gateway.revokes.isEmpty())
        assertEquals(0, local.revision)
    }

    @Test
    fun `grant records revision and enables toggle`() = runTest {
        val (useCase, local, gateway) = useCase()
        val outcome = useCase.grant(caller(), "training-terms-v1", "digest-v1")
        val granted = outcome as TrainingConsentOutcome.Granted
        assertEquals(1, granted.receipt.revision)
        assertEquals(listOf(1L), gateway.grants)
        assertTrue(local.isToggleOn())
    }

    @Test
    fun `revisions increase per decision`() = runTest {
        val (useCase, _, gateway) = useCase()
        useCase.grant(caller(), "training-terms-v1", "digest-v1")
        useCase.revoke(caller())
        assertEquals(listOf(1L), gateway.grants)
        assertEquals(listOf(2L), gateway.revokes)
    }

    @Test
    fun `grant refuses offline without queueing`() = runTest {
        val (useCase, local, gateway) = useCase(connected = false)
        assertEquals(TrainingConsentOutcome.Offline, useCase.grant(caller(), "training-terms-v1", "digest-v1"))
        assertTrue(gateway.grants.isEmpty())
        assertEquals(0, local.revision)
        assertFalse(local.isToggleOn())
        assertFalse(local.hasPendingRevocation())
    }

    @Test
    fun `revoke queues offline as pending`() = runTest {
        val (useCase, local, gateway) = useCase(connected = false)
        local.setToggleOn(true)
        assertEquals(TrainingConsentOutcome.RevocationPending, useCase.revoke(caller()))
        assertTrue(gateway.revokes.isEmpty())
        assertFalse(local.isToggleOn())
        assertTrue(local.hasPendingRevocation())
    }

    @Test
    fun `sync uploads pending and clears`() = runTest {
        val (useCase, local, gateway) = useCase(connected = false)
        useCase.revoke(caller())
        val online = TrainingConsentUseCase(local, gateway, FakeNetwork(true))
        val outcome = online.syncPending(caller())
        val revoked = outcome as TrainingConsentOutcome.Revoked
        assertEquals(1, revoked.receipt.revision)
        assertFalse(local.hasPendingRevocation())
    }

    @Test
    fun `rejections carry fixed kinds`() = runTest {
        val gateway = FakeGateway(failure = TrainingConsentException(TrainingConsentRejectKind.STALE_REVISION, "stale"))
        val (useCase, _, _) = useCase(gateway = gateway)
        val outcome = useCase.grant(caller(), "training-terms-v1", "digest-v1")
        val rejected = outcome as TrainingConsentOutcome.Rejected
        assertEquals(TrainingConsentRejectKind.STALE_REVISION, rejected.kind)
    }

    @Test
    fun `blank terms refuse before io`() = runTest {
        val (useCase, _, gateway) = useCase()
        try {
            useCase.grant(caller(), "", "digest-v1")
            fail("blank terms accepted")
        } catch (_: IllegalArgumentException) {
        }
        assertTrue(gateway.grants.isEmpty())
    }
}
