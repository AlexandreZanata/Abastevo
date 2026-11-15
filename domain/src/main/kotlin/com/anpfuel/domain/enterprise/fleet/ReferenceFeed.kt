package com.anpfuel.domain.enterprise.fleet

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncFailure
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.sync.canonicalTime
import com.anpfuel.domain.enterprise.sync.exactKeys
import com.anpfuel.domain.enterprise.sync.objectValue
import com.anpfuel.domain.enterprise.sync.text
import com.anpfuel.domain.enterprise.sync.uuid
import com.anpfuel.domain.enterprise.sync.version
import java.time.Instant
import java.util.Locale
import com.anpfuel.domain.enterprise.sync.array as jsonArray
import com.anpfuel.domain.enterprise.sync.invalid as protocolFailure

/*
 * Kotlin client of the signed reference feed (EMP-OFFL-006 wire, EMP-FLEET-013,
 * B-BR-EMP-348..352, EDR-224..227). Parsing never grants authority: a cached
 * asset is a read copy and every operation that names it is decided at intake.
 */

enum class ReferenceAssignmentKind { PRIMARY, TEMPORARY, ADDITIONAL }
enum class ReferenceMeterKind { ODOMETER, HOUR_METER }
enum class ReferenceFeedMode { DELTA, RESET }
enum class ReferenceResetReason { NEW_CURSOR, CURSOR_EXPIRED, CURSOR_AHEAD, FORBIDDEN }
enum class ReferenceChangeOp { UPSERT, DELETE }

class ReferenceMeter(val id: String, val kind: ReferenceMeterKind, val modulus: Long?, val segmentId: String, val installedAt: Instant) {
    override fun toString(): String = "ReferenceMeter(redacted)"
}

class ReferenceAssignment(val kind: ReferenceAssignmentKind, val validTo: Instant?) {
    override fun toString(): String = "ReferenceAssignment(redacted)"
}

/** The minimised asset entity of B-BR-EMP-336; nothing else about the asset reaches the device. */
class ReferenceAsset(
    val id: String,
    val revision: Long,
    val kind: AssetKind,
    val usageBasis: UsageBasis,
    val state: AssetLifecycle,
    val name: String,
    val fleetNumber: String,
    val plate: String,
    products: List<String>,
    meters: List<ReferenceMeter>,
    val assignment: ReferenceAssignment,
) {
    private val productValues = products.toList()
    private val meterValues = meters.toList()
    val products: List<String> get() = productValues.toList()
    val meters: List<ReferenceMeter> get() = meterValues.toList()
    override fun toString(): String = "ReferenceAsset(redacted)"
}

class ReferenceChange(val revision: Long, val entityId: String, val op: ReferenceChangeOp, val asset: ReferenceAsset?) {
    override fun toString(): String = "ReferenceChange(redacted)"
}

data class ReferenceFeedRequestSnapshot(val requestId: String, val familyId: String, val accessToken: String,
    val membershipId: String, val enrollmentId: String, val cursor: String?) {
    override fun toString(): String = "ReferenceFeedRequestSnapshot(redacted)"
}

/** The exact signed request body; its canonical bytes are what the device signs and sends. */
class ReferenceFeedRequest private constructor(private val value: ReferenceFeedRequestSnapshot, raw: ByteArray) {
    private val encoded = raw.copyOf()
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    fun snapshot(): ReferenceFeedRequestSnapshot = value
    override fun toString(): String = "ReferenceFeedRequest(redacted)"

