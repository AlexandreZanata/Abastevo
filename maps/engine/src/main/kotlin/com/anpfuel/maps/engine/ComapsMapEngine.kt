package com.anpfuel.maps.engine

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.View
import app.organicmaps.sdk.util.Config
import app.organicmaps.sdk.Framework
import app.organicmaps.sdk.MapViewHelper
import app.organicmaps.sdk.OrganicMaps
import app.organicmaps.sdk.PlacePageActivationListener
import app.organicmaps.sdk.bookmarks.data.BookmarkCategory
import app.organicmaps.sdk.bookmarks.data.BookmarkManager
import app.organicmaps.sdk.bookmarks.data.DataChangedListener
import app.organicmaps.sdk.bookmarks.data.FeatureId
import app.organicmaps.sdk.bookmarks.data.MapObject
import app.organicmaps.sdk.location.AndroidNativeProvider
import app.organicmaps.sdk.location.BaseLocationProvider
import app.organicmaps.sdk.location.LocationProviderFactory
import app.organicmaps.sdk.search.SearchEngine
import app.organicmaps.sdk.search.SearchListener
import app.organicmaps.sdk.search.SearchResult
import app.organicmaps.sdk.sound.TtsPlayer
import app.organicmaps.sdk.location.NavigationFixBridge
import app.organicmaps.sdk.location.TrackRecorder
import app.organicmaps.sdk.routing.RoutingInfo
import app.organicmaps.sdk.util.ConnectionState
import app.organicmaps.sdk.widget.placepage.PlacePageData
import com.anpfuel.domain.routing.GuidancePolicy
import com.anpfuel.domain.routing.NativeRouteRequest
import com.anpfuel.domain.routing.NativeRouteResult
import com.anpfuel.domain.routing.NavigationFix
import com.anpfuel.domain.routing.NavigationFixPolicy
import com.anpfuel.domain.routing.NavigationProgress
import java.io.File

/**
 * Real CoMaps-derived adapter. It owns initialization, the map surface, search,
 * car routing and a bookmarked Abastevo overlay; no app.organicmaps type is
 * exposed to callers.
 */
