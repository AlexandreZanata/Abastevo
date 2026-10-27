package com.anpfuel.app.ui.contributors

import com.anpfuel.application.usecase.contributor.ReadOwnProfileUseCase
import com.anpfuel.application.usecase.contributor.UpsertProfileUseCase
import com.anpfuel.domain.contributor.ContributorException
import com.anpfuel.domain.contributor.ProfileEdit
import com.anpfuel.domain.contributor.ProfileRead
import com.anpfuel.domain.contributor.ProfileVisibility
import com.anpfuel.domain.contributor.StoredProfile
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ProfileViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val readProfile = mockk<ReadOwnProfileUseCase>()
    private val upsertProfile = mockk<UpsertProfileUseCase>()
    private val stored = StoredProfile("Ana", emptyList(), ProfileVisibility.PUBLIC, true, false)
    private val read = ProfileRead(stored, listOf("Suporte"))
    private val edit = ProfileEdit("Ana", emptyList(), ProfileVisibility.PUBLIC, true, false)

    @BeforeEach fun setup() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun teardown() { Dispatchers.resetMain() }

    private fun model() = ProfileViewModel(readProfile, upsertProfile)

    @Test fun `refresh reads profile and roles`() = runTest(dispatcher) {
        coEvery { readProfile() } returns read
        val vm = model()
        vm.refresh()
        advanceUntilIdle()
        assertEquals(read, vm.uiState.value.read)
        assertNull(vm.uiState.value.notice)
    }

    @Test fun `save marks saved on success`() = runTest(dispatcher) {
        coEvery { readProfile() } returns ProfileRead(null, emptyList())
        coEvery { upsertProfile(edit) } returns read
        val vm = model()
        vm.save(edit)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.saved)
        assertFalse(vm.uiState.value.saving)
        assertEquals("Ana", vm.uiState.value.read?.profile?.displayName)
    }

    @Test fun `forbidden quota conflict and offline map distinctly`() = runTest(dispatcher) {
        coEvery { readProfile() } returns ProfileRead(null, emptyList())
        val vm = model()
        coEvery { upsertProfile(edit) } throws ContributorException.Forbidden("contributor.forbidden")
        vm.save(edit)
        advanceUntilIdle()
        assertEquals(ProfileNotice.FORBIDDEN, vm.uiState.value.notice)
        coEvery { upsertProfile(edit) } throws ContributorException.Quota("contributor.quota")
        vm.save(edit)
        advanceUntilIdle()
        assertEquals(ProfileNotice.QUOTA, vm.uiState.value.notice)
        coEvery { upsertProfile(edit) } throws ContributorException.Conflict("contributor.exists")
        vm.save(edit)
        advanceUntilIdle()
        assertEquals(ProfileNotice.CONFLICT, vm.uiState.value.notice)
        coEvery { upsertProfile(edit) } returns read
        vm.save(edit)
        advanceUntilIdle()
        assertNull(vm.uiState.value.notice)
    }
}
