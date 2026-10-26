package com.anpfuel.app.ui.map

import android.content.Context
import androidx.core.content.ContextCompat
import androidx.lifecycle.ViewModel
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.routing.NativeRouteRequest
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

@HiltViewModel
class GuidanceViewModel @Inject constructor(
    @ApplicationContext private val appContext: Context,
    private val sessions: AuthSessionStore,
) : ViewModel() {
    val serviceState = GuidanceServiceState.flow
    private val mutableResume = MutableStateFlow<GuidanceRecoveryStore.Saved?>(null)
    val resume: StateFlow<GuidanceRecoveryStore.Saved?> = mutableResume.asStateFlow()

    fun currentOwner(): String {
        return try {
            sessions.load()?.accountId ?: "anonymous"
        } catch (_: Exception) {
            "anonymous"
        }
    }

    fun refreshResume() {
        if (GuidanceServiceState.current().active) {
            mutableResume.value = null
            return
        }
        val saved = try {
            GuidanceRecoveryStore(appContext).load()
        } catch (_: Exception) {
            null
        }
        mutableResume.value = if (saved != null && saved.owner == currentOwner()) saved else null
        GuidanceServiceState.update { it.copy(resumeAvailable = mutableResume.value != null) }
    }

    fun start(request: NativeRouteRequest): Boolean {
        return try {
            request.validate()
            val intent = GuidanceService.startIntent(appContext, currentOwner(), request)
            ContextCompat.startForegroundService(appContext, intent)
            refreshResume()
            true
        } catch (_: Exception) {
            false
        }
    }

    fun resumeSaved(): Boolean {
        val saved = mutableResume.value ?: return false
        if (saved.owner != currentOwner()) return false
        return start(saved.request)
    }

    fun stop() {
        runCatching {
            appContext.startService(GuidanceService.stopIntent(appContext))
        }
        mutableResume.value = null
    }
}
