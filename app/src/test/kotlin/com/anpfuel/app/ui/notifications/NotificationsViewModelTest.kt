package com.anpfuel.app.ui.notifications

import com.anpfuel.application.usecase.notification.AccountFeedState
import com.anpfuel.application.usecase.notification.LoadNotificationsUseCase
import com.anpfuel.application.usecase.notification.MarkNotificationsReadUseCase
import com.anpfuel.application.usecase.notification.NotificationFeed
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.AppNotificationContent
import com.anpfuel.domain.notification.AppNotificationSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** B-BR-NOTIF-01 screen behavior. */
@OptIn(ExperimentalCoroutinesApi::class)
class NotificationsViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val load = mockk<LoadNotificationsUseCase>()
    private val mark = mockk<MarkNotificationsReadUseCase>(relaxed = true)

    private val reply = AppNotification("a1", AppNotificationSource.ACCOUNT,
        AppNotificationContent.ForumReply("7b0e2a54-0000-4000-8000-000000000001", "Topic", "Ana"), 30, read = false)
    private val drop = AppNotification("l1", AppNotificationSource.LOCAL,
        AppNotificationContent.VehiclePriceDrop("Gol", "R$ 6,47"), 20, read = false)
    private val decided = AppNotification("a2", AppNotificationSource.ACCOUNT,
        AppNotificationContent.ContributionDecided("Posto", null, true), 10, read = true)

    @BeforeEach fun setUp() { Dispatchers.setMain(dispatcher) }
    @AfterEach fun tearDown() { Dispatchers.resetMain() }

    private fun loaded(): NotificationsViewModel {
        coEvery { load() } returns NotificationFeed(listOf(reply, drop, decided), 2, AccountFeedState.FRESH)
        return NotificationsViewModel(load, mark)
    }

    @Test
    fun `refresh shows the merged feed`() = runTest(dispatcher) {
        val vm = loaded()
        vm.refresh(); advanceUntilIdle()

        assertEquals(3, vm.state.value.items.size)
        assertEquals(2, vm.state.value.unread)
        assertEquals(false, vm.state.value.loading)
    }

    @Test
    fun `opening an item marks it read and returns its target`() = runTest(dispatcher) {
        val vm = loaded()
        vm.refresh(); advanceUntilIdle()

        assertEquals(NotificationTarget.ForumTopic("7b0e2a54-0000-4000-8000-000000000001"), vm.open(reply))
        advanceUntilIdle()

        assertEquals(1, vm.state.value.unread)
        coVerify(exactly = 1) { mark.mark(listOf(reply)) }
        assertEquals(NotificationTarget.Contributions, vm.open(decided))
        assertEquals(NotificationTarget.Explore, vm.open(drop))
    }

    @Test
    fun `mark all reads everything and calls the server only with account items`() = runTest(dispatcher) {
        val vm = loaded()
        vm.refresh(); advanceUntilIdle()

        vm.markAllRead(); advanceUntilIdle()

        assertEquals(0, vm.state.value.unread)
        coVerify(exactly = 1) { mark.markAll(true) }
    }

    @Test
    fun `a failed server mark is reported and the feed reloads`() = runTest(dispatcher) {
        val vm = loaded()
        vm.refresh(); advanceUntilIdle()
        coEvery { mark.mark(any()) } throws IOException("offline")

        vm.open(reply); advanceUntilIdle()

        assertTrue(vm.state.value.markFailed)
        coVerify(atLeast = 2) { load() }
    }
}
