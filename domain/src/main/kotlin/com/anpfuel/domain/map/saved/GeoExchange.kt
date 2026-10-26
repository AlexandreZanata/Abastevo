package com.anpfuel.domain.map.saved

import java.math.BigDecimal
import java.math.RoundingMode
import java.time.Instant

/** Parsed file content; nothing is persisted by parsing. */
data class ExchangeContent(
    val places: List<SavedPlace>,
    val trips: List<SavedTrip>,
    val contentSha256: String,
    val skippedUnsupported: Int,
    val truncatedFields: Int,
)

/**
 * Safe GPX 1.1 / KML 2.2 / KMZ exchange (B-BR-MAP25-04). Parsing is strict and
 * all-or-nothing: the first malformed coordinate, limit breach or forbidden construct
 * refuses the whole file before any record exists.
 */
object GeoExchange {
    const val NS = "https://abastevo.com.br/ns/saved/1"

    fun parse(bytes: ByteArray, nowMillis: Long, newId: () -> String): ExchangeContent {
        if (bytes.isEmpty()) throw SavedValidationException("file-empty")
        if (bytes.size > SavedBounds.MAX_FILE_BYTES) throw SavedValidationException("file-too-large")
        val hash = SavedFingerprint.sha256(bytes)
        val xml = if (KmzReader.looksLikeZip(bytes)) KmzReader.readKml(bytes) else bytes
        val root = StrictXml.parse(xml)
        val ctx = Context(nowMillis, newId)
        when (root.local.lowercase()) {
            "gpx" -> ctx.gpx(root)
            "kml" -> ctx.kml(root)
            else -> throw SavedValidationException("format-unsupported")
        }
        if (ctx.places.size > SavedBounds.MAX_IMPORT_RECORDS || ctx.trips.size > SavedBounds.MAX_TRIPS) {
            throw SavedValidationException("too-many-records")
        }
        if (ctx.places.isEmpty() && ctx.trips.isEmpty()) throw SavedValidationException("file-has-no-records")
        if (ctx.places.map { it.id }.toSet().size != ctx.places.size || ctx.trips.map { it.id }.toSet().size != ctx.trips.size) {
            throw SavedValidationException("duplicate-record-id")
        }
        return ExchangeContent(ctx.places, ctx.trips, hash, ctx.skipped, ctx.truncated)
    }

    private class Context(val now: Long, val newId: () -> String) {
        val places = ArrayList<SavedPlace>()
        val trips = ArrayList<SavedTrip>()
        var skipped = 0
        var truncated = 0

        fun fit(value: String?, max: Int): String? {
            val text = value?.trim()?.filter { !it.isISOControl() || it == '\n' || it == '\t' }?.takeIf { it.isNotEmpty() } ?: return null
            if (text.length <= max) return text
            truncated++
            return text.take(max)
        }

        fun point(lat: String?, lon: String?): GeoPoint {
            if (lat == null || lon == null) throw SavedValidationException("coordinate-malformed")
            return GeoPoint.parse(lat, lon)
        }

        private val uuid = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
        fun idOf(ext: Map<String, String>): String = ext["abv:id"]?.lowercase()?.takeIf { uuid.matches(it) } ?: newId()

        fun place(ext: Map<String, String>, name: String?, note: String?, point: GeoPoint) {
            val station = ext["abv:stationId"]?.lowercase()?.takeIf { uuid.matches(it) }
            val kind = if (ext["abv:kind"] == "STATION" && station != null) SavedPlaceKind.STATION else SavedPlaceKind.PLACE
            val source = runCatching { SavedSourceKind.valueOf(ext["abv:sourceKind"] ?: "") }.getOrDefault(SavedSourceKind.IMPORT)
            places.add(
                SavedPlace(
                    id = idOf(ext), kind = kind,
                    name = fit(name, SavedBounds.MAX_NAME) ?: "Imported place",
                    point = point,
                    stationId = if (kind == SavedPlaceKind.STATION) station else null,
                    sourceKind = source,
                    sourceId = fit(ext["abv:sourceId"], SavedBounds.MAX_SOURCE_ID),
                    note = fit(note, SavedBounds.MAX_NOTE),
                    createdAtMillis = now,
                ),
            )
        }

        fun trip(ext: Map<String, String>, name: String?, note: String?, points: List<TripPoint>, plan: Boolean) {
            if (points.isEmpty()) { skipped++; return }
            if (points.size > SavedBounds.MAX_POINTS_PER_TRIP) throw SavedValidationException("track-too-many-points")
            val avoid = ext["abv:avoid"]?.split(',')?.map { it.trim() }?.filter { it.isNotEmpty() }?.toSet().orEmpty()
            val asPlan = plan && ext["abv:kind"] == "ROUTE_PLAN" && points.size in 2..SavedBounds.MAX_ROUTE_PLAN_POINTS &&
                points.map { it.point }.distinct().size == points.size
            val kind = when {
                asPlan -> SavedTripKind.ROUTE_PLAN
                ext["abv:kind"] == "RECORDED_TRACK" -> SavedTripKind.RECORDED_TRACK
                else -> SavedTripKind.IMPORTED_TRACK
            }
            trips.add(
                SavedTrip(
                    id = idOf(ext), kind = kind, name = fit(name, SavedBounds.MAX_NAME) ?: "Imported trip",
                    points = if (kind == SavedTripKind.ROUTE_PLAN) points.map { TripPoint(it.point) } else points,
                    avoid = if (kind == SavedTripKind.ROUTE_PLAN) avoid.filter { it in setOf("TOLL", "MOTORWAY", "FERRY", "UNPAVED") }.toSet() else emptySet(),
                    sourceId = fit(ext["abv:sourceId"], SavedBounds.MAX_SOURCE_ID),
                    note = fit(note, SavedBounds.MAX_NOTE),
                    createdAtMillis = now,
                ),
            )
        }

