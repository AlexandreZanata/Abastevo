package com.anpfuel.domain.enterprise.sync

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import java.security.MessageDigest
import java.time.ZoneId

/** Independently supplied local partition; values alone never prove live server authority. */
class SyncBinding private constructor(val ownerAccountId: String, val apiOrigin: String, val organizationId: String) {
    override fun toString(): String = "SyncBinding(redacted)"
    companion object {
        fun of(account: String, origin: String, organization: String): SyncBinding =
            try { SyncBinding(uuid(account), canonicalOrigin(origin), uuid(organization)) } catch (_: SyncProtocolException) { throw invalid(SyncFailure.BINDING) }
    }
}

class SyncEntityRef internal constructor(val type: String, val id: String) {
    override fun toString(): String = "SyncEntityRef(redacted)"
}
class SyncEntity internal constructor(val reference: SyncEntityRef, val expectedRevision: Long?) {
    override fun toString(): String = "SyncEntity(redacted)"
}
class SyncAttachment internal constructor(val id: String, val sha256: String, val bytes: Long, val mime: String) {
    override fun toString(): String = "SyncAttachment(redacted)"
}

/** Complete immutable v1 intent. Delivery family, credentials and receipts are not this value. */
class EnterpriseOperation private constructor(
    val operationId: String,
    val binding: SyncBinding,
    val membershipId: String,
    val enrollmentId: String,
    val type: String,
    val operationSchema: Long,
    val entity: SyncEntity,
    val declaration: TimeDeclaration,
    dependencies: List<String>,
    references: List<SyncEntityRef>,
    attachments: List<SyncAttachment>,
    val payload: SyncJson.Object,
    val payloadDigest: String,
    canonical: ByteArray,
) {
    private val dependencyValues = dependencies.toList()
    private val referenceValues = references.toList()
    private val attachmentValues = attachments.toList()
    private val encoded = canonical.copyOf()
    val dependsOn: List<String> get() = dependencyValues.toList()
    val softReferences: List<SyncEntityRef> get() = referenceValues.toList()
    val attachments: List<SyncAttachment> get() = attachmentValues.toList()
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    val supportsSchema: Boolean get() = operationSchema == 1L

    fun sameIntent(other: EnterpriseOperation): Boolean {
        if (operationId != other.operationId) return false
        if (payloadDigest != other.payloadDigest) throw invalid(SyncFailure.REUSED)
        return true
    }
    override fun toString(): String = "EnterpriseOperation(redacted)"

    companion object {
        private val keys = setOf("schema_version", "operation_id", "owner_account_id", "api_origin", "org_id", "membership_id", "enrollment_id", "type", "operation_schema", "entity", "occurred_at", "entry_zone", "device_offset_seconds", "depends_on", "soft_references", "attachments", "payload")
        fun seal(raw: ByteArray, binding: SyncBinding, limits: SyncLimits = SyncLimits.DEFAULT): EnterpriseOperation = decode(raw, binding, false, limits)
        fun parse(raw: ByteArray, binding: SyncBinding, limits: SyncLimits = SyncLimits.DEFAULT): EnterpriseOperation = decode(raw, binding, true, limits)

        private fun decode(raw: ByteArray, binding: SyncBinding, withDigest: Boolean, limits: SyncLimits): EnterpriseOperation {
            val root = objectValue(CanonicalJson.parse(raw, limits.maxOperationBytes, limits.maxOperationDepth))
            exactKeys(root, if (withDigest) keys + "payload_digest" else keys)
            version(root)
            val operationId = uuid(text(root, "operation_id"))
            val owner = uuid(text(root, "owner_account_id"))
            val origin = text(root, "api_origin")
            if (canonicalOrigin(origin) != origin) throw invalid()
            val organization = uuid(text(root, "org_id"))
            if (owner != binding.ownerAccountId || origin != binding.apiOrigin || organization != binding.organizationId) throw invalid(SyncFailure.BINDING)
            val membership = uuid(text(root, "membership_id"))
            val enrollment = uuid(text(root, "enrollment_id"))
            val type = text(root, "type").also { if (!operationType(it)) throw invalid() }
            val schema = positive(root["operation_schema"])
            val entityValue = objectValue(root["entity"])
            exactKeys(entityValue, setOf("type", "id", "expected_revision"))
            val entity = SyncEntity(entityRef(entityValue), nullablePositive(entityValue["expected_revision"]))
            val declaration = declaration(root)
            val dependencies = array(root, "depends_on").map { uuid(string(it)) }.sorted()
            if (dependencies.toSet().size != dependencies.size || operationId in dependencies) throw invalid()
            val references = array(root, "soft_references").map {
                val value = objectValue(it)
                exactKeys(value, setOf("type", "id"))
                entityRef(value)
            }.sortedWith(compareBy({ it.type }, { it.id }))
            if (references.map { it.type to it.id }.toSet().size != references.size) throw invalid()
            val attachments = array(root, "attachments").map {
                val value = objectValue(it)
                exactKeys(value, setOf("id", "sha256", "bytes", "mime"))
                SyncAttachment(uuid(text(value, "id")), text(value, "sha256").also { hash(it) }, positive(value["bytes"]), text(value, "mime").also { if (!mime.matches(it)) throw invalid() })
            }.sortedBy { it.id }
            if (attachments.map { it.id }.toSet().size != attachments.size) throw invalid()
            val payload = objectValue(root["payload"])
            val normalized = root.fields.toMutableMap().apply {
                remove("payload_digest")
                put("depends_on", SyncJson.Array(dependencies.map { SyncJson.Text(it) }))
                put("soft_references", SyncJson.Array(references.map { SyncJson.Object(mapOf("type" to SyncJson.Text(it.type), "id" to SyncJson.Text(it.id))) }))
                put("attachments", SyncJson.Array(attachments.map { SyncJson.Object(mapOf("id" to SyncJson.Text(it.id), "sha256" to SyncJson.Text(it.sha256), "bytes" to SyncJson.Integer(it.bytes.toString()), "mime" to SyncJson.Text(it.mime))) }))
            }
            val computedDigest = "sha256:" + MessageDigest.getInstance("SHA-256").digest(CanonicalJson.encode(SyncJson.Object(normalized), limits.maxOperationDepth)).joinToString("") { "%02x".format(it.toInt() and 255) }
            if (withDigest) {
                val supplied = text(root, "payload_digest")
                digest(supplied)
                if (supplied != computedDigest) throw invalid(SyncFailure.DIGEST)
            }
            normalized["payload_digest"] = SyncJson.Text(computedDigest)
            val canonical = CanonicalJson.encode(SyncJson.Object(normalized), limits.maxOperationDepth)
            if (canonical.size > limits.maxOperationBytes) throw invalid(SyncFailure.LIMIT)
            return EnterpriseOperation(operationId, binding, membership, enrollment, type, schema, entity, declaration, dependencies, references, attachments, payload, computedDigest, canonical)
        }
    }
}