    companion object {
        const val MAX_BYTES = 4096
        const val MAX_CURSOR = 1024
        private val KEYS = setOf("schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id", "cursor")

        fun create(value: ReferenceFeedRequestSnapshot): ReferenceFeedRequest {
            uuid(value.requestId); uuid(value.familyId); uuid(value.membershipId); uuid(value.enrollmentId)
            if (value.accessToken.length !in 1..512 || value.accessToken.any { it !in '!'..'~' }) throw protocolFailure()
            value.cursor?.let(::cursor)
            val root = SyncJson.Object(mapOf("schema_version" to SyncJson.Integer("1"), "request_id" to SyncJson.Text(value.requestId),
                "family_id" to SyncJson.Text(value.familyId), "access_token" to SyncJson.Text(value.accessToken),
                "membership_id" to SyncJson.Text(value.membershipId), "enrollment_id" to SyncJson.Text(value.enrollmentId),
                "cursor" to (value.cursor?.let { SyncJson.Text(it) } ?: SyncJson.Null)))
            val bytes = CanonicalJson.encode(root)
            if (bytes.size > MAX_BYTES) throw protocolFailure(SyncFailure.LIMIT)
            return ReferenceFeedRequest(value, bytes)
        }

        fun parse(raw: ByteArray): ReferenceFeedRequest {
            val root = objectValue(CanonicalJson.parse(raw, MAX_BYTES))
            exactKeys(root, KEYS); version(root)
            val cursor = root["cursor"].let { if (it === SyncJson.Null) null else (it as? SyncJson.Text)?.value ?: throw protocolFailure() }
            return create(ReferenceFeedRequestSnapshot(text(root, "request_id"), text(root, "family_id"), text(root, "access_token"),
                text(root, "membership_id"), text(root, "enrollment_id"), cursor))
        }
    }
}

/** A complete response, already bound to its request and to the revision held with the request's cursor. */
class ReferenceFeedResponse private constructor(
    val requestId: String,
    val membershipId: String,
    val enrollmentId: String,
    val mode: ReferenceFeedMode,
    val reason: ReferenceResetReason?,
    val fromRevision: Long?,
    val revision: Long,
    val confirmedAt: Instant,
    val cursor: String,
    changes: List<ReferenceChange>,
    raw: ByteArray,
) {
    private val values = changes.toList()
    private val encoded = raw.copyOf()
    val changes: List<ReferenceChange> get() = values.toList()
    /** Canonical projection of the accepted wire (sorted keys, no transport escapes). */
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    override fun toString(): String = "ReferenceFeedResponse(redacted)"

    companion object {
        /** 200 assets at their OpenAPI maxima, Go HTML escapes included, stay below this bound. */
        const val MAX_BYTES = 2 * 1024 * 1024
        const val MAX_CHANGES = 200
        private val KEYS = setOf("schema_version", "request_id", "membership_id", "enrollment_id", "mode", "reason",
            "from_revision", "revision", "confirmed_at", "cursor", "entity_types", "changes")
        private val CHANGE_KEYS = setOf("revision", "entity_type", "entity_id", "op", "asset")

        fun parse(raw: ByteArray, request: ReferenceFeedRequest, heldRevision: Long?): ReferenceFeedResponse {
            val root = objectValue(CanonicalJson.parseLarge(raw, MAX_BYTES))
            exactKeys(root, KEYS); version(root)
            val requestId = uuid(text(root, "request_id"))
            val membershipId = uuid(text(root, "membership_id"))
            val enrollmentId = uuid(text(root, "enrollment_id"))
            val mode = closed<ReferenceFeedMode>(text(root, "mode"))
            val reason = root["reason"].let { if (it === SyncJson.Null) null else closed<ReferenceResetReason>(string(it)) }
            val from = root["from_revision"].let { if (it === SyncJson.Null) null else integer(it, 0) }
            val revision = integer(root["revision"], 0)
            val confirmedAt = instant(text(root, "confirmed_at"))
            val cursor = cursor(text(root, "cursor"))
            val types = jsonArray(root, "entity_types")
            if (types.size != 1 || string(types[0]) != ENTITY_ASSET) throw protocolFailure()
            val items = jsonArray(root, "changes")
            if (items.size > MAX_CHANGES) throw protocolFailure(SyncFailure.LIMIT)
            val changes = items.map { item ->
                val change = objectValue(item)
                exactKeys(change, CHANGE_KEYS)
                if (text(change, "entity_type") != ENTITY_ASSET) throw protocolFailure()
                val entity = uuid(text(change, "entity_id"))
                val asset = change["asset"].let { if (it === SyncJson.Null) null else assetOf(objectValue(it)) }
                ReferenceChange(integer(change["revision"], 1), entity, closed(text(change, "op")), asset)
            }
            // Structure first: anything the server could never write is INVALID (B-BR-EMP-339/340).
            when (mode) {
                ReferenceFeedMode.DELTA -> if (reason != null || from == null || from > revision) throw protocolFailure()
                ReferenceFeedMode.RESET -> if (reason == null || from != null ||
                    (reason == ReferenceResetReason.FORBIDDEN && changes.isNotEmpty())) throw protocolFailure()
            }
            var prior = from ?: 0
            val entities = mutableSetOf<String>()
            for (change in changes) {
                if (change.revision <= prior || change.revision > revision || !entities.add(change.entityId)) throw protocolFailure()
                prior = change.revision
                when (change.op) {
                    ReferenceChangeOp.UPSERT -> if (change.asset?.id != change.entityId) throw protocolFailure()
                    ReferenceChangeOp.DELETE -> if (change.asset != null || mode == ReferenceFeedMode.RESET) throw protocolFailure()
                }
            }
            // Then correlation: a well-formed answer to another request or another held state is BINDING.
            val sent = request.snapshot()
            if (requestId != sent.requestId || membershipId != sent.membershipId || enrollmentId != sent.enrollmentId) throw binding()
            if (sent.cursor == null) {
                if (heldRevision != null || mode != ReferenceFeedMode.RESET ||
                    reason !in setOf(ReferenceResetReason.NEW_CURSOR, ReferenceResetReason.FORBIDDEN)) throw binding()
            } else {
                if (heldRevision == null) throw binding()
                when (mode) {
                    ReferenceFeedMode.DELTA -> if (from != heldRevision) throw binding()
                    ReferenceFeedMode.RESET -> if (reason == ReferenceResetReason.NEW_CURSOR) throw binding()
                }
            }
            return ReferenceFeedResponse(requestId, membershipId, enrollmentId, mode, reason, from, revision, confirmedAt, cursor,
                changes, CanonicalJson.encode(root))
        }
    }
}

