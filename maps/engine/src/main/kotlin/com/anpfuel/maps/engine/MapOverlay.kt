package com.anpfuel.maps.engine

import java.util.Locale
import kotlin.math.cos
import kotlin.math.sqrt

enum class MapStationLocationQuality {
    UNKNOWN,
    CITY_CENTROID,
    REVIEWED,
}

enum class MapStationType {
    FUEL,
    CHARGING,
    FUEL_AND_CHARGING,
}

data class MapOverlayMarker(
    val uuid: String,
    val latitude: Double,
    val longitude: Double,
    val title: String,
    val fuel: Boolean = true,
    val charging: Boolean = false,
    val locationQuality: MapStationLocationQuality = MapStationLocationQuality.UNKNOWN,
    val subtitle: String = "",
)

sealed interface MapOverlaySelection {
    data class Station(val uuid: String) : MapOverlaySelection
    data class Cluster(val members: List<MapOverlayMarker>) : MapOverlaySelection
}

data class MapOverlayCluster(
    val id: String,
    val latitude: Double,
    val longitude: Double,
    val markers: List<MapOverlayMarker>,
) {
    val count: Int get() = markers.size
    val isCluster: Boolean get() = markers.size > 1
    val fuel: Boolean get() = markers.any { it.fuel }
    val charging: Boolean get() = markers.any { it.charging }
}

data class MapOverlayPreparation(
    val clusters: List<MapOverlayCluster>,
    val rejected: Int,
    val duplicates: Int,
)

data class MapOverlayLabels(
    val fuel: String = "Fuel",
    val charging: String = "Charging",
    val fuelAndCharging: String = "Fuel and charging",
    val unknownLocation: String = "Location unknown",
    val cityCentroidLocation: String = "Approximate city location",
    val reviewedLocation: String = "Reviewed location",
)

data class MapOverlayListItem(
    val uuid: String,
    val title: String,
    val type: MapStationType,
    val typeLabel: String,
    val locationLabel: String,
    val contentDescription: String,
)