private val mime = Regex("[a-z0-9][a-z0-9!#$&^_.+\\-]*/[a-z0-9][a-z0-9!#$&^_.+\\-]*")
internal fun uuid(raw: String): String = try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { throw invalid() }
internal fun objectValue(value: SyncJson?): SyncJson.Object = value as? SyncJson.Object ?: throw invalid()
internal fun string(value: SyncJson?): String = (value as? SyncJson.Text)?.value ?: throw invalid()
internal fun text(value: SyncJson.Object, key: String): String = string(value[key])
internal fun array(value: SyncJson.Object, key: String): List<SyncJson> = (value[key] as? SyncJson.Array)?.items() ?: throw invalid()
internal fun exactKeys(value: SyncJson.Object, keys: Set<String>) { if (value.entries().map { it.key }.toSet() != keys) throw invalid() }
internal fun version(value: SyncJson.Object) { if ((value["schema_version"] as? SyncJson.Integer)?.lexeme != "1") throw invalid() }
internal fun positive(value: SyncJson?): Long {
    val number = (value as? SyncJson.Integer)?.lexeme?.toLongOrNull() ?: throw invalid()
    if (number < 1) throw invalid()
    return number
}
internal fun nullablePositive(value: SyncJson?): Long? = if (value === SyncJson.Null) null else positive(value)
internal fun hash(value: String) { if (value.length != 64 || value.any { it !in '0'..'9' && it !in 'a'..'f' }) throw invalid() }
internal fun digest(value: String) { if (!value.startsWith("sha256:")) throw invalid(); hash(value.substring(7)) }
internal fun entityRef(value: SyncJson.Object): SyncEntityRef = SyncEntityRef(text(value, "type").also { if (!segmented(it)) throw invalid() }, uuid(text(value, "id")))
internal fun segmented(value: String): Boolean {
    if (value.isEmpty() || value[0] !in 'a'..'z') return false
    var afterUnderscore = false
    for (character in value) {
        if (character == '_') { if (afterUnderscore) return false; afterUnderscore = true }
        else { if (character !in 'a'..'z' && character !in '0'..'9') return false; afterUnderscore = false }
    }
    return !afterUnderscore
}
private fun operationType(value: String): Boolean {
    val segments = value.split('.')
    return segments.size >= 2 && segments.all(::segmented)
}
internal fun canonicalTime(raw: String): java.time.Instant {
    val instant = try { EnterpriseTime.parseInstant(raw) } catch (_: IllegalArgumentException) { throw invalid() }
    if (EnterpriseTime.formatInstant(instant) != raw) throw invalid()
    return instant
}
private fun declaration(value: SyncJson.Object): TimeDeclaration {
    val occurredAt = canonicalTime(text(value, "occurred_at"))
    val zone = text(value, "entry_zone")
    if (zone.length !in 1..128 || zone.any { it !in 'A'..'Z' && it !in 'a'..'z' && it !in '0'..'9' && it !in "_+/-" } ||
        (zone != "UTC" && '/' !in zone) || "//" in zone || zone.startsWith("posix/") || zone.startsWith("right/") || zone.startsWith("SystemV/") || zone !in ZoneId.getAvailableZoneIds()) throw invalid()
    val offset = (value["device_offset_seconds"] as? SyncJson.Integer)?.lexeme?.toLongOrNull() ?: throw invalid()
    if (offset !in -64800..64800) throw invalid()
    return TimeDeclaration(occurredAt, zone, offset)
}
