package com.anpfuel.data.map

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.domain.map.MapRegionIndex
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/** Resolves the CoMaps Brazil leaf id (e.g. `Brazil_Mato Grosso`) of a point. */
interface MapRegionResolver {
    suspend fun regionFor(latitude: Double, longitude: Double): String?
}

/**
 * Offline lookup over the bundled simplified borders
 * (`assets/map_regions_brazil.txt`), parsed once on first use off the main
 * thread. Null outside Brazil.
 */
@Singleton
class AssetMapRegionResolver @Inject constructor(
    @ApplicationContext private val context: Context,
) : MapRegionResolver {
    private val lock = Mutex()
    @Volatile private var index: MapRegionIndex? = null

    override suspend fun regionFor(latitude: Double, longitude: Double): String? = withContext(Dispatchers.Default) {
        val loaded = index ?: lock.withLock {
            index ?: withContext(Dispatchers.IO) {
                context.assets.open(ASSET_FILE).bufferedReader().use { MapRegionIndex.parse(it.readText()) }
            }.also { index = it }
        }
        loaded.regionFor(latitude, longitude)
    }

    companion object {
        const val ASSET_FILE = "map_regions_brazil.txt"
    }
}

/** Per-region "Agora não" for the map download suggestion. */
interface MapRegionDismissalStore {
    fun isDismissed(regionSlug: String): Boolean
    fun dismiss(regionSlug: String)
}

@Singleton
class SharedPreferencesMapRegionDismissalStore @Inject constructor(
    @ApplicationContext context: Context,
) : MapRegionDismissalStore {
    private val prefs: SharedPreferences = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun isDismissed(regionSlug: String): Boolean =
        regionSlug in prefs.getStringSet(KEY_DISMISSED, emptySet()).orEmpty()

    override fun dismiss(regionSlug: String) {
        if (!isMapRegionSlug(regionSlug)) return
        val next = prefs.getStringSet(KEY_DISMISSED, emptySet()).orEmpty().toMutableSet().apply { add(regionSlug) }
        prefs.edit().putStringSet(KEY_DISMISSED, next).apply()
    }

    companion object {
        const val PREFS_NAME = "map_regions"
        const val KEY_DISMISSED = "dismissed_region_suggestions"
    }
}
