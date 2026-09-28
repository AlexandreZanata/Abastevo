package com.anpfuel.data.remote

/**
 * Compares discovered catalog size with visible week blocks on the ANP listing page (Phase 12.2.5).
 */
internal data class WeekCatalogComparison(
    val visibleWeekHeaderCount: Int,
    val completeVisibleWeekBlockCount: Int,
    val catalogEntryCount: Int,
) {
    fun requireValidDiscovery() {
        check(catalogEntryCount >= 50) {
            "Gate 12.2 requires at least 50 discoverable weeks; got $catalogEntryCount"
        }
        check(catalogEntryCount == completeVisibleWeekBlockCount) {
            "Catalog entry count ($catalogEntryCount) does not match complete visible week blocks " +
                "($completeVisibleWeekBlockCount)"
        }
        check(catalogEntryCount <= visibleWeekHeaderCount) {
            "Catalog entries ($catalogEntryCount) exceed visible week headers ($visibleWeekHeaderCount)"
        }
    }
}

internal object AnpListingWeekCatalogComparator {

    fun compare(html: String, baseUrl: String = AnpEndpoints.LISTING_PAGE_URL): WeekCatalogComparison {
        val catalog = AnpListingWeekCatalogParser.parse(html, baseUrl)
        return WeekCatalogComparison(
            visibleWeekHeaderCount = AnpListingVisibleWeekParser.countVisibleWeekHeaders(html, baseUrl),
            completeVisibleWeekBlockCount = AnpListingVisibleWeekParser.countCompleteVisibleWeekBlocks(
                html,
                baseUrl,
            ),
            catalogEntryCount = catalog.size,
        )
    }
}