/** The protected local copy of one organization's feed (B-BR-EMP-348); cache version 1. */
class ReferenceCache private constructor(
    val membershipId: String,
    val enrollmentId: String,
    val revision: Long,
    val cursor: String,
    val confirmedAt: Instant,
    val forbidden: Boolean,
    assets: List<ReferenceAsset>,
    val received: EnterpriseDeviceClock,
    raw: ByteArray,
) {
    private val values = assets.toList()
    private val encoded = raw.copyOf()
    /** Sorted by identifier; exactly the entities of the last applied response. */
    val assets: List<ReferenceAsset> get() = values.toList()
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    fun sameState(other: ReferenceCache?): Boolean = other != null && encoded.contentEquals(other.encoded)
    override fun toString(): String = "ReferenceCache(redacted)"

    companion object {
        const val VERSION = 1
        const val MAX_BYTES = 4 * 1024 * 1024
        private val KEYS = setOf("schema_version", "membership_id", "enrollment_id", "revision", "cursor", "confirmed_at",
            "forbidden", "assets", "received")

        private fun of(membershipId: String, enrollmentId: String, revision: Long, cursor: String, confirmedAt: Instant,
            forbidden: Boolean, assets: List<ReferenceAsset>, received: EnterpriseDeviceClock): ReferenceCache {
            if (assets.size > ReferenceFeedResponse.MAX_CHANGES) throw protocolFailure(SyncFailure.LIMIT)
            val sorted = assets.sortedBy { it.id }
            val root = SyncJson.Object(mapOf("schema_version" to SyncJson.Integer(VERSION.toString()),
                "membership_id" to SyncJson.Text(membershipId), "enrollment_id" to SyncJson.Text(enrollmentId),
                "revision" to SyncJson.Integer(revision.toString()), "cursor" to SyncJson.Text(cursor),
                "confirmed_at" to SyncJson.Text(EnterpriseTime.formatInstant(confirmedAt)), "forbidden" to SyncJson.Bool(forbidden),
                "assets" to SyncJson.Array(sorted.map(::assetJson)), "received" to SyncJson.Object(mapOf(
                    "boot" to SyncJson.Text(received.boot), "elapsed_millis" to SyncJson.Integer(received.elapsedMillis.toString()),
                    "wall_millis" to SyncJson.Integer(received.wallMillis.toString())))))
            val bytes = CanonicalJson.encode(root)
            if (bytes.size > MAX_BYTES) throw protocolFailure(SyncFailure.LIMIT)
            return ReferenceCache(membershipId, enrollmentId, revision, cursor, confirmedAt, forbidden, sorted, received, bytes)
        }

        /** Strict v1 decoding; any other version or shape is refused (read as absent by the store). */
        fun decode(raw: ByteArray): ReferenceCache {
            val root = objectValue(CanonicalJson.parseLarge(raw, MAX_BYTES))
            exactKeys(root, KEYS); version(root)
            val forbidden = (root["forbidden"] as? SyncJson.Bool)?.value ?: throw protocolFailure()
            val items = jsonArray(root, "assets")
            if (items.size > ReferenceFeedResponse.MAX_CHANGES) throw protocolFailure(SyncFailure.LIMIT)
            val assets = items.map { assetOf(objectValue(it)) }
            if (assets.zipWithNext().any { (a, b) -> a.id >= b.id } || (forbidden && assets.isNotEmpty())) throw protocolFailure()
            val clock = objectValue(root["received"])
            exactKeys(clock, setOf("boot", "elapsed_millis", "wall_millis"))
            val received = try {
                EnterpriseDeviceClock(text(clock, "boot"), integer(clock["elapsed_millis"], 0), integer(clock["wall_millis"], 0))
            } catch (_: IllegalArgumentException) { throw protocolFailure() }
            val cache = of(uuid(text(root, "membership_id")), uuid(text(root, "enrollment_id")), integer(root["revision"], 0),
                cursor(text(root, "cursor")), instant(text(root, "confirmed_at")), forbidden, assets, received)
            // Only the canonical encoding is a stored cache; anything else was not written by this codec.
            if (!cache.encoded.contentEquals(raw)) throw protocolFailure()
            return cache
        }

        /** The revision the request's cursor was held with; null without a cursor (B-BR-EMP-349). */
        fun heldRevision(prior: ReferenceCache?, request: ReferenceFeedRequest): Long? {
            val sent = request.snapshot()
            val cursor = sent.cursor ?: return null
            if (prior == null || prior.cursor != cursor || prior.membershipId != sent.membershipId ||
                prior.enrollmentId != sent.enrollmentId) throw binding()
            return prior.revision
        }

        /** Applies one bound response whole, or throws and changes nothing (B-BR-EMP-349). */
        fun apply(prior: ReferenceCache?, request: ReferenceFeedRequest, response: ReferenceFeedResponse,
            received: EnterpriseDeviceClock): ReferenceCache {
            val sent = request.snapshot()
            if (response.requestId != sent.requestId || response.membershipId != sent.membershipId ||
                response.enrollmentId != sent.enrollmentId) throw binding()
            val held = heldRevision(prior, request)
            val assets = when (response.mode) {
                ReferenceFeedMode.RESET -> response.changes.map { requireNotNull(it.asset) }
                ReferenceFeedMode.DELTA -> {
                    if (prior == null || held != response.fromRevision) throw binding()
                    val current = prior.values.associateByTo(linkedMapOf()) { it.id }
                    for (change in response.changes) when (change.op) {
                        ReferenceChangeOp.UPSERT -> current[change.entityId] = requireNotNull(change.asset)
                        // A tombstone for an entity this device never held changes nothing.
                        ReferenceChangeOp.DELETE -> current.remove(change.entityId)
                    }
                    current.values.toList()
                }
            }
            return of(sent.membershipId, sent.enrollmentId, response.revision, response.cursor, response.confirmedAt,
                response.reason == ReferenceResetReason.FORBIDDEN, assets, received)
        }
    }
}

