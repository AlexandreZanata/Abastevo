package com.anpfuel.app.ui.stationprofile

import androidx.lifecycle.SavedStateHandle
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.profile.*
import com.anpfuel.data.remote.profile.ProfileHttpFailure
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.profile.StationProfile
import io.mockk.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class StationManagementViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val actions = mockk<StationProfileActions>()
    private val read = mockk<GetStationProfileUseCase>()
    private val sessions = mockk<AuthSessionStore>()
    private val approved = OwnedProfileClaim("c", "station", "manager", setOf("profile.edit", "reply.official"), "approved", "d", "private", 0)
    @BeforeEach fun before() {
        Dispatchers.setMain(dispatcher)
        every { sessions.load() } returns PortableAuth.Session("f", "a", "t", "r", 9999999999, 9999999999)
        coEvery { actions.mine("station") } returns listOf(approved)
        coEvery { read("station") } returns StationProfileOutcome.Found(StationProfile("station", "Posto", mapOf("phone" to "old"), revision = 3))
    }
    @AfterEach fun after() { Dispatchers.resetMain() }
    private fun vm() = StationManagementViewModel(SavedStateHandle(mapOf("stationId" to "station")), actions, read, sessions)
    @Test fun `revision conflict blocks stale form until refresh and edits only changed business fields`() = runTest(dispatcher) {
        val vm = vm(); vm.refresh(); advanceUntilIdle()
        vm.field("price", "1")
        assertFalse("price" in vm.state.value.fields)
        vm.field("phone", "new")
        coEvery { actions.edit("station", 3, mapOf("phone" to "new")) } throws ProfileHttpFailure(409)
        vm.save(); advanceUntilIdle()
        assertFalse(vm.state.value.canEdit)
        assertEquals(ManagementNotice.ReloadRequired, vm.state.value.notice)
        vm.refresh(); advanceUntilIdle()
        assertTrue(vm.state.value.canEdit)
        assertEquals("old", vm.state.value.fields["phone"])
    }
    @Test fun `revoked grant does not accept cached approval or announce success`() = runTest(dispatcher) {
        val vm = vm(); vm.refresh(); advanceUntilIdle()
        vm.reply("Response")
        coEvery { actions.reply(any(), any(), any()) } throws ProfileHttpFailure(403)
        vm.sendReply(); advanceUntilIdle()
        assertEquals(ManagementNotice.Refused, vm.state.value.notice)
        assertTrue(vm.state.value.scopes.isEmpty())
        assertEquals("", vm.state.value.reply)
    }
}
