package com.anpfuel.data.enterprise.trip

import com.anpfuel.application.enterprise.agreement.FieldVoucherNotice
import com.anpfuel.application.enterprise.management.FieldManagementAdvisory
import com.anpfuel.application.enterprise.trip.TripFieldReference
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.route.EnterpriseRoutePlan
import com.anpfuel.domain.enterprise.route.GeoPointE7
import com.anpfuel.domain.enterprise.route.PlannedDistanceSource
import com.anpfuel.domain.enterprise.route.RouteAvoidOptions
import com.anpfuel.domain.enterprise.route.RoutePlanOwnerKind
import com.anpfuel.domain.enterprise.route.RouteWaypoint
import com.anpfuel.domain.enterprise.route.RouteWaypointSource
import java.time.OffsetDateTime
import org.json.JSONObject
import org.json.JSONTokener

internal object TripFieldWire {
    const val MAX_BYTES = 128 * 1024

    fun read(raw: String, requestId: String, member: String, enrollment: String, tripId: String): TripFieldReference {
        val input = JSONTokener(raw)
        val root = input.nextValue() as JSONObject
        require(input.nextClean() == '\u0000')
        require(root.keys().asSequence().toSet() == setOf("schema_version", "request_id", "membership_id", "enrollment_id", "field"))
        require(root.getInt("schema_version") == 1 && root.getString("request_id") == requestId &&
            root.getString("membership_id") == member && root.getString("enrollment_id") == enrollment)
        val field = root.getJSONObject("field")
        require(field.keys().asSequence().toSet() == setOf("trip_id", "plan", "expired", "due_soon", "nearest_days", "open_maintenance", "voucher"))
        require(field.getString("trip_id") == tripId)
        val expired = field.count("expired")
        val due = field.count("due_soon")
        val nearest = if (field.isNull("nearest_days")) null else field.count("nearest_days")
        require((nearest == null) == (due == 0))
        val advisory = FieldManagementAdvisory(expired, due, nearest, field.count("open_maintenance"), 0)
        val voucher = field.optJSONObject("voucher")?.let(::voucher)
        require(voucher != null || field.isNull("voucher"))
        val plan = field.optJSONObject("plan")?.let(::plan)
        require(plan != null || field.isNull("plan"))
        require(plan == null || plan.ownerKind == RoutePlanOwnerKind.MISSION || plan.ownerId == tripId)
        return TripFieldReference(tripId, plan, advisory, voucher)
    }

    private fun voucher(v: JSONObject): FieldVoucherNotice {
        require(v.keys().asSequence().toSet() == setOf("id", "product", "unit", "max_quantity_milli", "max_centavos", "valid_until", "state"))
        return FieldVoucherNotice(
            v.getString("id").also(EnterprisePartition::requireId), v.getString("product"), v.getString("unit"),
            v.getLong("max_quantity_milli"), v.getLong("max_centavos"), v.getLong("valid_until"), v.getString("state"),
        )
    }

    private fun plan(p: JSONObject): EnterpriseRoutePlan {
        require(p.keys().asSequence().toSet() == setOf(
            "id", "revision", "owner_kind", "owner_id", "waypoints", "avoid_toll", "avoid_motorway", "avoid_ferry",
            "avoid_unpaved", "heavy_asset_notice", "planned_distance_metres", "planned_distance_source", "updated_at",
        ))
        val waypoints = p.getJSONArray("waypoints")
        require(waypoints.length() in 2..EnterpriseRoutePlan.MAX_WAYPOINTS)
        val planStops = List(waypoints.length()) { index ->
            val stop = waypoints.getJSONObject(index)
            val source = RouteWaypointSource.valueOf(stop.getString("source"))
            RouteWaypoint(
                stop.getInt("sequence"), stop.getString("label"), source,
                stop.textOrNull("place_id"), stop.textOrNull("station_id"), stop.textOrNull("note"),
                if (source == RouteWaypointSource.TEXT_ONLY) null else GeoPointE7(stop.getInt("lat_e7"), stop.getInt("lon_e7")),
            )
        }
        // A server estimate from an engine remains available on the company view. The app only
        // admits an engine estimate together with its actual on-device computation.
        val manual = p.textOrNull("planned_distance_source") == "MANUAL"
        return EnterpriseRoutePlan.create(
            id = p.getString("id"), revision = p.getLong("revision"),
            ownerKind = RoutePlanOwnerKind.valueOf(p.getString("owner_kind")), ownerId = p.getString("owner_id"),
            waypoints = planStops,
            options = RouteAvoidOptions(p.getBoolean("avoid_toll"), p.getBoolean("avoid_motorway"),
                p.getBoolean("avoid_ferry"), p.getBoolean("avoid_unpaved")),
            plannedDistanceMetres = if (manual) p.getLong("planned_distance_metres") else null,
            plannedDistanceSource = if (manual) PlannedDistanceSource.MANUAL else null,
            heavyAssetNotice = p.getBoolean("heavy_asset_notice"),
            updatedAt = OffsetDateTime.parse(p.getString("updated_at")).toInstant(),
        )
    }

    private fun JSONObject.count(key: String): Int = getInt(key).also { require(it >= 0) }
    private fun JSONObject.textOrNull(key: String): String? = if (isNull(key)) null else getString(key)
}
