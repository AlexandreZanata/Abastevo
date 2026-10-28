package com.anpfuel.app.ui.map

import android.content.Context
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.work.WorkManager
import com.anpfuel.data.map.MapDistribution
import com.anpfuel.data.map.MapDistributionConfig
import com.anpfuel.data.map.MapDownloadException
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.data.map.PACKAGE_FAILED
import com.anpfuel.data.map.PACKAGE_READY
import com.anpfuel.data.map.VerifiedMapManifest
import com.anpfuel.data.worker.MapDownloadScheduler
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class MapPackageUi(
    val packageId: String,
    val bytes: Long,
    val downloadedBytes: Long,
    val status: String,
    val error: String?,
)

data class MapDownloadsUi(
    val loading: Boolean = true,
    val missingUrl: Boolean = false,
    val missingRegion: Boolean = false,
    val missingKeys: Boolean = false,
    val keysInvalid: Boolean = false,
    val activeGeneration: String? = null,
    val published: Boolean = false,
    val candidateGeneration: String? = null,
    val candidateBytes: Long = 0,
    val downloadedBytes: Long = 0,
    val candidatePackages: List<MapPackageUi> = emptyList(),
    val candidateDependencies: List<String> = emptyList(),
    val downloading: Boolean = false,
    val error: String? = null,
    val retryable: Boolean = false,
    val wifiOnly: Boolean = true,
    val inactiveGenerations: List<String> = emptyList(),
) {
    val unconfigured: Boolean get() = missingUrl || missingRegion || missingKeys || keysInvalid
}

/** Enqueue/cancel seam over WorkManager unique work; faked in unit tests. */
interface MapWorkControl {
    fun enqueue(generation: String, requireUnmetered: Boolean)
    fun cancel(generation: String)
}

class WorkManagerControl @Inject constructor(
    @ApplicationContext private val context: Context,
    private val config: MapDistributionConfig,
) : MapWorkControl {
    override fun enqueue(generation: String, requireUnmetered: Boolean) {
        MapDownloadScheduler.enqueue(WorkManager.getInstance(context), generation, config.baseUrl, requireUnmetered)
    }

    override fun cancel(generation: String) {
        MapDownloadScheduler.cancel(WorkManager.getInstance(context), generation)
    }
}

/**
 * Usable download flow: configured origin -> real envelope -> pinned-key
 * verification -> catalog/dependencies/size -> explicit unique-work download
 * with progress/cancel/retry -> verified activation -> observable state.
 * Missing configuration shows the specific pendency; a network failure is
 * reported as retryable and never as "no package" nor success.
 */