/** One row of "Meus ativos" with the facts the screen may state (B-BR-EMP-350/351). */
class MyAsset(val asset: ReferenceAsset, val available: Boolean, val assignmentEnded: Boolean, val confirmed: Boolean) {
    /** Choosing it shows the review warning first (B-BR-EMP-352). */
    val reviewWarning: Boolean get() = !available || !confirmed
    override fun toString(): String = "MyAsset(redacted)"
}

class MyAssetsSnapshot(val confirmedAt: Instant, val listConfirmed: Boolean, val forbidden: Boolean, assets: List<MyAsset>) {
    private val values = assets.toList()
    val assets: List<MyAsset> get() = values.toList()
    override fun toString(): String = "MyAssetsSnapshot(redacted)"
}

/** What the picker returns to its calling screen; a hint, never authority (B-BR-EMP-352). */
class EnterpriseAssetChoice(val organizationId: String, val assetId: String, val assetRevision: Long, val kind: AssetKind,
    val usageBasis: UsageBasis, meters: List<ReferenceMeter>, val confirmedAt: Instant, val reviewWarningAccepted: Boolean) {
    private val meterValues = meters.toList()
    val meters: List<ReferenceMeter> get() = meterValues.toList()
    override fun toString(): String = "EnterpriseAssetChoice(redacted)"
}

