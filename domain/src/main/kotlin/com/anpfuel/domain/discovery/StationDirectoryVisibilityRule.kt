package com.anpfuel.domain.discovery

/** Selects supplemental directory rows for the mixed price/directory screen. */
object StationDirectoryVisibilityRule {
    fun additionalStations(
        directory: List<ServerStation>,
        visiblePriceCnpjs: Collection<String>,
    ): List<ServerStation> {
        val visible = visiblePriceCnpjs.toHashSet()
        return directory.filter { station -> station.cnpjNormalized == null || station.cnpjNormalized !in visible }
    }
}