private val CANONICAL_UUID = Regex("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
private const val METERS_PER_DEGREE = 111_320.0
private const val EARTH_RADIUS_METERS = 6_371_000.0

internal fun prepareOverlay(
    markers: List<MapOverlayMarker>,
    radiusMeters: Double = 60.0,
    generation: Long = System.currentTimeMillis(),
): MapOverlayPreparation {
    require(radiusMeters > 0.0)
    val unique = LinkedHashMap<String, MapOverlayMarker>()
    var rejected = 0
    var duplicates = 0
    for (marker in markers) {
        val uuid = marker.uuid.lowercase(Locale.ROOT)
        if (!CANONICAL_UUID.matches(uuid) || !marker.latitude.isFinite() || !marker.longitude.isFinite() ||
            (!marker.fuel && !marker.charging)
        ) {
            rejected++
            continue
        }
        val normalized = marker.copy(uuid = uuid)
        val previous = unique[uuid]
        if (previous == null) {
            unique[uuid] = normalized
        } else {
            duplicates++
            unique[uuid] = mergeDuplicate(previous, normalized)
        }
    }
    if (unique.isEmpty()) return MapOverlayPreparation(emptyList(), rejected, duplicates)

    val meanLatitude = unique.values.sumOf { it.latitude } / unique.size
    val latStep = radiusMeters / METERS_PER_DEGREE
    val lonStep = radiusMeters / (METERS_PER_DEGREE * cos(Math.toRadians(meanLatitude)).coerceAtLeast(0.1))
    val clustersByCell = HashMap<Pair<Long, Long>, MutableList<MutableCluster>>()
    val clusters = mutableListOf<MutableCluster>()

    for (marker in unique.values) {
        val cell = cellOf(marker.latitude, marker.longitude, latStep, lonStep)
        var nearest: MutableCluster? = null
        var nearestDistance = Double.MAX_VALUE
        for (dy in -1L..1L) {
            for (dx in -1L..1L) {
                clustersByCell[cell.first + dy to cell.second + dx]?.forEach { candidate ->
                    val distance = distanceMeters(marker.latitude, marker.longitude, candidate.latitude, candidate.longitude)
                    if (distance <= radiusMeters && distance < nearestDistance) {
                        nearest = candidate
                        nearestDistance = distance
                    }
                }
            }
        }
        if (nearest == null) {
            val cluster = MutableCluster(marker)
            clusters += cluster
            clustersByCell.getOrPut(cell) { mutableListOf() }.add(cluster)
        } else {
            nearest.members += marker
        }
    }

    return MapOverlayPreparation(
        clusters = clusters.mapIndexed { index, cluster ->
            MapOverlayCluster("c$generation-$index", cluster.latitude, cluster.longitude, cluster.members.toList())
        },
        rejected = rejected,
        duplicates = duplicates,
    )
}

fun overlayListItems(
    clusters: List<MapOverlayCluster>,
    labels: MapOverlayLabels = MapOverlayLabels(),
): List<MapOverlayListItem> = clusters
    .flatMap { it.markers }
    .distinctBy { it.uuid }
    .sortedWith(compareBy({ it.title.lowercase(Locale.ROOT) }, { it.uuid }))
    .map { marker ->
        val typeLabel = typeLabel(marker, labels)
        val locationLabel = locationLabel(marker.locationQuality, labels)
        val description = listOf(marker.title, typeLabel, locationLabel, marker.subtitle)
            .filter { it.isNotBlank() }
            .joinToString(", ")
        MapOverlayListItem(
            uuid = marker.uuid,
            title = marker.title,
            type = stationType(marker),
            typeLabel = typeLabel,
            locationLabel = locationLabel,
            contentDescription = description,
        )
    }

internal const val OVERLAY_CATEGORY_NAME = "Abastevo overlay v1:"

// BookmarkManager appends decimal suffixes when a category name already exists.
internal fun overlayCategoryName(generation: Long): String = "${OVERLAY_CATEGORY_NAME}g$generation:"

internal fun isOverlayCategoryGeneration(name: String, generation: Long): Boolean =
    name.startsWith(overlayCategoryName(generation)) &&
        name.removePrefix(overlayCategoryName(generation)).all { it in '0'..'9' }

internal fun isOwnedOverlayCategory(name: String): Boolean {
    if (!name.startsWith(OVERLAY_CATEGORY_NAME)) return false
    val suffix = name.removePrefix(OVERLAY_CATEGORY_NAME)
    return suffix.all { it in '0'..'9' } || Regex("g[0-9]+:[0-9]*").matches(suffix)
}

internal fun buildOverlayKml(
    clusters: List<MapOverlayCluster>,
    categoryName: String = OVERLAY_CATEGORY_NAME,
): String = buildString {
    appendLine("""<?xml version="1.0" encoding="UTF-8"?>""")
    appendLine("""<kml xmlns="http://www.opengis.net/kml/2.2">""")
    appendLine("  <Document>")
    appendLine("    <name>${escapeXml(categoryName)}</name>")
    for (cluster in clusters) {
        val marker = cluster.markers.singleOrNull()
        if (marker == null) {
            appendPlacemark(
                name = "ABASTEVO_CLUSTER:${escapeXml(cluster.id)}",
                description = "${cluster.count} stations",
                latitude = cluster.latitude,
                longitude = cluster.longitude,
                style = "bluegray",
                icon = "Information",
            )
        } else {
            appendPlacemark(
                name = "ABASTEVO:${escapeXml(marker.uuid)}",
                description = describe(marker),
                latitude = marker.latitude,
                longitude = marker.longitude,
                style = if (marker.charging) "teal" else "orange",
                icon = if (marker.charging) "ChargingStation" else "Gas",
            )
        }
    }
    appendLine("  </Document>")
    appendLine("</kml>")
}

private class MutableCluster(val anchor: MapOverlayMarker) {
    val members = mutableListOf(anchor)
    val latitude: Double get() = anchor.latitude
    val longitude: Double get() = anchor.longitude
}

private fun mergeDuplicate(existing: MapOverlayMarker, incoming: MapOverlayMarker): MapOverlayMarker {
    val useIncomingLocation = incoming.locationQuality.rank() > existing.locationQuality.rank()
    return existing.copy(
        latitude = if (useIncomingLocation) incoming.latitude else existing.latitude,
        longitude = if (useIncomingLocation) incoming.longitude else existing.longitude,
        title = existing.title.ifBlank { incoming.title },
        fuel = existing.fuel || incoming.fuel,
        charging = existing.charging || incoming.charging,
        locationQuality = if (useIncomingLocation) incoming.locationQuality else existing.locationQuality,
        subtitle = existing.subtitle.ifBlank { incoming.subtitle },
    )
}

private fun MapStationLocationQuality.rank(): Int = when (this) {
    MapStationLocationQuality.UNKNOWN -> 1
    MapStationLocationQuality.CITY_CENTROID -> 2
    MapStationLocationQuality.REVIEWED -> 3
}

private fun cellOf(latitude: Double, longitude: Double, latStep: Double, lonStep: Double): Pair<Long, Long> =
    kotlin.math.floor(latitude / latStep).toLong() to kotlin.math.floor(longitude / lonStep).toLong()

private fun distanceMeters(lat1: Double, lon1: Double, lat2: Double, lon2: Double): Double {
    val x = Math.toRadians(lon2 - lon1) * cos(Math.toRadians((lat1 + lat2) / 2.0))
    val y = Math.toRadians(lat2 - lat1)
    return sqrt(x * x + y * y) * EARTH_RADIUS_METERS
}

private fun stationType(marker: MapOverlayMarker): MapStationType = when {
    marker.fuel && marker.charging -> MapStationType.FUEL_AND_CHARGING
    marker.charging -> MapStationType.CHARGING
    else -> MapStationType.FUEL
}

private fun typeLabel(marker: MapOverlayMarker, labels: MapOverlayLabels): String = when {
    marker.fuel && marker.charging -> labels.fuelAndCharging
    marker.charging -> labels.charging
    else -> labels.fuel
}

private fun locationLabel(quality: MapStationLocationQuality, labels: MapOverlayLabels): String = when (quality) {
    MapStationLocationQuality.UNKNOWN -> labels.unknownLocation
    MapStationLocationQuality.CITY_CENTROID -> labels.cityCentroidLocation
    MapStationLocationQuality.REVIEWED -> labels.reviewedLocation
}

private fun describe(marker: MapOverlayMarker): String = listOf(
    marker.title,
    typeLabel(marker, MapOverlayLabels()),
    locationLabel(marker.locationQuality, MapOverlayLabels()),
).filter { it.isNotBlank() }.joinToString(" - ")

private fun StringBuilder.appendPlacemark(
    name: String,
    description: String,
    latitude: Double,
    longitude: Double,
    style: String,
    icon: String,
) {
    appendLine("    <Placemark>")
    appendLine("      <name>$name</name>")
    appendLine("      <description>${escapeXml(description)}</description>")
    appendLine("      <styleUrl>#placemark-$style</styleUrl>")
    appendLine("""      <ExtendedData xmlns:mwm="https://comaps.app">""")
    appendLine("        <mwm:icon>$icon</mwm:icon>")
    appendLine("      </ExtendedData>")
    appendLine("      <Point><coordinates>$longitude,$latitude</coordinates></Point>")
    appendLine("    </Placemark>")
}

private fun escapeXml(value: String): String = value
    .replace("&", "&amp;")
    .replace("<", "&lt;")
    .replace(">", "&gt;")
    .replace("\"", "&quot;")
    .replace("'", "&apos;")