object MyAssets {
    /** BR-OFFL-016 default: cached reference data is not confirmed after 24 hours without contact. */
    const val CONFIRMATION_MILLIS = 86_400_000L

    /** Device-clock age of a commit: elapsed time within one boot, wall time after a reboot. */
    fun confirmed(received: EnterpriseDeviceClock, now: EnterpriseDeviceClock): Boolean {
        val age = if (now.boot == received.boot) now.elapsedMillis - received.elapsedMillis else now.wallMillis - received.wallMillis
        return age in 0 until CONFIRMATION_MILLIS
    }

    fun present(cache: ReferenceCache, now: EnterpriseDeviceClock): MyAssetsSnapshot {
        val listConfirmed = confirmed(cache.received, now)
        val rows = cache.assets.map { asset ->
            val ended = asset.assignment.validTo?.let { now.wallMillis >= it.toEpochMilli() } ?: false
            MyAsset(asset, asset.state == AssetLifecycle.ACTIVE, ended, listConfirmed && !ended)
        }.sortedWith(compareBy({ it.asset.name.lowercase(Locale.ROOT) }, { it.asset.fleetNumber }, { it.asset.id }))
        return MyAssetsSnapshot(cache.confirmedAt, listConfirmed, cache.forbidden, rows)
    }

    /** Null when the asset is not in the snapshot or its warning was not accepted. */
    fun choose(organizationId: String, snapshot: MyAssetsSnapshot, assetId: String, warningAccepted: Boolean): EnterpriseAssetChoice? {
        EnterprisePartition.requireId(organizationId)
        val row = snapshot.assets.firstOrNull { it.asset.id == assetId } ?: return null
        if (row.reviewWarning && !warningAccepted) return null
        val asset = row.asset
        return EnterpriseAssetChoice(organizationId, asset.id, asset.revision, asset.kind, asset.usageBasis, asset.meters,
            snapshot.confirmedAt, row.reviewWarning)
    }
}

private const val ENTITY_ASSET = "asset"
private const val CATALOG = "fuel-products-v1"
private val GO_ZERO: Instant = Instant.parse("0001-01-01T00:00:00Z")
private val ASSET_KEYS = setOf("id", "revision", "kind", "usage_basis", "state", "name", "fleet_number", "plate", "catalog",
    "products", "meters", "assignment")
private val METER_KEYS = setOf("id", "kind", "modulus", "segment_id", "installed_at")

