package com.anpfuel.maps.engine

import android.content.Context
import android.util.Log
import android.view.View
import app.organicmaps.sdk.Framework
import app.organicmaps.sdk.MapView
import app.organicmaps.sdk.MapViewHelper
import app.organicmaps.sdk.OrganicMaps
import app.organicmaps.sdk.PlacePageActivationListener
import app.organicmaps.sdk.bookmarks.data.BookmarkManager
import app.organicmaps.sdk.bookmarks.data.FeatureId
import app.organicmaps.sdk.bookmarks.data.Icon
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
    private var selectedListener: ((String) -> Unit)? = null
    private var placePageListener: PlacePageActivationListener? = null
    private var searchListener: SearchListener? = null
    private var routingContainer: RoutingController.Container? = null
    private var overlayCategory = -1L
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

    override fun addOverlayMarker(uuid: String, latitude: Double, longitude: Double, title: String): Boolean {
        if (!initialized) return false
        val context = appContext ?: return false
        val manager = BookmarkManager.INSTANCE
        val file = File(context.cacheDir, "abastevo-overlay.kml")
        val kml = """
            <?xml version="1.0" encoding="UTF-8"?>
            <kml xmlns="http://www.opengis.net/kml/2.2">
              <Document>
                <name>Abastevo</name>
                <Placemark>
                  <name>ABASTEVO:$uuid</name>
                  <description>${title.replace("<", " ").replace(">", " ")}</description>
                  <Point><coordinates>$longitude,$latitude</coordinates></Point>
                </Placemark>
              </Document>
            </kml>
        """.trimIndent()
        return try {
            file.writeText(kml)
            manager.loadBookmarksFile(file.absolutePath, true)
            val category = manager.getCategories().lastOrNull()
            if (category != null) {
                overlayCategory = category.id
                manager.showBookmarkCategoryOnMap(category.id)
            }
            Log.i("ComapsMapEngine", "overlay marker category=${category?.id} uuid=$uuid")
            true
        } catch (error: Exception) {
            Log.e("ComapsMapEngine", "overlay marker failed for $uuid", error)
            false
        }
    }

    override fun setOnMarkerSelected(listener: (String) -> Unit) {
        selectedListener = listener
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
        placePageListener?.let { Framework.nativeRemovePlacePageActivationListener(it) }
        placePageListener = null
        selectedListener = null
    }

    private fun installPlacePageListener() {
        if (placePageListener != null) return
        val listener = object : PlacePageActivationListener {
            override fun onPlacePageActivated(data: PlacePageData) {
                val title = (data as? MapObject)?.title ?: return
                if (title.startsWith("ABASTEVO:")) {
                    selectedListener?.invoke(title.removePrefix("ABASTEVO:"))
                }
            }

            override fun onPlacePageDeactivated() = Unit
            override fun onSwitchFullScreenMode() = Unit
        }
        placePageListener = listener
        Framework.nativePlacePageActivationListener(listener)
    }
}