        private fun gpxExtensions(node: XmlNode): Map<String, String> =
            node.child("extensions")?.children?.associate { it.name to it.text.toString().trim() }.orEmpty()

        private fun elevation(text: String?): Int? = text?.let {
            try {
                BigDecimal(it.trim()).setScale(2, RoundingMode.HALF_UP).movePointRight(2).intValueExact()
            } catch (_: Exception) { null }
        }?.takeIf { it in -1_000_000..10_000_000 }

        private fun instant(text: String?): Long? = text?.let { runCatching { Instant.parse(it.trim()).toEpochMilli() }.getOrNull() }?.takeIf { it >= 0 }

        fun gpx(root: XmlNode) {
            for (wpt in root.all("wpt")) {
                place(gpxExtensions(wpt), wpt.textOf("name"), wpt.textOf("desc") ?: wpt.textOf("cmt"), point(wpt.attributes["lat"], wpt.attributes["lon"]))
            }
            for (trk in root.all("trk")) {
                val points = trk.all("trkseg").flatMap { seg ->
                    seg.all("trkpt").map {
                        TripPoint(point(it.attributes["lat"], it.attributes["lon"]), elevation(it.textOf("ele")), instant(it.textOf("time")))
                    }
                }
                trip(gpxExtensions(trk), trk.textOf("name"), trk.textOf("desc"), points, plan = false)
            }
            for (rte in root.all("rte")) {
                val points = rte.all("rtept").map { TripPoint(point(it.attributes["lat"], it.attributes["lon"])) }
                trip(gpxExtensions(rte), rte.textOf("name"), rte.textOf("desc"), points, plan = true)
            }
            skipped += root.children.count { it.local !in setOf("wpt", "trk", "rte", "metadata", "extensions") }
        }

        private fun coordinates(text: String): List<TripPoint> =
            text.trim().split(Regex("\\s+")).filter { it.isNotEmpty() }.map { token ->
                val parts = token.split(',')
                if (parts.size !in 2..3) throw SavedValidationException("coordinate-malformed")
                TripPoint(point(parts[1], parts[0]), parts.getOrNull(2)?.let { elevation(it) })
            }

        private fun kmlExtended(node: XmlNode): Map<String, String> {
            val map = HashMap<String, String>()
            node.child("ExtendedData")?.all("Data")?.forEach { d ->
                val key = d.attributes["name"] ?: return@forEach
                map[key] = d.textOf("value") ?: ""
            }
            return map
        }

        private fun stripHtml(text: String?): String? = text?.replace(Regex("<[^>]{0,200}>"), " ")?.replace(Regex("\\s+"), " ")?.trim()

        fun kml(root: XmlNode) {
            fun walk(node: XmlNode, depth: Int) {
                if (depth > SavedBounds.MAX_XML_DEPTH) throw SavedValidationException("xml-too-deep")
                for (child in node.children) {
                    when (child.local) {
                        "Document", "Folder" -> walk(child, depth + 1)
                        "Placemark" -> placemark(child)
                    }
                }
            }
            walk(root, 0)
        }

        private fun placemark(node: XmlNode) {
            val ext = kmlExtended(node)
            val name = node.textOf("name")
            val note = stripHtml(node.textOf("description"))
            val geometries = ArrayList<XmlNode>()
            fun collect(n: XmlNode) {
                for (c in n.children) when (c.local) {
                    "Point", "LineString" -> geometries.add(c)
                    "MultiGeometry" -> collect(c)
                    "Polygon", "Track", "MultiTrack", "LinearRing" -> skipped++
                }
            }
            collect(node)
            if (geometries.isEmpty()) { skipped++; return }
            for (g in geometries) {
                val coords = g.textOf("coordinates") ?: throw SavedValidationException("coordinate-malformed")
                val points = coordinates(coords)
                if (g.local == "Point") {
                    if (points.size != 1) throw SavedValidationException("coordinate-malformed")
                    place(ext, name, note, points.single().point)
                } else {
                    trip(ext, name, note, points, plan = true)
                }
            }
        }
    }

    // ---- export ----------------------------------------------------------------

    const val LICENSE_NOTICE =
        "Contains public ANP open data, Abastevo community contributions and OpenStreetMap-derived " +
            "route plans. Check the original source terms before redistributing. This file may reveal " +
            "personal places and routes."

