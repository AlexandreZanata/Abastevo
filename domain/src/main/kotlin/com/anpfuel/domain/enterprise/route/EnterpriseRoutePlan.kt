package com.anpfuel.domain.enterprise.route

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncJson
import java.time.Instant

/*
 * Enterprise route plans, device CAR computations and sealed local route book
 * (EMP-ROUTE-001..005, BR-ROUTE-001..010). Coordinates and labels are redacted
 * in string representations.
 */

enum class RouteFailure {
    INVALID, POINT_LIMIT, FOREIGN_BINDING, PROFILE_UNSUPPORTED, REVISION_CONFLICT
}

class RouteDomainException(val failure: RouteFailure = RouteFailure.INVALID) :
    IllegalArgumentException("route: ${failure.name.lowercase()}")

private fun invalid(failure: RouteFailure = RouteFailure.INVALID): RouteDomainException =
    RouteDomainException(failure)

enum class RoutePlanOwnerKind {
    MISSION, TRIP;

    companion object {
        fun parse(raw: String): RoutePlanOwnerKind =
            entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

enum class RouteWaypointSource {
    ORGANIZATION_PLACE, STATION_CATALOG, OFFLINE_MAP_SEARCH, COORDINATES, TEXT_ONLY;

    companion object {
        fun parse(raw: String): RouteWaypointSource =
            entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

enum class PlannedDistanceSource {
    MANUAL, ENGINE;

    companion object {
        fun parse(raw: String): PlannedDistanceSource =
            entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

enum class RouteComputationKind {
    PLANNING, DEPARTURE, RECALCULATION;

    companion object {
        fun parse(raw: String): RouteComputationKind =
            entries.firstOrNull { it.name == raw } ?: throw invalid()
    }
}

/** Validated WGS84 coordinate in 1e-7 degrees. */
class GeoPointE7(val latE7: Int, val lonE7: Int) {
    init {
        if (latE7 !in -900_000_000..900_000_000 || lonE7 !in -1_800_000_000..1_800_000_000) {
            throw invalid()
        }
    }

    override fun equals(other: Any?): Boolean =
        other is GeoPointE7 && latE7 == other.latE7 && lonE7 == other.lonE7

    override fun hashCode(): Int = 31 * latE7 + lonE7
    override fun toString(): String = "GeoPointE7(redacted)"
}

/** Routing avoidance options (BR-ROUTE-004). */
data class RouteAvoidOptions(
    val avoidToll: Boolean = false,
    val avoidMotorway: Boolean = false,
    val avoidFerry: Boolean = false,
    val avoidUnpaved: Boolean = false,
) {
    override fun toString(): String =
        "RouteAvoidOptions(toll=$avoidToll,motorway=$avoidMotorway,ferry=$avoidFerry,unpaved=$avoidUnpaved)"
}

/** Ordered route waypoint; TEXT_ONLY omits coordinates until driver resolves it on device. */
class RouteWaypoint(
    val sequence: Int,
    label: String,
    val source: RouteWaypointSource,
    val placeId: String? = null,
    val stationId: String? = null,
    note: String? = null,
    val point: GeoPointE7? = null,
    val dwellMinutes: Int? = null,
) {
    val label: String = validateRouteText(label, 160, required = true)
    val note: String? = note?.let { validateRouteText(it, 280, required = false).takeIf(String::isNotEmpty) }

    init {
        if (sequence < 1) throw invalid()
        placeId?.let { runCatching { EnterprisePartition.requireId(it) }.getOrElse { throw invalid() } }
        stationId?.let { runCatching { EnterprisePartition.requireId(it) }.getOrElse { throw invalid() } }
        when (source) {
            RouteWaypointSource.TEXT_ONLY -> if (point != null) throw invalid()
            RouteWaypointSource.ORGANIZATION_PLACE -> if (placeId == null || point == null) throw invalid()
            RouteWaypointSource.STATION_CATALOG -> if (stationId == null || point == null) throw invalid()
            RouteWaypointSource.OFFLINE_MAP_SEARCH, RouteWaypointSource.COORDINATES -> if (point == null) throw invalid()
        }
        if (dwellMinutes != null && dwellMinutes !in 0..10_080) throw invalid()
    }

    val resolvable: Boolean get() = point != null

    fun withSequence(newSequence: Int): RouteWaypoint =
        RouteWaypoint(newSequence, label, source, placeId, stationId, note, point, dwellMinutes)

    override fun equals(other: Any?): Boolean =
        other is RouteWaypoint &&
            sequence == other.sequence &&
            label == other.label &&
            source == other.source &&
            placeId == other.placeId &&
            stationId == other.stationId &&
            note == other.note &&
            point == other.point &&
            dwellMinutes == other.dwellMinutes

    override fun hashCode(): Int {
        var result = sequence
        result = 31 * result + label.hashCode()
        result = 31 * result + source.hashCode()
        result = 31 * result + (placeId?.hashCode() ?: 0)
        result = 31 * result + (stationId?.hashCode() ?: 0)
        result = 31 * result + (note?.hashCode() ?: 0)
        result = 31 * result + (point?.hashCode() ?: 0)
        result = 31 * result + (dwellMinutes ?: 0)
        return result
    }

    override fun toString(): String = "RouteWaypoint(seq=$sequence,source=${source.name},redacted)"
}

/** Result of an on-device or workstation CAR route computation (EMP-ROUTE-005/006). */
class EnterpriseRouteComputation(
    val id: String,
    val ownerKind: RoutePlanOwnerKind,
    val ownerId: String,
    val planRevision: Long,
    engineId: String,
    mapDataVersion: String,
    val profile: String = PROFILE_CAR,
    val kind: RouteComputationKind,
    val options: RouteAvoidOptions,
    val distanceMetres: Long,
    val durationSeconds: Long,
    line: List<GeoPointE7>,
    attribution: String = OSM_ATTRIBUTION,
    val computedAt: Instant,
) {
    val engineId: String = validateRouteText(engineId, 64, required = true)
    val mapDataVersion: String = validateRouteText(mapDataVersion, 64, required = true)
    val attribution: String = validateRouteText(attribution, 160, required = true)
    val line: List<GeoPointE7> = line.toList()

    init {
        runCatching {
            EnterprisePartition.requireId(id)
            EnterprisePartition.requireId(ownerId)
        }.getOrElse { throw invalid() }
        if (planRevision < 1L) throw invalid()
        if (profile != PROFILE_CAR) throw invalid(RouteFailure.PROFILE_UNSUPPORTED)
        if (distanceMetres !in 0L..MAX_DISTANCE_METRES || durationSeconds !in 0L..MAX_DURATION_SECONDS) throw invalid()
        if (this.line.size > MAX_GEOMETRY_POINTS) throw invalid()
    }

    override fun equals(other: Any?): Boolean =
        other is EnterpriseRouteComputation &&
            id == other.id &&
            ownerKind == other.ownerKind &&
            ownerId == other.ownerId &&
            planRevision == other.planRevision &&
            engineId == other.engineId &&
            mapDataVersion == other.mapDataVersion &&
            profile == other.profile &&
            kind == other.kind &&
            options == other.options &&
            distanceMetres == other.distanceMetres &&
            durationSeconds == other.durationSeconds &&
            line == other.line &&
            attribution == other.attribution &&
            computedAt == other.computedAt

    override fun hashCode(): Int {
        var result = id.hashCode()
        result = 31 * result + ownerKind.hashCode()
        result = 31 * result + ownerId.hashCode()
        result = 31 * result + planRevision.hashCode()
        result = 31 * result + distanceMetres.hashCode()
        result = 31 * result + durationSeconds.hashCode()
        return result
    }

    override fun toString(): String = "EnterpriseRouteComputation(rev=$planRevision,kind=${kind.name},redacted)"

    companion object {
        const val PROFILE_CAR = "CAR"
        const val OSM_ATTRIBUTION = "© OpenStreetMap contributors"
        const val MAX_DISTANCE_METRES = 100_000_000L
        const val MAX_DURATION_SECONDS = 10_000_000L
        const val MAX_GEOMETRY_POINTS = 1000
    }
}

/** Validated multi-stop enterprise route plan (1..102 waypoints: origin + <=100 stops + dest). */
class EnterpriseRoutePlan private constructor(
    val id: String,
    val revision: Long,
    val ownerKind: RoutePlanOwnerKind,
    val ownerId: String,
    waypoints: List<RouteWaypoint>,
    val options: RouteAvoidOptions,
    val plannedDistanceMetres: Long?,
    val plannedDistanceSource: PlannedDistanceSource?,
    val heavyAssetNotice: Boolean,
    val latestComputation: EnterpriseRouteComputation?,
    val updatedAt: Instant,
) {
    val waypoints: List<RouteWaypoint> = waypoints.toList()

    val hasUnresolvedTextStops: Boolean get() = waypoints.any { !it.resolvable }

    val routableWaypoints: List<RouteWaypoint> get() = waypoints.filter { it.resolvable }

    fun withComputation(computation: EnterpriseRouteComputation): EnterpriseRoutePlan {
        if (computation.ownerKind != ownerKind || computation.ownerId != ownerId || computation.planRevision != revision) {
            throw invalid()
        }
        return EnterpriseRoutePlan(
            id = id,
            revision = revision,
            ownerKind = ownerKind,
            ownerId = ownerId,
            waypoints = waypoints,
            options = options,
            plannedDistanceMetres = plannedDistanceMetres ?: computation.distanceMetres,
            plannedDistanceSource = plannedDistanceSource ?: PlannedDistanceSource.ENGINE,
            heavyAssetNotice = heavyAssetNotice,
            latestComputation = computation,
            updatedAt = computation.computedAt,
        )
    }

    override fun equals(other: Any?): Boolean =
        other is EnterpriseRoutePlan &&
            id == other.id &&
            revision == other.revision &&
            ownerKind == other.ownerKind &&
            ownerId == other.ownerId &&
            waypoints == other.waypoints &&
            options == other.options &&
            plannedDistanceMetres == other.plannedDistanceMetres &&
            plannedDistanceSource == other.plannedDistanceSource &&
            heavyAssetNotice == other.heavyAssetNotice &&
            latestComputation == other.latestComputation &&
            updatedAt == other.updatedAt

    override fun hashCode(): Int {
        var result = id.hashCode()
        result = 31 * result + revision.hashCode()
        result = 31 * result + ownerId.hashCode()
        return result
    }

    override fun toString(): String = "EnterpriseRoutePlan(rev=$revision,stops=${waypoints.size},redacted)"

    companion object {
        const val MIN_WAYPOINTS = 1
        const val MAX_WAYPOINTS = 102

        fun create(
            id: String,
            revision: Long,
            ownerKind: RoutePlanOwnerKind,
            ownerId: String,
            waypoints: List<RouteWaypoint>,
            options: RouteAvoidOptions = RouteAvoidOptions(),
            plannedDistanceMetres: Long? = null,
            plannedDistanceSource: PlannedDistanceSource? = null,
            heavyAssetNotice: Boolean = false,
            latestComputation: EnterpriseRouteComputation? = null,
            updatedAt: Instant,
        ): EnterpriseRoutePlan {
            runCatching {
                EnterprisePartition.requireId(id)
                EnterprisePartition.requireId(ownerId)
            }.getOrElse { throw invalid() }
            if (revision < 1L) throw invalid()
            if (waypoints.size > MAX_WAYPOINTS) throw invalid(RouteFailure.POINT_LIMIT)
            if (waypoints.size < MIN_WAYPOINTS) throw invalid()
            val seenCoords = HashSet<GeoPointE7>()
            val normalized = waypoints.mapIndexed { index, wp ->
                if (wp.sequence != index + 1) throw invalid()
                wp.point?.let { pt -> if (!seenCoords.add(pt)) throw invalid() }
                wp
            }
            if ((plannedDistanceMetres == null) != (plannedDistanceSource == null)) throw invalid()
            if (plannedDistanceMetres != null && plannedDistanceMetres !in 1L..EnterpriseRouteComputation.MAX_DISTANCE_METRES) {
                throw invalid()
            }
            if (plannedDistanceSource == PlannedDistanceSource.ENGINE && latestComputation == null) {
                throw invalid()
            }
            if (latestComputation != null &&
                (latestComputation.ownerKind != ownerKind || latestComputation.ownerId != ownerId || latestComputation.planRevision != revision)
            ) {
                throw invalid()
            }
            return EnterpriseRoutePlan(
                id = id,
                revision = revision,
                ownerKind = ownerKind,
                ownerId = ownerId,
                waypoints = normalized,
                options = options,
                plannedDistanceMetres = plannedDistanceMetres,
                plannedDistanceSource = plannedDistanceSource,
                heavyAssetNotice = heavyAssetNotice,
                latestComputation = latestComputation,
                updatedAt = updatedAt,
            )
        }
    }
}

/** Sealed partition-bound local route book stored inside the encrypted enterprise vault. */
class EnterpriseRouteBook private constructor(
    val membershipId: String,
    val enrollmentId: String,
    plans: List<EnterpriseRoutePlan>,
) {
    val plans: List<EnterpriseRoutePlan> = plans.toList()

    init {
        runCatching {
            EnterprisePartition.requireId(membershipId)
            EnterprisePartition.requireId(enrollmentId)
        }.getOrElse { throw invalid() }
        val ownerKeys = this.plans.map { it.ownerKind to it.ownerId }
        if (ownerKeys.distinct().size != ownerKeys.size) throw invalid()
        if (this.plans.size > MAX_PLANS) throw invalid()
    }

    fun requireBinding(membershipId: String, enrollmentId: String): EnterpriseRouteBook {
        if (membershipId != this.membershipId || enrollmentId != this.enrollmentId) {
            throw invalid(RouteFailure.FOREIGN_BINDING)
        }
        return this
    }

    fun planFor(ownerKind: RoutePlanOwnerKind, ownerId: String): EnterpriseRoutePlan? =
        plans.firstOrNull { it.ownerKind == ownerKind && it.ownerId == ownerId }

    fun withPlan(plan: EnterpriseRoutePlan): EnterpriseRouteBook {
        val rest = plans.filterNot { it.ownerKind == plan.ownerKind && it.ownerId == plan.ownerId }
        return EnterpriseRouteBook(membershipId, enrollmentId, rest + plan)
    }

    val canonicalBytes: ByteArray
        get() = CanonicalJson.encode(
            SyncJson.Object(
                mapOf(
                    "format" to SyncJson.Text(FORMAT),
                    "version" to SyncJson.Integer(VERSION.toString()),
                    "membership_id" to SyncJson.Text(membershipId),
                    "enrollment_id" to SyncJson.Text(enrollmentId),
                    "plans" to SyncJson.Array(plans.map(::encodePlan)),
                ),
            ),
        )

    override fun equals(other: Any?): Boolean =
        other is EnterpriseRouteBook &&
            membershipId == other.membershipId &&
            enrollmentId == other.enrollmentId &&
            plans == other.plans

    override fun hashCode(): Int = 31 * (31 * membershipId.hashCode() + enrollmentId.hashCode()) + plans.hashCode()
    override fun toString(): String = "EnterpriseRouteBook(plans=${plans.size},redacted)"

    companion object {
        const val FORMAT = "route-book"
        const val VERSION = 1L
        const val MAX_PLANS = 200
        const val MAX_BYTES = 4 * 1024 * 1024

        private val ROOT_KEYS = setOf("format", "version", "membership_id", "enrollment_id", "plans")
        private val PLAN_KEYS = setOf(
            "id", "revision", "owner_kind", "owner_id", "waypoints", "options",
            "planned_distance_metres", "planned_distance_source", "heavy_asset_notice",
            "latest_computation", "updated_at",
        )
        private val WAYPOINT_KEYS = setOf(
            "sequence", "label", "source", "place_id", "station_id", "note",
            "lat_e7", "lon_e7", "dwell_minutes",
        )
        private val OPTIONS_KEYS = setOf("avoid_toll", "avoid_motorway", "avoid_ferry", "avoid_unpaved")
        private val COMP_KEYS = setOf(
            "id", "owner_kind", "owner_id", "plan_revision", "engine_id", "map_data_version",
            "profile", "kind", "options", "distance_metres", "duration_seconds", "line",
            "attribution", "computed_at",
        )
        private val PT_KEYS = setOf("lat_e7", "lon_e7")

        fun empty(membershipId: String, enrollmentId: String): EnterpriseRouteBook =
            EnterpriseRouteBook(membershipId, enrollmentId, emptyList())

        fun decode(raw: ByteArray): EnterpriseRouteBook {
            val root = obj(CanonicalJson.parseLarge(raw, MAX_BYTES), ROOT_KEYS)
            if (text(root, "format") != FORMAT || integer(root, "version") != VERSION) throw invalid()
            val book = EnterpriseRouteBook(
                membershipId = text(root, "membership_id"),
                enrollmentId = text(root, "enrollment_id"),
                plans = arr(root, "plans").map(::decodePlan),
            )
            if (!book.canonicalBytes.contentEquals(raw)) throw invalid()
            return book
        }

        private fun encodePlan(p: EnterpriseRoutePlan): SyncJson.Object = SyncJson.Object(
            mapOf(
                "id" to SyncJson.Text(p.id),
                "revision" to SyncJson.Integer(p.revision.toString()),
                "owner_kind" to SyncJson.Text(p.ownerKind.name),
                "owner_id" to SyncJson.Text(p.ownerId),
                "waypoints" to SyncJson.Array(p.waypoints.map(::encodeWaypoint)),
                "options" to encodeOptions(p.options),
                "planned_distance_metres" to (p.plannedDistanceMetres?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null),
                "planned_distance_source" to (p.plannedDistanceSource?.let { SyncJson.Text(it.name) } ?: SyncJson.Null),
                "heavy_asset_notice" to SyncJson.Bool(p.heavyAssetNotice),
                "latest_computation" to (p.latestComputation?.let(::encodeComp) ?: SyncJson.Null),
                "updated_at" to SyncJson.Text(EnterpriseTime.formatInstant(p.updatedAt)),
            ),
        )

        private fun encodeWaypoint(w: RouteWaypoint): SyncJson.Object = SyncJson.Object(
            mapOf(
                "sequence" to SyncJson.Integer(w.sequence.toString()),
                "label" to SyncJson.Text(w.label),
                "source" to SyncJson.Text(w.source.name),
                "place_id" to (w.placeId?.let(SyncJson::Text) ?: SyncJson.Null),
                "station_id" to (w.stationId?.let(SyncJson::Text) ?: SyncJson.Null),
                "note" to (w.note?.let(SyncJson::Text) ?: SyncJson.Null),
                "lat_e7" to (w.point?.latE7?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null),
                "lon_e7" to (w.point?.lonE7?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null),
                "dwell_minutes" to (w.dwellMinutes?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null),
            ),
        )

        private fun encodeOptions(o: RouteAvoidOptions): SyncJson.Object = SyncJson.Object(
            mapOf(
                "avoid_toll" to SyncJson.Bool(o.avoidToll),
                "avoid_motorway" to SyncJson.Bool(o.avoidMotorway),
                "avoid_ferry" to SyncJson.Bool(o.avoidFerry),
                "avoid_unpaved" to SyncJson.Bool(o.avoidUnpaved),
            ),
        )

        private fun encodeComp(c: EnterpriseRouteComputation): SyncJson.Object = SyncJson.Object(
            mapOf(
                "id" to SyncJson.Text(c.id),
                "owner_kind" to SyncJson.Text(c.ownerKind.name),
                "owner_id" to SyncJson.Text(c.ownerId),
                "plan_revision" to SyncJson.Integer(c.planRevision.toString()),
                "engine_id" to SyncJson.Text(c.engineId),
                "map_data_version" to SyncJson.Text(c.mapDataVersion),
                "profile" to SyncJson.Text(c.profile),
                "kind" to SyncJson.Text(c.kind.name),
                "options" to encodeOptions(c.options),
                "distance_metres" to SyncJson.Integer(c.distanceMetres.toString()),
                "duration_seconds" to SyncJson.Integer(c.durationSeconds.toString()),
                "line" to SyncJson.Array(
                    c.line.map { pt ->
                        SyncJson.Object(
                            mapOf(
                                "lat_e7" to SyncJson.Integer(pt.latE7.toString()),
                                "lon_e7" to SyncJson.Integer(pt.lonE7.toString()),
                            ),
                        )
                    },
                ),
                "attribution" to SyncJson.Text(c.attribution),
                "computed_at" to SyncJson.Text(EnterpriseTime.formatInstant(c.computedAt)),
            ),
        )

        private fun decodePlan(node: SyncJson): EnterpriseRoutePlan {
            val o = obj(node, PLAN_KEYS)
            val compNode = o.fields["latest_computation"]
            val comp = if (compNode === SyncJson.Null) null else decodeComp(obj(compNode, COMP_KEYS))
            return EnterpriseRoutePlan.create(
                id = text(o, "id"),
                revision = integer(o, "revision"),
                ownerKind = RoutePlanOwnerKind.parse(text(o, "owner_kind")),
                ownerId = text(o, "owner_id"),
                waypoints = arr(o, "waypoints").map(::decodeWaypoint),
                options = decodeOptions(obj(o.fields["options"], OPTIONS_KEYS)),
                plannedDistanceMetres = optInteger(o, "planned_distance_metres"),
                plannedDistanceSource = optText(o, "planned_distance_source")?.let(PlannedDistanceSource::parse),
                heavyAssetNotice = bool(o, "heavy_asset_notice"),
                latestComputation = comp,
                updatedAt = EnterpriseTime.parseInstant(text(o, "updated_at")),
            )
        }

        private fun decodeWaypoint(node: SyncJson): RouteWaypoint {
            val w = obj(node, WAYPOINT_KEYS)
            val lat = optInteger(w, "lat_e7")?.toIntExact()
            val lon = optInteger(w, "lon_e7")?.toIntExact()
            val pt = when {
                lat == null && lon == null -> null
                lat != null && lon != null -> GeoPointE7(lat, lon)
                else -> throw invalid()
            }
            return RouteWaypoint(
                sequence = integer(w, "sequence").toIntExact(),
                label = text(w, "label"),
                source = RouteWaypointSource.parse(text(w, "source")),
                placeId = optText(w, "place_id"),
                stationId = optText(w, "station_id"),
                note = optText(w, "note"),
                point = pt,
                dwellMinutes = optInteger(w, "dwell_minutes")?.toIntExact(),
            )
        }

        private fun decodeOptions(o: SyncJson.Object): RouteAvoidOptions = RouteAvoidOptions(
            avoidToll = bool(o, "avoid_toll"),
            avoidMotorway = bool(o, "avoid_motorway"),
            avoidFerry = bool(o, "avoid_ferry"),
            avoidUnpaved = bool(o, "avoid_unpaved"),
        )

        private fun decodeComp(c: SyncJson.Object): EnterpriseRouteComputation = EnterpriseRouteComputation(
            id = text(c, "id"),
            ownerKind = RoutePlanOwnerKind.parse(text(c, "owner_kind")),
            ownerId = text(c, "owner_id"),
            planRevision = integer(c, "plan_revision"),
            engineId = text(c, "engine_id"),
            mapDataVersion = text(c, "map_data_version"),
            profile = text(c, "profile"),
            kind = RouteComputationKind.parse(text(c, "kind")),
            options = decodeOptions(obj(c.fields["options"], OPTIONS_KEYS)),
            distanceMetres = integer(c, "distance_metres"),
            durationSeconds = integer(c, "duration_seconds"),
            line = arr(c, "line").map { item ->
                val p = obj(item, PT_KEYS)
                GeoPointE7(integer(p, "lat_e7").toIntExact(), integer(p, "lon_e7").toIntExact())
            },
            attribution = text(c, "attribution"),
            computedAt = EnterpriseTime.parseInstant(text(c, "computed_at")),
        )

        private fun obj(value: SyncJson?, keys: Set<String>): SyncJson.Object {
            val result = value as? SyncJson.Object ?: throw invalid()
            if (result.fields.keys != keys) throw invalid()
            return result
        }

        private fun arr(value: SyncJson.Object, key: String): List<SyncJson> =
            (value.fields[key] as? SyncJson.Array ?: throw invalid()).values

        private fun text(value: SyncJson.Object, key: String): String =
            (value.fields[key] as? SyncJson.Text)?.value ?: throw invalid()

        private fun optText(value: SyncJson.Object, key: String): String? =
            value.fields[key].let { if (it === SyncJson.Null) null else text(value, key) }

        private fun integer(value: SyncJson.Object, key: String): Long =
            (value.fields[key] as? SyncJson.Integer)?.lexeme?.toLongOrNull() ?: throw invalid()

        private fun optInteger(value: SyncJson.Object, key: String): Long? =
            value.fields[key].let { if (it === SyncJson.Null) null else integer(value, key) }

        private fun bool(value: SyncJson.Object, key: String): Boolean =
            (value.fields[key] as? SyncJson.Bool)?.value ?: throw invalid()

        private fun Long.toIntExact(): Int {
            if (this < Int.MIN_VALUE || this > Int.MAX_VALUE) throw invalid()
            return toInt()
        }
    }
}

private fun validateRouteText(raw: String, maxCodePoints: Int, required: Boolean): String {
    val trimmed = raw.trim()
    if (required && trimmed.isEmpty()) throw invalid()
    if (trimmed.codePointCount(0, trimmed.length) > maxCodePoints) throw invalid()
    for (ch in raw) {
        if (Character.isISOControl(ch) || ch in '\u202A'..'\u202E' || ch in '\u2066'..'\u2069') {
            throw invalid()
        }
    }
    return trimmed
}
