package com.anpfuel.maps.engine

import android.content.Context

interface MapCameraStore {
    fun load(): MapCamera?
    fun save(camera: MapCamera)
}

class SharedPreferencesMapCameraStore(context: Context) : MapCameraStore {
    private val prefs = context.applicationContext.getSharedPreferences("abastevo_map_camera", Context.MODE_PRIVATE)
    override fun load(): MapCamera? {
        if (!prefs.contains("lat") || !prefs.contains("lon") || !prefs.contains("zoom")) return null
        val lat = prefs.getFloat("lat", Float.NaN).toDouble()
        val lon = prefs.getFloat("lon", Float.NaN).toDouble()
        val zoom = prefs.getInt("zoom", -1)
        if (lat.isNaN() || lon.isNaN() || zoom !in 0..22) return null
        return MapCamera(lat, lon, zoom)
    }
    override fun save(camera: MapCamera) {
        prefs.edit().putFloat("lat", camera.latitude.toFloat())
            .putFloat("lon", camera.longitude.toFloat())
            .putInt("zoom", camera.zoom).apply()
    }
}
