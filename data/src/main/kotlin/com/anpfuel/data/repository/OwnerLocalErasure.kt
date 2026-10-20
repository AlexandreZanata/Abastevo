package com.anpfuel.data.repository

import com.anpfuel.application.port.ContributionScopeProvider
import com.anpfuel.application.portable.PhotoCache
import com.anpfuel.data.local.dao.PhotoUploadSessionDao
import com.anpfuel.domain.repository.ContributionOutboxRepository
import javax.inject.Inject

/**
 * SEC04-D owner erasure for local private copies (AUD-05): after logout
 * or account deletion, no pending operation of the erased owner may
 * remain to send, and its private media copies go with it.
 *
 * Coverage: outbox rows scoped to the owner (payload-embedded scope,
 * no schema change), photo upload sessions of the owner
 * ([PhotoUploadSessionDao.deleteByOwner]), and sealed-cache copies for
 * those sessions (best-effort deletes; unknown ids are a no-op).
 * Unscoped/unparseable rows are kept (fail-closed, quarantined from
 * dispatch by the existing rule). Call BEFORE clearing identity keys:
 * scope resolution needs the current device identity.
 */
class OwnerLocalErasure @Inject constructor(
    private val outbox: ContributionOutboxRepository,
    private val photoSessions: PhotoUploadSessionDao,
    private val photoCache: PhotoCache,
    private val scopeProvider: ContributionScopeProvider,
    private val quality:com.anpfuel.application.port.QualityReportLocalErasure=com.anpfuel.application.port.QualityReportLocalErasure {true},
) {
    data class Report(val outboxPurged: Int, val sessionsPurged: Int, val cacheDeleted: Int,val qualityErased:Boolean=true)

    /** Current owner scope, or null when identity is unavailable. */
    suspend fun currentScopeOrNull(): String? = try {
        scopeProvider.currentScope().ownerScope.takeIf { it.isNotBlank() }
    } catch (_: Exception) {
        null
    }

    suspend fun purgeCurrentOwner(): Report {
        return purgeScope(currentScopeOrNull() ?: return Report(0, 0, 0,qualityErased=false))
    }

    suspend fun purgeScope(ownerScope: String): Report {
        if (ownerScope.isBlank()) return Report(0, 0, 0,qualityErased=false)
        val qualityErased=try {quality.eraseOwner(ownerScope)}catch(_:Exception){false}
        val outboxPurged = try {
            outbox.purgeOwned(ownerScope)
        } catch (_: Exception) {
            0
        }
        val sessions = try {
            photoSessions.listByOwner(ownerScope)
        } catch (_: Exception) {
            emptyList()
        }
        var cacheDeleted = 0
        for (session in sessions) {
            runCatching { photoCache.delete(session.photoId); cacheDeleted++ }
            runCatching { photoCache.delete(session.captureId); cacheDeleted++ }
        }
        val sessionsPurged = try {
            photoSessions.deleteByOwner(ownerScope)
        } catch (_: Exception) {
            0
        }
        return Report(outboxPurged, sessionsPurged, cacheDeleted,qualityErased)
    }
}
