package com.anpfuel.maps.engine

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.View
import app.organicmaps.sdk.Framework
import app.organicmaps.sdk.MapView
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
import app.organicmaps.sdk.routing.RoutingController
import app.organicmaps.sdk.search.SearchEngine
import app.organicmaps.sdk.search.SearchListener
import app.organicmaps.sdk.search.SearchResult
import app.organicmaps.sdk.util.ConnectionState
import app.organicmaps.sdk.widget.placepage.PlacePageData
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
    private var appContext: Context? = null
    private var overlayListener: ((MapOverlaySelection) -> Unit)? = null
    private var placePageListener: PlacePageActivationListener? = null
    private var searchListener: SearchListener? = null
    private var routingContainer: RoutingController.Container? = null
    private var overlayCategory = -1L
    private var overlayClusters: List<MapOverlayCluster> = emptyList()
    private var overlayClusterMembers: Map<String, List<MapOverlayMarker>> = emptyMap()
    private var overlayLoadingListener: BookmarkManager.BookmarksLoadingListener? = null
    private var overlayCategoriesListener: DataChangedListener? = null
    private var overlayGeneration = 0L
    private val pendingReady = mutableListOf<() -> Unit>()

    override fun initialize(context: Context, onReady: () -> Unit): Boolean {
        if (initialized) {
            installPlacePageListener()
            onReady()
            return true
        }
        if (initializing) {
            pendingReady += onReady
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
        organicMaps = engine
        return try {
            engine.init {
                initialized = true
                initializing = false
                installPlacePageListener()
                val callbacks = pendingReady.toList()
                pendingReady.clear()
                onReady()
                callbacks.forEach { it() }
            }
            true
        } catch (error: Exception) {
            initializing = false
            pendingReady.clear()
            Log.e("ComapsMapEngine", "native map initialization failed", error)
            false
        }
    }

    override fun newMapView(context: Context): View {
        val mapView = MapView(context)
        organicMaps?.let { MapViewHelper.setLocationHelper(mapView, it.locationHelper) }
        return mapView
    }

    override fun setViewport(latitude: Double, longitude: Double, zoom: Int) {
        if (initialized) {
            Framework.nativeSetViewportCenter(latitude, longitude, zoom)
        }
    }

    override fun currentCenter(): MapCenter? {
        if (!initialized) return null
        val center = Framework.nativeGetParsedCenterLatLon() ?: return null
        if (center.size != 2 || center[0].isNaN() || center[1].isNaN()) return null
        return MapCenter(center[0], center[1])
    }

    override fun recenter(): Boolean {
        if (!initialized) return false
        val position = organicMaps?.locationHelper?.myPosition ?: return false
        Framework.nativeSetViewportCenter(position.lat, position.lon, 15)
        return true
    }

    override fun isInitialized(): Boolean = initialized

    override fun search(
        context: Context,
        query: String,
        onResult: (List<MapSearchResult>) -> Unit,
    ): Boolean {
        if (!initialized) return false
        cancelSearch()
        val timestamp = System.currentTimeMillis()
        val collected = mutableListOf<MapSearchResult>()
        val listener = object : SearchListener {
            override fun onResultsUpdate(results: Array<SearchResult>, resultTimestamp: Long) {
                if (resultTimestamp != timestamp) return
                for (result in results) {
                    if (result.lat == 0.0 && result.lon == 0.0) continue
                    collected += MapSearchResult(result.name, result.lat, result.lon)
                }
            }

            override fun onResultsEnd(resultTimestamp: Long) {
                if (resultTimestamp != timestamp) return
                SearchEngine.INSTANCE.removeListener(this)
                if (searchListener === this) searchListener = null
                onResult(collected.toList())
            }
        }
        searchListener = listener
        SearchEngine.INSTANCE.addListener(listener)
        return SearchEngine.INSTANCE.search(context, query, false, timestamp, false, 0.0, 0.0)
    }

    override fun buildCarRoute(
        startLatitude: Double,
        startLongitude: Double,
        endLatitude: Double,
        endLongitude: Double,
        onResult: (MapRouteInfo?) -> Unit,
    ): Boolean {
        if (!initialized) return false
        cancelRoute()
        val controller = RoutingController.get()
        val container = object : RoutingController.Container {
            override fun showRoutePlan(show: Boolean, completionListener: Runnable?) {
                completionListener?.run()
            }

            override fun onBuiltRoute() {
                if (routingContainer !== this) return
                val info = Framework.nativeGetRouteFollowingInfo()
                if (info == null) {
                    onResult(null)
                } else {
                    onResult(MapRouteInfo(info.distToTarget.mDistance, info.distToTarget.mUnits.name, info.totalTimeInSeconds))
                }
            }

            override fun onCommonBuildError(lastResultCode: Int, lastMissingMaps: Array<String>) {
                if (routingContainer === this) routingContainer = null
                onResult(null)
            }

            override fun onDrivingOptionsBuildError() {
                if (routingContainer === this) routingContainer = null
                onResult(null)
            }
        }
        routingContainer = container
        controller.attach(container)
        val start = MapObject.createMapObject(FeatureId.EMPTY, MapObject.POI, "Start", "", startLatitude, startLongitude)
        val end = MapObject.createMapObject(FeatureId.EMPTY, MapObject.POI, "Destination", "", endLatitude, endLongitude)
        controller.prepare(start, end)
        return true
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
            val oldCategory = overlayCategory
            val oldIds = manager.getCategories().map { it.id }.toSet()
            val file = File(context.cacheDir, "abastevo-overlay-$generation.kml")
            file.writeText(buildOverlayKml(preparation.clusters))
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
                        if (oldCategory >= 0 && oldCategory != category.id) {
                            manager.deleteCategory(oldCategory)
                        }
                        overlayCategory = category.id
                        overlayClusters = preparation.clusters
                        overlayClusterMembers = preparation.clusters
                            .filter { it.isCluster }
                            .associate { it.id to it.markers }
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
                manager.getCategories().lastOrNull { it.id !in oldIds && it.name.startsWith("Abastevo") }

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

    override fun cancelRoute() {
        routingContainer = null
        if (initialized) {
            RoutingController.get().detach()
            Framework.nativeCloseRouting()
        }
    }

    override fun release() {
        cancelSearch()
        cancelRoute()
        clearOverlayListeners()
        placePageListener?.let { Framework.nativeRemovePlacePageActivationListener(it) }
        placePageListener = null
        overlayListener = null
        overlayClusters = emptyList()
        overlayClusterMembers = emptyMap()
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
                val title = (data as? MapObject)?.title ?: return
                Log.i("ComapsMapEngine", "place page activated title=$title")
                overlaySelectionFor(title, overlayClusterMembers)?.let { overlayListener?.invoke(it) }
            }

            override fun onPlacePageDeactivated() = Unit
            override fun onSwitchFullScreenMode() = Unit
        }
        placePageListener = listener
        Framework.nativePlacePageActivationListener(listener)
    }
}

internal fun overlaySelectionFor(
    title: String,
    clusterMembers: Map<String, List<MapOverlayMarker>>,
): MapOverlaySelection? = when {
    title.startsWith("ABASTEVO_CLUSTER:") ->
        clusterMembers[title.removePrefix("ABASTEVO_CLUSTER:")]?.let { MapOverlaySelection.Cluster(it) }

    title.startsWith("ABASTEVO:") -> MapOverlaySelection.Station(title.removePrefix("ABASTEVO:"))
    else -> null
}
