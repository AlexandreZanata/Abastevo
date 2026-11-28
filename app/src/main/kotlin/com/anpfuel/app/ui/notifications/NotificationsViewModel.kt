package com.anpfuel.app.ui.notifications

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.usecase.notification.AccountFeedState
import com.anpfuel.application.usecase.notification.LoadNotificationsUseCase
import com.anpfuel.application.usecase.notification.MarkNotificationsReadUseCase
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.AppNotificationContent
import com.anpfuel.domain.notification.AppNotificationSource
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class NotificationsUiState(
    val loading: Boolean = true,
    val items: List<AppNotification> = emptyList(),
    val unread: Int = 0,
    val accountState: AccountFeedState = AccountFeedState.SIGNED_OUT,
    /** A mark-as-read call to the server failed; the items stay unread. */
    val markFailed: Boolean = false,
)

/** Where tapping an item leads (B-BR-NOTIF-01). */
sealed interface NotificationTarget {
    data class ForumTopic(val topicId: String) : NotificationTarget
    data object Contributions : NotificationTarget
    data object Explore : NotificationTarget
}

/** B-BR-NOTIF-01 — the Notifications screen: one list, mark read, open the target. */
@HiltViewModel
class NotificationsViewModel @Inject constructor(
    private val load: LoadNotificationsUseCase,
    private val markRead: MarkNotificationsReadUseCase,
) : ViewModel() {

    private val _state = MutableStateFlow(NotificationsUiState())
    val state: StateFlow<NotificationsUiState> = _state.asStateFlow()

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(loading = it.items.isEmpty()) }
            val feed = load()
            _state.update { it.copy(loading = false, items = feed.items, unread = feed.unread, accountState = feed.accountState) }
        }
    }

    /** Marks [item] read (optimistically) and returns where to go. */
    fun open(item: AppNotification): NotificationTarget {
        if (!item.read) {
            applyRead { it.id == item.id && it.source == item.source }
            viewModelScope.launch { guarded { markRead.mark(listOf(item)) } }
        }
        return targetOf(item)
    }

    fun markAllRead() {
        val hasAccountItems = _state.value.items.any { it.source == AppNotificationSource.ACCOUNT && !it.read }
        applyRead { true }
        viewModelScope.launch { guarded { markRead.markAll(hasAccountItems) } }
    }

    private fun applyRead(predicate: (AppNotification) -> Boolean) {
        _state.update { state ->
            val items = state.items.map { if (predicate(it)) it.copy(read = true) else it }
            state.copy(items = items, unread = items.count { !it.read }, markFailed = false)
        }
    }

    private suspend fun guarded(block: suspend () -> Unit) {
        try {
            block()
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: Exception) {
            _state.update { it.copy(markFailed = true) }
            refresh()
        }
    }

    companion object {
        fun targetOf(item: AppNotification): NotificationTarget = when (val content = item.content) {
            is AppNotificationContent.ForumReply -> NotificationTarget.ForumTopic(content.topicId)
            is AppNotificationContent.ContributionDecided -> NotificationTarget.Contributions
            is AppNotificationContent.CityAverageDrop,
            is AppNotificationContent.VehiclePriceDrop,
            is AppNotificationContent.NewSurveyWeek,
            -> NotificationTarget.Explore
        }
    }
}
