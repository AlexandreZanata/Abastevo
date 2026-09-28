package com.anpfuel.data.remote

import java.io.File

internal object AnpFixtureFiles {

    fun readListingHtml(): String = resolve(LISTING_FIXTURE).readText()

    fun readFullListingHtml(): String = resolve(FULL_LISTING_FIXTURE).readText()

    fun resolve(fileName: String): File {
        val candidates = listOf(
            File("fixtures/$fileName"),
            File("data/fixtures/$fileName"),
            File("../fixtures/$fileName"),
            File("../data/fixtures/$fileName"),
        )
        return candidates.firstOrNull { it.exists() }
            ?: error("Fixture file not found: $fileName (cwd=${File(".").absolutePath})")
    }

    const val LISTING_FIXTURE = "anp-listing.html"
    const val FULL_LISTING_FIXTURE = "anp-listing-full.html"
}
