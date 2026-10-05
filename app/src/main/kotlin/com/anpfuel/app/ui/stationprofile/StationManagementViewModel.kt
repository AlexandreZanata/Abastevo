package com.anpfuel.app.ui.stationprofile

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.profile.*
import com.anpfuel.data.remote.profile.ProfileHttpFailure
import com.anpfuel.domain.profile.ProfileBusinessFieldsRule
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

enum class ManagementNotice { None, SignIn, Unavailable, Invalid, Refused, ReloadRequired, Saved, ReplySent }

data class StationManagementState(
    val fields: Map<String, String> = emptyMap(),
    val original: Map<String, String> = emptyMap(),
    val revision: Int = 0,
    val scopes: Set<String> = emptySet(),
    val busy: Boolean = false,
    val notice: ManagementNotice = ManagementNotice.None,
    val reply: String = "",
    val fuelWire: String = "GASOLINE_REGULAR",
) {
    val changed: Map<String, String> get() = fields.filter { (key, value) -> original[key] != value }
    val canEdit: Boolean get() = revision > 0 && "profile.edit" in scopes && !busy
    val canReply: Boolean get() = "reply.official" in scopes && !busy
}

@HiltViewModel
class StationManagementViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val actions: StationProfileActions,
    private val read: GetStationProfileUseCase,
    private val sessions: AuthSessionStore,
) : ViewModel() {
    val stationId: String = checkNotNull(savedStateHandle["stationId"])
    private val mutableState = MutableStateFlow(StationManagementState())
    val state = mutableState.asStateFlow()

    fun field(key: String, value: String) {
        if (!state.value.canEdit || key !in com.anpfuel.domain.profile.PUBLIC_BUSINESS_KEYS || value.length > 1024) return
        mutableState.value = state.value.copy(fields = state.value.fields + (key to value), notice = ManagementNotice.None)
    }
    fun reply(text: String) {
        if (state.value.canReply && text.length <= 1024) mutableState.value = state.value.copy(reply = text, notice = ManagementNotice.None)
    }
    fun fuel(wire: String) {
        if (state.value.canReply && wire in com.anpfuel.data.mapper.WireFuelMapper.wireValues()) mutableState.value = state.value.copy(fuelWire = wire)
    }

    private fun runAction(block: suspend () -> Unit) {
        if (state.value.busy) return
        val owner = sessions.load()?.accountId
        mutableState.value = state.value.copy(busy = true, notice = ManagementNotice.None)
        viewModelScope.launch {
            try { block() }
            catch (error: CancellationException) { throw error }
            catch (_: ProfileSignInRequired) { mutableState.value = StationManagementState(notice = ManagementNotice.SignIn) }
            catch (_: ProfileInputInvalid) { mutableState.value = state.value.copy(notice = ManagementNotice.Invalid) }
            catch (error: ProfileHttpFailure) {
                mutableState.value = if (error.status == 409) state.value.copy(scopes = emptySet(), notice = ManagementNotice.ReloadRequired)
                else if (error.status in setOf(401, 403, 404)) StationManagementState(notice = if (error.status == 401) ManagementNotice.SignIn else ManagementNotice.Refused)
                else state.value.copy(scopes = emptySet(), notice = ManagementNotice.Unavailable)
            }
            catch (_: Exception) { mutableState.value = state.value.copy(scopes = emptySet(), notice = ManagementNotice.Unavailable) }
            finally {
                if (sessions.load()?.accountId != owner) mutableState.value = StationManagementState(notice = ManagementNotice.SignIn)
                mutableState.value = state.value.copy(busy = false)
            }
        }
    }

    fun refresh() = runAction {
        // Approved claim scopes are advisory; live grant/window/account/operator is server-checked on each command.
        val owned = actions.mine(stationId)
        val profile = (read(stationId) as? StationProfileOutcome.Found)?.profile ?: throw ProfileInputInvalid()
        mutableState.value = StationManagementState(
            fields = profile.business, original = profile.business, revision = profile.revision,
            scopes = owned.filter { it.state == "approved" }.flatMap { it.scopes }.toSet(), busy = true,
        )
    }
    fun save() {
        if (!state.value.canEdit) return
        val changed = state.value.changed
        if (!ProfileBusinessFieldsRule.valid(changed)) { mutableState.value = state.value.copy(notice = ManagementNotice.Invalid); return }
        runAction {
            actions.edit(stationId, state.value.revision, changed)
            val profile = (read(stationId) as? StationProfileOutcome.Found)?.profile
            if (profile == null) {
                mutableState.value = state.value.copy(scopes = emptySet(), notice = ManagementNotice.ReloadRequired)
            } else {
                mutableState.value = state.value.copy(fields = profile.business, original = profile.business, revision = profile.revision, notice = ManagementNotice.Saved)
            }
        }
    }
    fun sendReply() {
        if (!state.value.canReply) return
        runAction {
            actions.reply(stationId, state.value.fuelWire, state.value.reply)
            mutableState.value = state.value.copy(reply = "", notice = ManagementNotice.ReplySent)
        }
    }
}
