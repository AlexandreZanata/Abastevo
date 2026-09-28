package com.anpfuel.app.navigation

import android.content.Context
import android.content.Intent
import android.net.Uri

sealed interface MapNavigationResult {
    data object Launched : MapNavigationResult
    data object NoAppFound : MapNavigationResult
}

/**
 * UC-013 — Opens Google Maps with a normalized station search query.
 */
object MapAppChooser {

    private const val GOOGLE_MAPS_PACKAGE = "com.google.android.apps.maps"

    fun openNavigation(context: Context, query: String): MapNavigationResult {
        val encodedQuery = Uri.encode(query)

        val mapsAppIntent = Intent(
            Intent.ACTION_VIEW,
            Uri.parse("https://www.google.com/maps/search/?api=1&query=$encodedQuery"),
        ).apply {
            setPackage(GOOGLE_MAPS_PACKAGE)
        }
        if (mapsAppIntent.resolveActivity(context.packageManager) != null) {
            context.startActivity(mapsAppIntent)
            return MapNavigationResult.Launched
        }

        val fallbackIntent = Intent(
            Intent.ACTION_VIEW,
            Uri.parse("https://www.google.com/maps/search/?api=1&query=$encodedQuery"),
        )
        if (fallbackIntent.resolveActivity(context.packageManager) != null) {
            context.startActivity(fallbackIntent)
            return MapNavigationResult.Launched
        }

        return MapNavigationResult.NoAppFound
    }
}