    private fun esc(text: String): String = buildString(text.length) {
        for (c in text) when (c) {
            '&' -> append("&amp;"); '<' -> append("&lt;"); '>' -> append("&gt;")
            '"' -> append("&quot;"); '\'' -> append("&apos;")
            else -> if (!c.isISOControl() || c == '\n' || c == '\t') append(c)
        }
    }

    private fun coord(micro: Long): String = BigDecimal.valueOf(micro, 6).toPlainString()
    private fun ele(cm: Int): String = BigDecimal.valueOf(cm.toLong(), 2).toPlainString()

    private fun extGpx(vararg pairs: Pair<String, String?>) = pairs.filter { it.second != null }
        .joinToString("") { "<abv:${it.first}>${esc(it.second!!)}</abv:${it.first}>" }
        .let { if (it.isEmpty()) "" else "<extensions>$it</extensions>" }

    fun writeGpx(places: List<SavedPlace>, trips: List<SavedTrip>): ByteArray {
        val sb = StringBuilder()
        sb.append("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
        sb.append("<gpx version=\"1.1\" creator=\"Abastevo\" xmlns=\"http://www.topografix.com/GPX/1/1\" xmlns:abv=\"$NS\">\n")
        sb.append("<metadata><desc>${esc(LICENSE_NOTICE)}</desc></metadata>\n")
        for (p in places.sortedBy { it.id }) {
            sb.append("<wpt lat=\"${coord(p.point.latMicro)}\" lon=\"${coord(p.point.lonMicro)}\"><name>${esc(p.name)}</name>")
            p.note?.let { sb.append("<desc>${esc(it)}</desc>") }
            sb.append(extGpx("id" to p.id, "kind" to p.kind.name, "stationId" to p.stationId, "sourceKind" to p.sourceKind.name, "sourceId" to p.sourceId))
            sb.append("</wpt>\n")
        }
        for (t in trips.sortedBy { it.id }) {
            val plan = t.kind == SavedTripKind.ROUTE_PLAN
            sb.append(if (plan) "<rte>" else "<trk>")
            sb.append("<name>${esc(t.name)}</name>")
            t.note?.let { sb.append("<desc>${esc(it)}</desc>") }
            sb.append(extGpx("id" to t.id, "kind" to t.kind.name, "sourceId" to t.sourceId, "avoid" to t.avoid.sorted().joinToString(",").ifEmpty { null }))
            if (!plan) sb.append("<trkseg>")
            for (pt in t.points) {
                val tag = if (plan) "rtept" else "trkpt"
                sb.append("<$tag lat=\"${coord(pt.point.latMicro)}\" lon=\"${coord(pt.point.lonMicro)}\">")
                pt.elevationCm?.let { sb.append("<ele>${ele(it)}</ele>") }
                pt.timeMillis?.let { sb.append("<time>${Instant.ofEpochMilli(it)}</time>") }
                sb.append("</$tag>")
            }
            if (!plan) sb.append("</trkseg>")
            sb.append(if (plan) "</rte>\n" else "</trk>\n")
        }
        sb.append("</gpx>\n")
        return sb.toString().toByteArray(Charsets.UTF_8)
    }

    private fun extKml(vararg pairs: Pair<String, String?>) = pairs.filter { it.second != null }
        .joinToString("") { "<Data name=\"abv:${it.first}\"><value>${esc(it.second!!)}</value></Data>" }
        .let { if (it.isEmpty()) "" else "<ExtendedData>$it</ExtendedData>" }

    fun writeKml(places: List<SavedPlace>, trips: List<SavedTrip>): ByteArray {
        val sb = StringBuilder()
        sb.append("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
        sb.append("<kml xmlns=\"http://www.opengis.net/kml/2.2\"><Document>\n")
        sb.append("<description>${esc(LICENSE_NOTICE)}</description>\n")
        for (p in places.sortedBy { it.id }) {
            sb.append("<Placemark><name>${esc(p.name)}</name>")
            p.note?.let { sb.append("<description>${esc(it)}</description>") }
            sb.append(extKml("id" to p.id, "kind" to p.kind.name, "stationId" to p.stationId, "sourceKind" to p.sourceKind.name, "sourceId" to p.sourceId))
            sb.append("<Point><coordinates>${coord(p.point.lonMicro)},${coord(p.point.latMicro)}</coordinates></Point></Placemark>\n")
        }
        for (t in trips.sortedBy { it.id }) {
            sb.append("<Placemark><name>${esc(t.name)}</name>")
            t.note?.let { sb.append("<description>${esc(it)}</description>") }
            sb.append(extKml("id" to t.id, "kind" to t.kind.name, "sourceId" to t.sourceId, "avoid" to t.avoid.sorted().joinToString(",").ifEmpty { null }))
            sb.append("<LineString><coordinates>")
            sb.append(t.points.joinToString(" ") { pt ->
                "${coord(pt.point.lonMicro)},${coord(pt.point.latMicro)}" + (pt.elevationCm?.let { ",${ele(it)}" } ?: "")
            })
            sb.append("</coordinates></LineString></Placemark>\n")
        }
        sb.append("</Document></kml>\n")
        return sb.toString().toByteArray(Charsets.UTF_8)
    }
}
