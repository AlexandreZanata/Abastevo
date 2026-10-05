package com.anpfuel.application.usecase.profile

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.*
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class StationProfileActionsTest {
    private val session = PortableAuth.Session("family", "account", "access", "refresh", 2000, 5000)
    private val store = mockk<AuthSessionStore>()
    private val gateway = mockk<ProfileClaimGateway>()
    private val actions = StationProfileActions(gateway, store) { 1000 }
    private val claim = OwnedProfileClaim("claim", "station", "manager", setOf("profile.edit"), "draft", "decl", "exact", 1800)

    @Test fun `guest expired session never calls claim gateway`() = runTest {
        every { store.load() } returns null
        assertThrows(ProfileSignInRequired::class.java) { kotlinx.coroutines.runBlocking { actions.open("station", "manager", setOf("profile.edit"), "key") } }
        every { store.load() } returns session.copy(accessExpiresAt = 1000)
        assertThrows(ProfileSignInRequired::class.java) { kotlinx.coroutines.runBlocking { actions.mine("station") } }
        coVerify(exactly = 0) { gateway.open(any(), any(), any(), any(), any()) }
        coVerify(exactly = 0) { gateway.mine(any()) }
    }
    @Test fun `reissued expired and terminal declarations cannot send original signed bytes`() = runTest {
        every { store.load() } returns session
        val bytes = "%PDF-original".toByteArray()
        for (live in listOf(claim.copy(declarationId = "new"), claim.copy(expiresAt = 1000), claim.copy(state = "approved"))) {
            coEvery { gateway.status(session, "claim") } returns live
            assertThrows(ProfileInputInvalid::class.java) { kotlinx.coroutines.runBlocking { actions.submit(claim, bytes) } }
        }
        coVerify(exactly = 0) { gateway.submit(any(), any(), any()) }
    }
    @Test fun `server acknowledgment receives original bytes and stable create key`() = runTest {
        every { store.load() } returns session
        coEvery { gateway.status(session, "claim") } returns claim
        coEvery { gateway.submit(session, claim, any()) } just Runs
        val bytes = "%PDF-original".toByteArray()
        actions.submit(claim, bytes)
        coVerify { gateway.submit(session, claim, match { it.contentEquals(bytes) }) }
        coEvery { gateway.open(session, "station", "manager", setOf("profile.edit"), "same") } returns claim
        repeat(2) { assertEquals(claim, actions.open("station", "manager", setOf("profile.edit"), "same")) }
        coVerify(exactly = 2) { gateway.open(session, "station", "manager", setOf("profile.edit"), "same") }
    }
}