private fun binding() = protocolFailure(SyncFailure.BINDING)
private fun string(value: SyncJson?): String = (value as? SyncJson.Text)?.value ?: throw protocolFailure()
private inline fun <reified T : Enum<T>> closed(raw: String): T = enumValues<T>().firstOrNull { it.name == raw } ?: throw protocolFailure()
private fun integer(value: SyncJson?, minimum: Long): Long {
    val number = (value as? SyncJson.Integer)?.lexeme?.toLongOrNull() ?: throw protocolFailure()
    if (number < minimum) throw protocolFailure()
    return number
}
private fun bounded(value: String, maxCodePoints: Int): String {
    if (value.codePointCount(0, value.length) > maxCodePoints) throw protocolFailure()
    return value
}
private fun cursor(value: String): String {
    if (value.length !in 1..ReferenceFeedRequest.MAX_CURSOR || value.any { it !in '!'..'~' }) throw protocolFailure()
    return value
}
private fun instant(raw: String): Instant = canonicalTime(raw).also { if (it == GO_ZERO) throw protocolFailure() }

private fun assetOf(root: SyncJson.Object): ReferenceAsset {
    exactKeys(root, ASSET_KEYS)
    if (text(root, "catalog") != CATALOG) throw protocolFailure()
    val products = jsonArray(root, "products")
    if (products.size > 64) throw protocolFailure()
    val meters = jsonArray(root, "meters")
    if (meters.size > 16) throw protocolFailure()
    val assignment = objectValue(root["assignment"])
    exactKeys(assignment, setOf("kind", "valid_to"))
    return ReferenceAsset(uuid(text(root, "id")), integer(root["revision"], 1), closed(text(root, "kind")),
        closed(text(root, "usage_basis")), closed(text(root, "state")), bounded(text(root, "name"), 80),
        bounded(text(root, "fleet_number"), 40), bounded(text(root, "plate"), 40),
        products.map { bounded(string(it), 64) },
        meters.map { item ->
            val meter = objectValue(item)
            exactKeys(meter, METER_KEYS)
            ReferenceMeter(uuid(text(meter, "id")), closed(text(meter, "kind")),
                meter["modulus"].let { if (it === SyncJson.Null) null else integer(it, 1) },
                uuid(text(meter, "segment_id")), instant(text(meter, "installed_at")))
        },
        ReferenceAssignment(closed(text(assignment, "kind")),
            assignment["valid_to"].let { if (it === SyncJson.Null) null else instant(string(it)) }))
}

private fun assetJson(asset: ReferenceAsset): SyncJson = SyncJson.Object(mapOf(
    "id" to SyncJson.Text(asset.id), "revision" to SyncJson.Integer(asset.revision.toString()),
    "kind" to SyncJson.Text(asset.kind.name), "usage_basis" to SyncJson.Text(asset.usageBasis.name),
    "state" to SyncJson.Text(asset.state.name), "name" to SyncJson.Text(asset.name),
    "fleet_number" to SyncJson.Text(asset.fleetNumber), "plate" to SyncJson.Text(asset.plate),
    "catalog" to SyncJson.Text(CATALOG), "products" to SyncJson.Array(asset.products.map { SyncJson.Text(it) }),
    "meters" to SyncJson.Array(asset.meters.map { meter -> SyncJson.Object(mapOf(
        "id" to SyncJson.Text(meter.id), "kind" to SyncJson.Text(meter.kind.name),
        "modulus" to (meter.modulus?.let { SyncJson.Integer(it.toString()) } ?: SyncJson.Null),
        "segment_id" to SyncJson.Text(meter.segmentId),
        "installed_at" to SyncJson.Text(EnterpriseTime.formatInstant(meter.installedAt)))) }),
    "assignment" to SyncJson.Object(mapOf("kind" to SyncJson.Text(asset.assignment.kind.name),
        "valid_to" to (asset.assignment.validTo?.let { SyncJson.Text(EnterpriseTime.formatInstant(it)) } ?: SyncJson.Null))),
))