@HiltViewModel
class MapDownloadsViewModel @Inject constructor(
    private val config: MapDistributionConfig,
    private val distribution: MapDistribution,
    private val repository: MapPackageRepository,
    private val store: MapPackageStore,
    private val work: MapWorkControl,
    private val preferences: com.anpfuel.domain.repository.UserPreferencesRepository,
) : ViewModel() {
    private val _ui = MutableStateFlow(MapDownloadsUi())
    val ui: StateFlow<MapDownloadsUi> = _ui.asStateFlow()
    private var pollJob: Job? = null

    init {
        refresh()
    }

    fun refresh() {
        pollJob?.cancel()
        if (!config.configured || config.defaultRegion == null) {
            _ui.update {
                it.copy(
                    loading = false, missingUrl = config.missingUrl, missingRegion = config.defaultRegion == null,
                    missingKeys = config.missingKeys, keysInvalid = config.keysInvalid,
                )
            }
            return
        }
        viewModelScope.launch {
            _ui.update { it.copy(loading = true, error = null, retryable = false) }
            try {
                val wifiOnly = runCatching { preferences.getPreferences().autoSyncOnWifi }.getOrDefault(true)
                val active = store.activeGeneration()
                val published = repository.publishedState()?.complete == true
                val inactive = runCatching { store.inactiveGenerations() }.getOrDefault(emptyList())
                _ui.update {
                    val settled = it.candidateGeneration != null && it.candidateGeneration == active?.generation
                    it.copy(
                        loading = false, missingUrl = false, missingRegion = false,
                        missingKeys = false, keysInvalid = false,
                        activeGeneration = active?.generation, published = published,
                        wifiOnly = wifiOnly, inactiveGenerations = inactive,
                        // An already-active candidate is current, not an update.
                        candidateGeneration = if (settled) null else it.candidateGeneration,
                        candidatePackages = if (settled) emptyList() else it.candidatePackages,
                        candidateDependencies = if (settled) emptyList() else it.candidateDependencies,
                    )
                }
            } catch (_: Exception) {
                _ui.update { it.copy(loading = false, error = "load-failed", retryable = true) }
            }
        }
    }

    fun checkForUpdate() {
        pollJob?.cancel()
        val region = config.defaultRegion
        if (!config.configured || region == null) {
            refresh()
            return
        }
        viewModelScope.launch {
            _ui.update { it.copy(loading = true, error = null, retryable = false) }
            try {
                val envelope = distribution.fetchEnvelope(region)
                val verified = try {
                    repository.importManifest(envelope.decodeToString(), config.trustedKeys)
                } catch (e: MapDownloadException) {
                    if (e.message == "active-generation-reimport") {
                        // Already on this generation: show current state, no fake update.
                        refresh()
                        return@launch
                    }
                    throw e
                }
                showCandidate(verified)
            } catch (error: MapDownloadException) {
                _ui.update { it.copy(loading = false, error = error.message, retryable = false) }
            } catch (_: IOException) {
                _ui.update { it.copy(loading = false, error = "network-offline", retryable = true) }
            } catch (_: Exception) {
                _ui.update { it.copy(loading = false, error = "check-failed", retryable = true) }
            }
        }
    }

    private suspend fun showCandidate(verified: VerifiedMapManifest) {
        val rows = store.packages(verified.manifest.generation)
        val active = store.activeGeneration()
        _ui.update {
            it.copy(
                loading = false,
                candidateGeneration = verified.manifest.generation,
                candidateBytes = rows.sumOf { row -> row.bytes },
                downloadedBytes = rows.sumOf { row -> row.downloadedBytes },
                candidatePackages = rows.map { row ->
                    MapPackageUi(row.packageId, row.bytes, row.downloadedBytes, row.status, row.error)
                },
                candidateDependencies = verified.manifest.dependencies,
                activeGeneration = active?.generation,
                published = repository.publishedState()?.complete == true,
            )
        }
    }

    fun download() {
        val generation = _ui.value.candidateGeneration ?: return
        pollJob?.cancel()
        // Enqueue synchronously with the last refreshed Wi-Fi preference so the
        // unique work exists even if observation is cancelled immediately after.
        work.enqueue(generation, _ui.value.wifiOnly)
        pollJob = viewModelScope.launch {
            observeDownloadInner(generation)
        }
    }

    private suspend fun observeDownloadInner(generation: String) {
        _ui.update { it.copy(downloading = true, error = null, retryable = false) }
        try {
            while (true) {
                delay(500)
                val rows = store.packages(generation)
                _ui.update {
                    it.copy(
                        downloadedBytes = rows.sumOf { row -> row.downloadedBytes },
                        candidatePackages = rows.map { row ->
                            MapPackageUi(row.packageId, row.bytes, row.downloadedBytes, row.status, row.error)
                        },
                    )
                }
                val failed = rows.firstOrNull { it.status == PACKAGE_FAILED }
                if (failed != null) {
                    _ui.update { it.copy(downloading = false, error = failed.error ?: "download-failed", retryable = true) }
                    return
                }
                if (rows.isNotEmpty() && rows.all { it.status == PACKAGE_READY }) {
                    // The worker flips Room to active; observe the flip.
                    var guard = 0
                    while (guard++ < 40) {
                        delay(500)
                        val active = store.activeGeneration()
                        if (active?.generation == generation) {
                            _ui.update {
                                it.copy(
                                    downloading = false, activeGeneration = generation,
                                    published = repository.publishedState()?.complete == true,
                                    candidateGeneration = null, candidatePackages = emptyList(),
                                    candidateDependencies = emptyList(),
                                )
                            }
                            return
                        }
                    }
                    _ui.update { it.copy(downloading = false, error = "activation-pending", retryable = true) }
                    return
                }
            }
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            _ui.update { it.copy(downloading = false, error = "observe-failed", retryable = true) }
        }
    }

    fun cancel() {
        val generation = _ui.value.candidateGeneration ?: return
        pollJob?.cancel()
        work.cancel(generation)
        viewModelScope.launch {
            val rows = store.packages(generation)
            _ui.update {
                it.copy(
                    downloading = false,
                    downloadedBytes = rows.sumOf { row -> row.downloadedBytes },
                    candidatePackages = rows.map { row ->
                        MapPackageUi(row.packageId, row.bytes, row.downloadedBytes, row.status, row.error)
                    },
                )
            }
        }
    }

    fun retry() {
        _ui.update { it.copy(error = null, retryable = false) }
        if (_ui.value.candidateGeneration == null) {
            checkForUpdate()
        } else {
            download()
        }
    }

    /**
     * Deletes a staged inactive generation. The repository refuses the active
     * generation and any leased generation (route/navigation readers); refusal
     * surfaces as an explicit in-use state, never silent navigation cancel.
     */
    fun deleteInactive(generation: String) {
        viewModelScope.launch {
            _ui.update { it.copy(error = null, retryable = false) }
            try {
                val removed = repository.deleteGeneration(generation)
                if (!removed) {
                    _ui.update { it.copy(error = "in-use", retryable = false) }
                    return@launch
                }
                refresh()
            } catch (_: Exception) {
                _ui.update { it.copy(error = "delete-failed", retryable = true) }
            }
        }
    }

    /**
     * Activates the already-downloaded active generation (repair path for an
     * interrupted publish or a legacy row): the worker re-verifies, publishes
     * and flips without inventing any data.
     */
    fun activateActive() {
        val generation = _ui.value.activeGeneration ?: return
        viewModelScope.launch {
            val rows = store.packages(generation)
            _ui.update {
                it.copy(
                    candidateGeneration = generation,
                    candidateBytes = rows.sumOf { row -> row.bytes },
                    downloadedBytes = rows.sumOf { row -> row.downloadedBytes },
                    candidatePackages = rows.map { row ->
                        MapPackageUi(row.packageId, row.bytes, row.downloadedBytes, row.status, row.error)
                    },
                )
            }
            download()
        }
    }
}
