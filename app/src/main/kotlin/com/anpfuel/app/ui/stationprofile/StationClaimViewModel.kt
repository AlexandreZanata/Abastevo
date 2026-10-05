package com.anpfuel.app.ui.stationprofile

import android.content.ContentResolver
import android.net.Uri
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.profile.OwnedProfileClaim
import com.anpfuel.application.usecase.profile.ProfileInputInvalid
import com.anpfuel.application.usecase.profile.ProfileSignInRequired
import com.anpfuel.application.usecase.profile.StationProfileActions
import com.anpfuel.data.profile.SignedClaimDocument
import com.anpfuel.data.remote.profile.ProfileHttpFailure
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.UUID
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

enum class ClaimNotice { None, Unavailable, SignIn, Invalid, FileUnavailable, FileSaved, FileReady, ProofReceived, Refused, Changed, Cancelled }

data class StationClaimState(
    val claims: List<OwnedProfileClaim> = emptyList(),
    val selected: OwnedProfileClaim? = null,
    val role: String = "manager",
    val busy: Boolean = false,
    val fileBytes: Int = 0,
    val notice: ClaimNotice = ClaimNotice.None,
)

@HiltViewModel
class StationClaimViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val actions: StationProfileActions,
    private val sessions: AuthSessionStore,
) : ViewModel() {
    val stationId: String = checkNotNull(savedStateHandle["stationId"])
    private val mutableState = MutableStateFlow(StationClaimState())
    val state = mutableState.asStateFlow()
    // No URI, proof, credentials or private claim saved across process death.
    private var key = UUID.randomUUID().toString()
    private var owner: String? = null
    private var prepared: Pair<OwnedProfileClaim, ByteArray>? = null

    private fun clearFile() { prepared?.second?.fill(0); prepared = null }
    override fun onCleared() { clearFile(); super.onCleared() }

    fun role(role: String) {
        if (mutableState.value.busy || role !in setOf("manager", "administrator")) return
        if (role != mutableState.value.role) key = UUID.randomUUID().toString()
        mutableState.value = mutableState.value.copy(role = role)
    }

    fun select(claim: OwnedProfileClaim) {
        if (mutableState.value.busy || claim !in mutableState.value.claims) return
        clearFile()
        mutableState.value = mutableState.value.copy(selected = claim, notice = ClaimNotice.None, fileBytes = 0)
    }

    private fun runAction(block: suspend () -> Unit) {
        if (mutableState.value.busy) return
        val currentOwner = sessions.load()?.accountId
        if (currentOwner != owner) {
            clearFile()
            owner = currentOwner
            key = UUID.randomUUID().toString()
            mutableState.value = StationClaimState(role = mutableState.value.role)
        }
        mutableState.value = mutableState.value.copy(busy = true, notice = ClaimNotice.None)
        viewModelScope.launch {
            try {
                block()
            } catch (error: CancellationException) {
                throw error
            } catch (_: ProfileSignInRequired) {
                clearFile()
                mutableState.value = StationClaimState(notice = ClaimNotice.SignIn)
            } catch (_: ProfileInputInvalid) {
                mutableState.value = mutableState.value.copy(notice = ClaimNotice.Invalid)
            } catch (error: ProfileHttpFailure) {
                if (error.status == 401 || error.status == 403 || error.status == 404) {
                    clearFile()
                    mutableState.value = StationClaimState(notice = if (error.status == 401) ClaimNotice.SignIn else ClaimNotice.Refused)
                } else {
                    mutableState.value = mutableState.value.copy(notice = if (error.status == 409) ClaimNotice.Changed else ClaimNotice.Unavailable)
                }
            } catch (_: Exception) {
                mutableState.value = mutableState.value.copy(notice = ClaimNotice.Unavailable)
            } finally {
                if (sessions.load()?.accountId != owner) { clearFile(); mutableState.value = StationClaimState(notice = ClaimNotice.SignIn) }
                mutableState.value = mutableState.value.copy(busy = false)
            }
        }
    }

    fun refresh() = runAction {
        val previous = mutableState.value.selected?.id
        val claims = actions.mine(stationId)
        mutableState.value = mutableState.value.copy(claims = claims, selected = claims.firstOrNull { it.id == previous })
    }

    fun open() = runAction {
        val claim = actions.open(stationId, mutableState.value.role, setOf("profile.edit", "reply.official"), key)
        mutableState.value = mutableState.value.copy(selected = claim, claims = (listOf(claim) + mutableState.value.claims).distinctBy { it.id })
    }

    fun reissue() = runAction {
        val old = mutableState.value.selected ?: throw ProfileInputInvalid()
        clearFile()
        mutableState.value = mutableState.value.copy(fileBytes = 0)
        val claim = actions.reissue(old.id)
        mutableState.value = mutableState.value.copy(selected = claim, claims = mutableState.value.claims.map { if (it.id == claim.id) claim else it })
    }

    fun cancel() = runAction {
        val claim = mutableState.value.selected ?: throw ProfileInputInvalid()
        actions.cancel(claim.id)
        clearFile()
        key = UUID.randomUUID().toString()
        mutableState.value = mutableState.value.copy(selected = null, notice = ClaimNotice.Cancelled, claims = actions.mine(stationId))
    }

    fun export(resolver: ContentResolver, uri: Uri, claimId: String, declarationId: String) = runAction {
        val live = actions.status(claimId)
        if (sessions.load()?.accountId != owner) throw ProfileSignInRequired()
        if (!live.canSupplyProof || live.declarationId != declarationId || live.expiresAt <= System.currentTimeMillis() / 1000L) throw ProfileInputInvalid()
        try {
            SignedClaimDocument.export(resolver, uri, live.declaration)
            mutableState.value = mutableState.value.copy(notice = ClaimNotice.FileSaved)
        } catch (error: CancellationException) { throw error }
        catch (_: Exception) { mutableState.value = mutableState.value.copy(notice = ClaimNotice.FileUnavailable) }
    }

    fun import(resolver: ContentResolver, uri: Uri, claimId: String, declarationId: String) = runAction {
        val live = actions.status(claimId)
        if (sessions.load()?.accountId != owner) throw ProfileSignInRequired()
        if (!live.canSupplyProof || live.declarationId != declarationId || live.expiresAt <= System.currentTimeMillis() / 1000L) throw ProfileInputInvalid()
        val bytes = try { SignedClaimDocument.read(resolver, uri) }
        catch (error: CancellationException) { throw error }
        catch (_: Exception) { mutableState.value = mutableState.value.copy(notice = ClaimNotice.FileUnavailable); return@runAction }
        clearFile()
        prepared = live to bytes
        mutableState.value = mutableState.value.copy(notice = ClaimNotice.FileReady, fileBytes = bytes.size)
    }

    fun submitPrepared() = runAction {
        val file = prepared ?: throw ProfileInputInvalid()
        actions.submit(file.first, file.second)
        clearFile()
        mutableState.value = mutableState.value.copy(notice = ClaimNotice.ProofReceived, fileBytes = 0)
    }
}
