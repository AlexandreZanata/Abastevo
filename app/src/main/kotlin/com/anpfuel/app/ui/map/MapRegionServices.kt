package com.anpfuel.app.ui.map

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import com.anpfuel.data.map.MapDistribution
import com.anpfuel.data.map.MapDistributionConfig
import com.anpfuel.data.map.MapManifestVerifier
import com.anpfuel.data.map.MapPackageRepository
import com.anpfuel.data.map.MapPackageStore
import com.anpfuel.data.map.MapRegionDismissalStore
import com.anpfuel.data.map.MapRegionResolver
import com.anpfuel.domain.map.MapRegionCatalog
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.ConcurrentHashMap
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** A CoMaps Brazil leaf as the UI shows it. */
data class MapRegionUi(val countryId: String, val slug: String, val name: String) {
    companion object {
        fun of(countryId: String): MapRegionUi =
            MapRegionUi(countryId, MapRegionCatalog.slugOf(countryId), MapRegionCatalog.displayName(countryId))

        fun ofSlug(slug: String): MapRegionUi? = MapRegionCatalog.countryIdForSlug(slug)?.let(::of)
    }
}

/**
 * Region the map screen resolved for the opened city, shared with the
 * downloads screen (every "Baixar" entry point downloads that region only).
 * [requestDownload] marks a one-shot "start right away" from the suggestion.
 */
@Singleton
class MapRegionSelection @Inject constructor() {
    private val _selected = MutableStateFlow<String?>(null)
    val selected: StateFlow<String?> = _selected.asStateFlow()
    @Volatile private var pendingDownload: String? = null

    fun select(slug: String?) {
        _selected.value = slug
    }

    fun requestDownload(slug: String) {
        _selected.value = slug
        pendingDownload = slug
    }

    /** True once for the slug that [requestDownload] marked. */
    @Synchronized
    fun consumeDownloadRequest(slug: String): Boolean {
        if (pendingDownload != slug) return false
        pendingDownload = null
        return true
    }
}

/** Seam over region lookup, dismissals, network and the signed manifest (faked in unit tests). */
interface MapRegionServices {
    val downloadsConfigured: Boolean
    suspend fun regionFor(latitude: Double, longitude: Double): String?
    fun isDismissed(slug: String): Boolean
    fun dismiss(slug: String)
    fun select(slug: String?)
    fun requestDownload(slug: String)
    /** Automatic manifest fetches only on an unmetered (Wi-Fi) network. */
    fun canFetchManifestAutomatically(): Boolean
    /** Total bytes of the region's signed, verified manifest. */
    suspend fun regionDownloadBytes(slug: String): Long
    /** REGION package id of the active, completely published generation. */
    suspend fun activeRegionSlug(): String?
}

class DefaultMapRegionServices @Inject constructor(
    @ApplicationContext private val context: Context,
    private val resolver: MapRegionResolver,
    private val dismissals: MapRegionDismissalStore,
    private val selection: MapRegionSelection,
    private val config: MapDistributionConfig,
    private val distribution: MapDistribution,
    private val store: MapPackageStore,
    private val repository: MapPackageRepository,
) : MapRegionServices {
    override val downloadsConfigured: Boolean get() = config.configured

    override suspend fun regionFor(latitude: Double, longitude: Double): String? =
        resolver.regionFor(latitude, longitude)

    override fun isDismissed(slug: String): Boolean = dismissals.isDismissed(slug)

    override fun dismiss(slug: String) = dismissals.dismiss(slug)

    override fun select(slug: String?) = selection.select(slug)

    override fun requestDownload(slug: String) = selection.requestDownload(slug)

    override fun canFetchManifestAutomatically(): Boolean {
        val connectivity = context.getSystemService(ConnectivityManager::class.java) ?: return false
        val capabilities = connectivity.getNetworkCapabilities(connectivity.activeNetwork) ?: return false
        return capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
            capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)
    }

    override suspend fun regionDownloadBytes(slug: String): Long {
        sizes[slug]?.let { return it }
        val envelope = distribution.fetchEnvelope(slug).decodeToString()
        val verified = MapManifestVerifier.verify(envelope, config.trustedKeys)
        if (verified.manifest.regionId != slug) throw IllegalStateException("map-region-mismatch")
        return verified.manifest.packages.sumOf { it.bytes }.also { sizes[slug] = it }
    }

    override suspend fun activeRegionSlug(): String? {
        val active = store.activeGeneration() ?: return null
        val published = repository.publishedState() ?: return null
        if (!published.complete || published.generation != active.generation) return null
        return store.packages(active.generation).firstOrNull { it.role == "REGION" }?.packageId
    }

    private companion object {
        // Per-process cache: one signed manifest fetch per region and session.
        val sizes = ConcurrentHashMap<String, Long>()
    }
}
