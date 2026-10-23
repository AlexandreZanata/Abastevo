package com.anpfuel.app.ui.sync

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.ledger.ConfirmLedgerRestoreUseCase
import com.anpfuel.application.usecase.ledger.GetLedgerSyncStatusUseCase
import com.anpfuel.application.usecase.ledger.LedgerConflictChoice
import com.anpfuel.application.usecase.ledger.LedgerConflictRecord
import com.anpfuel.application.usecase.ledger.LedgerSyncStatus
import com.anpfuel.application.usecase.ledger.PreviewLedgerRestoreUseCase
import com.anpfuel.application.usecase.ledger.ResolveLedgerConflictUseCase
import com.anpfuel.application.usecase.ledger.ResolveOutcome
import com.anpfuel.application.usecase.ledger.RestoreDiff
import com.anpfuel.application.usecase.ledger.RestoreItemOutcome
import com.anpfuel.application.usecase.ledger.RestoreOutcome
import com.anpfuel.data.local.preferences.LedgerSyncFlagStore
import com.anpfuel.data.worker.LedgerSyncScheduler
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.repository.LedgerSyncScope
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

enum class SyncNotice { OFFLINE, ERROR, STALE, RESTORED, PARTIAL, RESOLVED }

private data class SyncActionResult(val notice: SyncNotice?, val staged: RestoreDiff? = null)

data class LedgerSyncUiState(
    val status: LedgerSyncStatus? = null,
    val busy: Boolean = false,
    val staged: RestoreDiff? = null,
    val notice: SyncNotice? = null,
)

/** No automatic uploads. Staged restore authority is deliberately never persisted. */
@HiltViewModel
class LedgerSyncViewModel @Inject constructor(
    private val status: GetLedgerSyncStatusUseCase,
    private val resolve: ResolveLedgerConflictUseCase,
    private val preview: PreviewLedgerRestoreUseCase,
    private val confirm: ConfirmLedgerRestoreUseCase,
    private val flags: LedgerSyncFlagStore,
    private val scope: LedgerSyncScope,
    private val sessions: AuthSessionStore,
    private val scheduler: LedgerSyncScheduler,
) : ViewModel() {
    private val state = MutableStateFlow(LedgerSyncUiState())
    val uiState = state.asStateFlow()
    private var generation = 0L

    fun refresh() {
        val request = ++generation
        val owner = sessions.load()?.accountId
        state.value = LedgerSyncUiState(busy = true)
        viewModelScope.launch {
            try {
                val result = status.invoke()
                if (request != generation) return@launch
                state.value = if (sessions.load()?.accountId == owner && result.accountId == owner) {
                    LedgerSyncUiState(status = result)
                } else LedgerSyncUiState(notice = SyncNotice.STALE)
            } catch (cancelled: CancellationException) { throw cancelled
            } catch (error: Exception) {
                if (request == generation) state.value = LedgerSyncUiState(notice = noticeFor(error))
            }
        }
    }

    fun setMasterEnabled(enabled: Boolean) {
        if (enabled && sessions.load() == null) return
        flags.setEnabled(enabled)
        if (!enabled) scheduler.cancel()
        refresh()
    }

    fun setDeviceEnabled(enabled: Boolean) {
        scope.setDeviceEnabled(enabled)
        if (!enabled) scheduler.cancel()
        refresh()
    }

    fun setDatasetEnabled(dataset: LedgerDataset, enabled: Boolean) {
        scope.setDatasetEnabled(dataset.wire, enabled)
        if (!enabled) scheduler.cancel()
        refresh()
    }

    private fun ready(): Boolean {
        val current = state.value.status ?: return false
        return !state.value.busy && current.accountId != null && current.accountId == sessions.load()?.accountId &&
            flags.isEnabled() && scope.isDeviceEnabled()
    }

    fun resolveConflict(record: LedgerConflictRecord, choice: LedgerConflictChoice) {
        if (choice == LedgerConflictChoice.CANCEL) return
        if (!ready() || !scope.isDatasetEnabled(record.dataset.wire)) return
        runAction {
            SyncActionResult(when (resolve(record.dataset, record.recordId, choice)) {
                is ResolveOutcome.Undecodable, ResolveOutcome.NoConflict, ResolveOutcome.NoContent -> SyncNotice.ERROR
                else -> SyncNotice.RESOLVED
            })
        }
    }

    fun previewRestore() {
        if (!ready()) return
        runAction {
            SyncActionResult(null, preview())
        }
    }

    fun cancelRestore() {
        ++generation
        state.value = state.value.copy(staged = null, busy = false, notice = null)
    }

    fun confirmRestore() {
        if (!ready()) return
        val diff = state.value.staged ?: return
        runAction {
            SyncActionResult(when (val result = confirm(diff)) {
                RestoreOutcome.StaleSnapshot -> SyncNotice.STALE
                RestoreOutcome.NothingToApply -> SyncNotice.RESTORED
                is RestoreOutcome.Applied -> if (result.items.any { it.outcome == RestoreItemOutcome.UNDECODABLE }) SyncNotice.PARTIAL else SyncNotice.RESTORED
            })
        }
    }

    private fun runAction(action: suspend () -> SyncActionResult) {
        val request = ++generation
        val owner = sessions.load()?.accountId
        state.value = state.value.copy(busy = true, staged = null, notice = null)
        viewModelScope.launch {
            try {
                val actionResult = action()
                if (request != generation) return@launch
                if (owner != sessions.load()?.accountId) {
                    state.value = LedgerSyncUiState(notice = SyncNotice.STALE)
                    return@launch
                }
                val result = status.invoke()
                if (request == generation && owner == sessions.load()?.accountId && result.accountId == owner) {
                    state.value = state.value.copy(status = result, busy = false, notice = actionResult.notice, staged = actionResult.staged)
                } else if (request == generation) state.value = LedgerSyncUiState(notice = SyncNotice.STALE)
            } catch (cancelled: CancellationException) { throw cancelled
            } catch (error: Exception) {
                if (request == generation) {
                    state.value = if (owner != sessions.load()?.accountId) LedgerSyncUiState(notice = SyncNotice.STALE)
                        else state.value.copy(busy = false, staged = null, notice = noticeFor(error))
                }
            }
        }
    }

    private fun noticeFor(error: Exception) = if (error is IOException) SyncNotice.OFFLINE else SyncNotice.ERROR
}