class ComapsMapEngine(
    private val flavor: String = "fdroid",
    private val versionCode: Int = 1,
    private val versionName: String = "1",
) : MapEngine {
    private var organicMaps: OrganicMaps? = null
    private var initializing = false
    private var initialized = false
    @Volatile private var renderingReady = false
    private var appContext: Context? = null
    private var overlayListener: ((MapOverlaySelection) -> Unit)? = null
    private var placePageListener: PlacePageActivationListener? = null
    private var searchListener: SearchListener? = null
    private val routePlanner = ComapsRoutePlanner { initialized }
    private var overlayCategory = -1L
    private var overlayClusters: List<MapOverlayCluster> = emptyList()
    private var overlayMarkers: List<MapOverlayMarker> = emptyList()
    private var overlayLoadingListener: BookmarkManager.BookmarksLoadingListener? = null
    private var overlayCategoriesListener: DataChangedListener? = null
    private var overlayGeneration = 0L
    private val readyCallbacks = MapReadyCallbacks()
    private var searchGeneration = 0L
    @Volatile
    private var lastViewport: MapCamera? = null
    private var lastReadyRequest: NativeRouteRequest? = null
    private var guidanceRequest: NativeRouteRequest? = null
    private var guiding = false
    private var guidanceGeneration = 0L
    private var rerouteStartNanos: Long? = null
    private var lastProgress: NavigationProgress? = null

    @Synchronized
    override fun initialize(context: Context, onReady: () -> Unit): Boolean {
        readyCallbacks.add(onReady)
        if (initialized) {
            appContext?.let { app ->
                if (Looper.myLooper() == Looper.getMainLooper()) {
                    syncStorageAndReloadOnMain(app, forceReload = false)
                }
            }
            installPlacePageListener()
            readyCallbacks.take().forEach { it() }
            return true
        }
        if (initializing) {
            return true
        }
        initializing = true
        val app = context.applicationContext
        appContext = app
        ConnectionState.INSTANCE.initialize(app)
        val locationFactory = object : LocationProviderFactory {
            override fun isGoogleLocationAvailable(context: Context) = false
            override fun getProvider(
                context: Context,
                listener: BaseLocationProvider.Listener,
            ): BaseLocationProvider = AndroidNativeProvider(context, listener)
        }
        val engine = OrganicMaps(
            app,
            flavor,
            app.packageName,
            versionCode,
            versionName,
            "${app.packageName}.mapfileprovider",
            locationFactory,
        )
        runCatching { Config.setStoragePath(canonicalStorageRoot(app)) }
        organicMaps = engine
        return try {
            engine.init {
                initialized = true
                initializing = false
                syncStorageAndReloadOnMain(app, forceReload = false)
                val callbacks = readyCallbacks.take()
                if (callbacks.isNotEmpty()) installPlacePageListener()
                callbacks.forEach { it() }
            }
            true
        } catch (error: Exception) {
            initializing = false
            readyCallbacks.invalidate()
            Log.e("ComapsMapEngine", "native map initialization failed", error)
            false
        }
    }

    override fun newMapView(context: Context): View {
        check(initialized) { "Map surface requested before native initialization" }
        renderingReady = false
        return MapViewHelper.create(context, checkNotNull(organicMaps).locationHelper, {
            lastViewport?.let { vp ->
                if (initialized) {
                    Framework.nativeZoomToPoint(vp.latitude, vp.longitude, vp.zoom, false)
                    Framework.nativeSetViewportCenter(vp.latitude, vp.longitude, vp.zoom)
                    Framework.nativeSetSearchViewport(vp.latitude, vp.longitude, vp.zoom)
                }
            }
        }, { renderingReady = true })
    }

    override fun isRenderingReady(): Boolean = renderingReady


    override fun setViewport(latitude: Double, longitude: Double, zoom: Int) {
        val clampedZoom = zoom.coerceIn(1, 20)
        lastViewport = MapCamera(latitude, longitude, clampedZoom)
        if (initialized) {
            Framework.nativeZoomToPoint(latitude, longitude, clampedZoom, false)
            Framework.nativeSetViewportCenter(latitude, longitude, clampedZoom)
            Framework.nativeSetSearchViewport(latitude, longitude, clampedZoom)
        }
    }

    private fun liveScreenCenter(): MapCenter? {
        if (!initialized) return null
        val screenCenter = try {
            Framework.nativeGetScreenRectCenter()
        } catch (_: Exception) {
            null
        }
        if (
            screenCenter != null &&
            screenCenter.size == 2 &&
            !screenCenter[0].isNaN() &&
            !screenCenter[1].isNaN() &&
            (kotlin.math.abs(screenCenter[0]) > 1e-6 || kotlin.math.abs(screenCenter[1]) > 1e-6)
        ) {
            return MapCenter(screenCenter[0], screenCenter[1])
        }
        return null
    }

    override fun currentCenter(): MapCenter? {
        if (!initialized) return null
        liveScreenCenter()?.let { return it }
        val parsed = try {
            Framework.nativeGetParsedCenterLatLon()
        } catch (_: Exception) {
            null
        }
        if (
            parsed != null &&
            parsed.size == 2 &&
            !parsed[0].isNaN() &&
            !parsed[1].isNaN() &&
            (kotlin.math.abs(parsed[0]) > 1e-6 || kotlin.math.abs(parsed[1]) > 1e-6)
        ) {
            return MapCenter(parsed[0], parsed[1])
        }
        return lastViewport?.let { MapCenter(it.latitude, it.longitude) }
    }

    override fun currentCamera(): MapCamera? {
        if (!initialized) return null
        val live = liveScreenCenter()
        if (live != null) {
            val scale = try {
                Framework.nativeGetDrawScale()
            } catch (_: Exception) {
                0
            }
            val zoom = if (scale in 1..20) scale else (lastViewport?.zoom ?: 13)
            return MapCamera(live.latitude, live.longitude, zoom).also { lastViewport = it }
        }
        return lastViewport ?: currentCenter()?.let { MapCamera(it.latitude, it.longitude, 13) }
    }

    override fun recenter(): Boolean {
        if (!initialized) return false
        val position = organicMaps?.locationHelper?.myPosition ?: return false
        setViewport(position.lat, position.lon, 15)
        return true
    }

    override fun isInitialized(): Boolean = initialized

    override fun suspendMapReads(): Boolean {
        if (guiding) return false
        if (!initialized) return true
        check(Looper.myLooper() == Looper.getMainLooper()) { "Native map storage requires Main" }
        val empty = File(checkNotNull(appContext).filesDir, "map-publication-idle").apply { mkdirs() }
        Framework.nativeChangeWritableDir(empty.canonicalPath + File.separator)
        return true
    }

    override fun reloadMaps(): Boolean {
        if (guiding) return false
        if (!initialized) return false
        val app = appContext ?: return false
        if (Looper.myLooper() == Looper.getMainLooper()) {
            return syncStorageAndReloadOnMain(app, forceReload = true)
        }
        val latch = java.util.concurrent.CountDownLatch(1)
        var ok = false
        Handler(Looper.getMainLooper()).post {
            try {
                ok = syncStorageAndReloadOnMain(app, forceReload = true)
            } finally {
                latch.countDown()
            }
        }
        latch.await(5, java.util.concurrent.TimeUnit.SECONDS)
        return ok
    }

    override fun writableDir(): String? {
        if (!initialized) return null
        return try {
            Framework.nativeGetWritableDir()
        } catch (_: Exception) {
            null
        }
    }

    private fun <T> callOnMain(block: () -> T): T {
        if (Looper.myLooper() == Looper.getMainLooper()) return block()
        val task = java.util.concurrent.FutureTask(java.util.concurrent.Callable { block() })
        Handler(Looper.getMainLooper()).post(task)
        return try {
            task.get(5, java.util.concurrent.TimeUnit.SECONDS)
        } catch (error: Exception) {
            task.cancel(false)
            throw error
        }
    }

    override fun isDownloadedMapAtScreenCenter(): Boolean =
        initialized && callOnMain { Framework.nativeIsDownloadedMapAtScreenCenter() }

    override fun downloadedMapsCount(): Int =
        if (!initialized) 0 else callOnMain {
            app.organicmaps.sdk.downloader.MapManager.nativeGetDownloadedCount()
        }

    private fun canonicalStorageRoot(app: Context): String {
        val canonical = runCatching { app.filesDir.canonicalFile }.getOrElse { app.filesDir.absoluteFile }
        val path = canonical.path
        return if (path.endsWith(File.separator)) path else "$path${File.separator}"
    }

    private fun syncStorageAndReloadOnMain(app: Context, forceReload: Boolean): Boolean {
        if (!initialized) return false
        return try {
            val desiredRoot = canonicalStorageRoot(app)
            Config.setStoragePath(desiredRoot)
            val currentDir = Framework.nativeGetWritableDir().orEmpty()
            val currentCanonical = runCatching { File(currentDir).canonicalFile.path }
                .getOrElse { File(currentDir).absolutePath }
            val desiredCanonical = desiredRoot.trimEnd(File.separatorChar)
            if (currentCanonical != desiredCanonical || forceReload) {
                // This JNI call removes/re-registers ALL local maps. ReloadWorldMaps
                // only reloads world overview files and misses regional packages.
                Framework.nativeChangeWritableDir(desiredRoot)
            }
            lastViewport?.let { vp ->
                Framework.nativeSetSearchViewport(vp.latitude, vp.longitude, vp.zoom)
            }
            true
        } catch (error: Exception) {
            Log.e("ComapsMapEngine", "failed to synchronize storage root and reload maps", error)
            false
        }
    }

    override fun search(
        context: Context,
        query: String,
        onResult: (List<MapSearchResult>) -> Unit,
    ): Boolean {
        if (!initialized) return false
        cancelSearch()
        val timestamp = ++searchGeneration
        val collected = mutableListOf<MapSearchResult>()
        val listener = object : SearchListener {
            override fun onResultsUpdate(results: Array<SearchResult>, resultTimestamp: Long) {
                if (searchListener !== this || resultTimestamp != timestamp) return
                // SDK updates are complete snapshots, not append-only pages.
                collected.clear()
                for (result in results) {
                    if (result.lat == 0.0 && result.lon == 0.0) continue
                    collected += MapSearchResult(result.name, result.lat, result.lon)
                }
            }

            override fun onResultsEnd(resultTimestamp: Long) {
                if (searchListener !== this || resultTimestamp != timestamp) return
                SearchEngine.INSTANCE.removeListener(this)
                if (searchListener === this) searchListener = null
                onResult(collected.toList())
            }
        }
        searchListener = listener
        SearchEngine.INSTANCE.addListener(listener)
        val center = currentCenter()
        val zoom = lastViewport?.zoom ?: 13
        if (center != null) {
            runCatching { Framework.nativeSetSearchViewport(center.latitude, center.longitude, zoom) }
        }
        return SearchEngine.INSTANCE.search(
            context,
            query,
            false,
            timestamp,
            center != null,
            center?.latitude ?: 0.0,
            center?.longitude ?: 0.0,
        )
    }

    override fun buildRoute(request: NativeRouteRequest, onResult: (NativeRouteResult) -> Unit): Boolean {
        val accepted = routePlanner.buildRoute(request) { result ->
            if (result is NativeRouteResult.Ready) {
                lastReadyRequest = request
            }
            onResult(result)
        }
        return accepted
    }

    override fun startGuidance(request: NativeRouteRequest): Boolean {
        check(Looper.myLooper() == Looper.getMainLooper()) { "Guidance requires Main" }
        request.validate()
        if (!initialized) return false
        if (guiding) return false
        val ready = lastReadyRequest ?: return false
        if (!GuidancePolicy.sameRoute(ready, request)) return false
        return try {
            // Never record a trace while guiding; the service owns no saved track.
            runCatching {
                if (TrackRecorder.nativeIsTrackRecordingEnabled()) {
                    TrackRecorder.nativeStopTrackRecording()
                }
            }
            Framework.nativeFollowRoute()
            runCatching {
                if (TrackRecorder.nativeIsTrackRecordingEnabled()) {
                    TrackRecorder.nativeStopTrackRecording()
                }
            }
            val locale = GuidancePolicy.nativeLocaleFor(request.language) ?: "en"
            TtsPlayer.configureNativeNotifications(locale, true)
            guidanceRequest = request
            guiding = true
            guidanceGeneration++
            rerouteStartNanos = null
            lastProgress = null
            true
        } catch (_: Exception) {
            guiding = false
            guidanceRequest = null
            false
        }
    }

    override fun feedGuidanceFix(fix: NavigationFix, nowNanos: Long): GuidanceFeed? {
        check(Looper.myLooper() == Looper.getMainLooper()) { "Guidance requires Main" }
        if (!guiding) return null
        if (!NavigationFixPolicy.accepts(fix, nowNanos)) {
            return GuidanceFeed(progress = lastProgress, notifications = emptyList(), rerouteRecoverable = false)
        }
        return try {
            runCatching {
                if (TrackRecorder.nativeIsTrackRecordingEnabled()) {
                    TrackRecorder.nativeStopTrackRecording()
                }
            }
            NavigationFixBridge.forward(
                fix.timeMillis,
                fix.latitude,
                fix.longitude,
                fix.accuracyMeters,
                fix.altitudeMeters,
                fix.speedMetersPerSecond,
                fix.bearingDegrees,
            )
            val info = Framework.nativeGetRouteFollowingInfo()
            val notifications = Framework.nativeGenerateNotifications(true).orEmpty().toList()
            val progress = info?.toNavigationProgress(notifications)
            val state = info?.routingSessionState
            val offRoute = state == RoutingInfo.RoutingSessionState.OffRoute ||
                state == RoutingInfo.RoutingSessionState.RouteRebuilding ||
                state == RoutingInfo.RoutingSessionState.RouteNeedsRebuild
            if (offRoute) {
                val started = rerouteStartNanos ?: nowNanos.also { rerouteStartNanos = it }
                if (GuidancePolicy.isRerouteExpired(started, nowNanos)) {
                    return GuidanceFeed(progress = progress ?: lastProgress, notifications = notifications, rerouteRecoverable = true)
                }
            } else {
                rerouteStartNanos = null
            }
            if (progress != null) lastProgress = progress
            GuidanceFeed(progress = progress ?: lastProgress, notifications = notifications, rerouteRecoverable = false)
        } catch (_: Exception) {
            GuidanceFeed(progress = lastProgress, notifications = emptyList(), rerouteRecoverable = false)
        }
    }

    override fun guidanceProgress(): NavigationProgress? {
        if (!guiding) return null
        if (Looper.myLooper() != Looper.getMainLooper()) return lastProgress
        return try {
            Framework.nativeGetRouteFollowingInfo()?.toNavigationProgress(emptyList())?.also { lastProgress = it }
                ?: lastProgress
        } catch (_: Exception) {
            lastProgress
        }
    }

    override fun stopGuidance() {
        guidanceGeneration++
        guiding = false
        guidanceRequest = null
        rerouteStartNanos = null
        lastProgress = null
        if (!initialized) return
        if (Looper.myLooper() == Looper.getMainLooper()) {
            runCatching { TtsPlayer.configureNativeNotifications("en", false) }
            runCatching { Framework.nativeDisableFollowing() }
            runCatching { Framework.nativeCloseRouting() }
            runCatching { Framework.nativeRemoveRoutePoints() }
        } else {
            val latch = java.util.concurrent.CountDownLatch(1)
            Handler(Looper.getMainLooper()).post {
                try {
                    runCatching { TtsPlayer.configureNativeNotifications("en", false) }
                    runCatching { Framework.nativeDisableFollowing() }
                    runCatching { Framework.nativeCloseRouting() }
                    runCatching { Framework.nativeRemoveRoutePoints() }
                } finally {
                    latch.countDown()
                }
            }
            runCatching { latch.await(5, java.util.concurrent.TimeUnit.SECONDS) }
        }
    }

    override fun isGuiding(): Boolean = guiding

    private fun RoutingInfo.toNavigationProgress(notifications: List<String>): NavigationProgress {
        val remaining = try { distToTarget.meters() } catch (_: Exception) { 0.0 }
        val turn = try { distToTurn.meters() } catch (_: Exception) { 0.0 }
        return NavigationProgress(
            remainingMeters = remaining,
            seconds = totalTimeInSeconds,
            turnMeters = turn,
            completionPercent = completionPercent,
            direction = try { carDirection.name } catch (_: Exception) { "UNKNOWN" },
            nextStreet = nextStreet.orEmpty(),
            nativeState = try { routingSessionState.name } catch (_: Exception) { "UNKNOWN" },
            notifications = notifications,
        )
    }

    override fun buildCarRoute(startLatitude: Double, startLongitude: Double, endLatitude: Double, endLongitude: Double, onResult: (MapRouteInfo?) -> Unit): Boolean =
        buildRoute(com.anpfuel.domain.routing.NativeRouteRequest(listOf(
            com.anpfuel.domain.routing.RoutePoint(startLatitude,startLongitude,"Start"),
            com.anpfuel.domain.routing.RoutePoint(endLatitude,endLongitude,"Destination"),
        ))) { result ->
            onResult((result as? com.anpfuel.domain.routing.NativeRouteResult.Ready)?.preview?.let { MapRouteInfo(it.distanceMeters,"Meters",it.seconds) })
        }

    override fun setOverlayMarkers(markers: List<MapOverlayMarker>): Boolean {
        if (!initialized || markers.isEmpty()) return false
        val context = appContext ?: return false
        val manager = BookmarkManager.INSTANCE
        return try {
            clearOverlayListeners()
            val generation = ++overlayGeneration
            val preparation = prepareOverlay(markers, generation = generation)
            if (preparation.clusters.isEmpty()) {
                Log.w("ComapsMapEngine", "overlay rejected=${preparation.rejected} duplicates=${preparation.duplicates}")
                return false
            }
            val categories = manager.getCategories()
            val oldCategories = categories.filter { isOwnedOverlayCategory(it.name) }.map { it.id }
            val oldIds = categories.map { it.id }.toSet()
            val file = File(context.cacheDir, "abastevo-overlay-$generation.kml")
            file.writeText(buildOverlayKml(preparation.clusters, overlayCategoryName(generation)))
            var settled = false
            fun settle(category: BookmarkCategory?) {
                if (settled) return
                settled = true
                clearOverlayListeners()
                if (category == null) {
                    Log.w("ComapsMapEngine", "overlay generation $generation not committed; previous category retained")
                    return
                }
                Handler(Looper.getMainLooper()).postDelayed({
                    if (!initialized || overlayGeneration != generation) return@postDelayed
                    try {
                        manager.showBookmarkCategoryOnMap(category.id)
                        oldCategories.filter { it != category.id }.forEach(manager::deleteCategory)
                        overlayCategory = category.id
                        overlayClusters = preparation.clusters
                        overlayMarkers = markers
                        Log.i(
                            "ComapsMapEngine",
                            "overlay committed generation=$generation category=${category.id} markers=${markers.size} " +
                                "clusters=${preparation.clusters.size} rejected=${preparation.rejected} " +
                                "duplicates=${preparation.duplicates}",
                        )
                    } catch (error: Exception) {
                        Log.e("ComapsMapEngine", "overlay commit failed; previous category retained", error)
                    }
                }, 100L)
            }
            fun newCategory(): BookmarkCategory? =
                manager.getCategories().lastOrNull { it.id !in oldIds && isOverlayCategoryGeneration(it.name, generation) }

            val loadingListener = object : BookmarkManager.BookmarksLoadingListener {
                override fun onBookmarksFileImportSuccessful() {
                    newCategory()?.let(::settle)
                }

                override fun onBookmarksFileImportFailed() {
                    settle(null)
                }

                override fun onBookmarksFileUnsupported(uri: android.net.Uri) {
                    settle(null)
                }

                override fun onBookmarksLoadingFinished() {
                    newCategory()?.let(::settle)
                }
            }
            val categoriesListener = DataChangedListener {
                newCategory()?.let(::settle)
            }
            overlayLoadingListener = loadingListener
            overlayCategoriesListener = categoriesListener
            manager.addLoadingListener(loadingListener)
            manager.addCategoriesUpdatesListener(categoriesListener)
            manager.loadBookmarksFile(file.absolutePath, true)
            Log.i(
                "ComapsMapEngine",
                "overlay generation=$generation markers=${markers.size} clusters=${preparation.clusters.size} pending",
            )
            true
        } catch (error: Exception) {
            clearOverlayListeners()
            Log.e("ComapsMapEngine", "overlay generation failed", error)
            false
        }
    }

    override fun overlayClusters(): List<MapOverlayCluster> = overlayClusters

    override fun setOnOverlaySelected(listener: (MapOverlaySelection) -> Unit) {
        overlayListener = listener
    }

    override fun cancelSearch() {
        searchListener?.let { SearchEngine.INSTANCE.removeListener(it) }
        searchListener = null
        if (initialized) SearchEngine.INSTANCE.cancel()
    }

    override fun cancelRoute() = routePlanner.cancelRoute()

    @Synchronized
    override fun release() {
        readyCallbacks.invalidate()
        organicMaps?.locationHelper?.stop()
        overlayGeneration++
        cancelSearch()
        cancelRoute()
        // Full teardown owns guidance too; ViewModel disposal must avoid this while guiding.
        runCatching { stopGuidance() }
        lastReadyRequest = null
        clearOverlayListeners()
        placePageListener?.let { Framework.nativeRemovePlacePageActivationListener(it) }
        placePageListener = null
        overlayListener = null
        overlayClusters = emptyList()
        overlayMarkers = emptyList()
    }

    private fun clearOverlayListeners() {
        overlayLoadingListener?.let { BookmarkManager.INSTANCE.removeLoadingListener(it) }
        overlayCategoriesListener?.let { BookmarkManager.INSTANCE.removeCategoriesUpdatesListener(it) }
        overlayLoadingListener = null
        overlayCategoriesListener = null
    }

    private fun installPlacePageListener() {
        if (placePageListener != null) return
        val listener = object : PlacePageActivationListener {
            override fun onPlacePageActivated(data: PlacePageData) {
                val objectData = data as? MapObject ?: return
                Log.i("ComapsMapEngine", "place page activated title=${objectData.title}")
                overlaySelectionFor(
                    objectData.lat,
                    objectData.lon,
                    overlayMarkers,
                    overlayClusters,
                )?.let { overlayListener?.invoke(it) }
            }

            override fun onPlacePageDeactivated() = Unit
            override fun onSwitchFullScreenMode() = Unit
        }
        placePageListener = listener
        Framework.nativePlacePageActivationListener(listener)
    }
}

/**
 * Identifies an Abastevo overlay object by its exact rendered point, never by
 * a technical label. The native place-page title is user-facing station text;
 * canonical UUIDs remain internal. Ambiguous/nearby points fail closed.
 */
internal fun overlaySelectionFor(
    latitude: Double,
    longitude: Double,
    markers: List<MapOverlayMarker>,
    clusters: List<MapOverlayCluster>,
): MapOverlaySelection? {
    fun matches(lat: Double, lon: Double): Boolean =
        kotlin.math.abs(lat - latitude) < 1e-6 && kotlin.math.abs(lon - longitude) < 1e-6

    val cluster = clusters.firstOrNull { matches(it.latitude, it.longitude) }
    if (cluster != null) {
        return if (cluster.markers.size == 1) {
            MapOverlaySelection.Station(cluster.markers.single().uuid)
        } else {
            MapOverlaySelection.Cluster(cluster.markers)
        }
    }
    return markers.firstOrNull { matches(it.latitude, it.longitude) }
        ?.let { MapOverlaySelection.Station(it.uuid) }
}
