package com.anpfuel.app.ui.stationprofile

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.profile.*
import com.anpfuel.domain.portable.PortableAuth
import io.mockk.*
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class StationClaimViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val actions = mockk<StationProfileActions>()
    private val sessions = mockk<AuthSessionStore>()
    private val session = PortableAuth.Session("f", "a", "t", "r", 9999999999, 9999999999)
    private val claim = OwnedProfileClaim("claim", "station", "manager", setOf("profile.edit"), "draft", "decl", "private", 9999999999)
    @BeforeEach fun before() { Dispatchers.setMain(dispatcher); every { sessions.load() } returns session }
    @AfterEach fun after() { Dispatchers.resetMain() }
    private fun vm() = StationClaimViewModel(SavedStateHandle(mapOf("stationId" to "station")), actions, sessions)

    @Test fun `offline create retries same key and success requires actual server response`() = runTest(dispatcher) {
        val keys = mutableListOf<String>()
        coEvery { actions.open("station", "manager", any(), capture(keys)) } throws IOException()
        val vm = vm()
        vm.open(); advanceUntilIdle()
        assertNull(vm.state.value.selected)
        assertEquals(ClaimNotice.Unavailable, vm.state.value.notice)
        coEvery { actions.open("station", "manager", any(), capture(keys)) } returns claim
        vm.open(); advanceUntilIdle()
        assertEquals(keys[0], keys[1])
        assertEquals(claim, vm.state.value.selected)
    }
    @Test fun `restart restores private status from server and logout removes all private state`() = runTest(dispatcher) {
        coEvery { actions.mine("station") } returns listOf(claim)
        val first = vm(); first.refresh(); advanceUntilIdle()
        assertEquals(1, first.state.value.claims.size)
        val restarted = vm()
        assertTrue(restarted.state.value.claims.isEmpty())
        restarted.refresh(); advanceUntilIdle()
        assertEquals(1, restarted.state.value.claims.size)
        every { sessions.load() } returns null
        coEvery { actions.mine("station") } throws ProfileSignInRequired()
        restarted.refresh(); advanceUntilIdle()
        assertTrue(restarted.state.value.claims.isEmpty())
        assertNull(restarted.state.value.selected)
        assertEquals(ClaimNotice.SignIn, restarted.state.value.notice)
    }
    @Test fun `changed role changes create key and refused owner actions clear stale claim`() = runTest(dispatcher) {
        val keys = mutableListOf<String>()
        coEvery { actions.open(any(), any(), any(), capture(keys)) } returns claim
        val vm = vm(); vm.open(); advanceUntilIdle()
        vm.role("administrator"); vm.open(); advanceUntilIdle()
        assertNotEquals(keys[0], keys[1])
        coEvery { actions.mine(any()) } throws com.anpfuel.data.remote.profile.ProfileHttpFailure(403)
        vm.refresh(); advanceUntilIdle()
        assertTrue(vm.state.value.claims.isEmpty())
        assertEquals(ClaimNotice.Refused, vm.state.value.notice)
    }
}
