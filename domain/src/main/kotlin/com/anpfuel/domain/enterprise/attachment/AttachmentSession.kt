package com.anpfuel.domain.enterprise.attachment

import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncFailure
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.sync.canonicalTime
import com.anpfuel.domain.enterprise.sync.exactKeys
import com.anpfuel.domain.enterprise.sync.hash
import com.anpfuel.domain.enterprise.sync.objectValue
import com.anpfuel.domain.enterprise.sync.positive
import com.anpfuel.domain.enterprise.sync.text
import com.anpfuel.domain.enterprise.sync.uuid
import com.anpfuel.domain.enterprise.sync.version
import java.time.Instant
import com.anpfuel.domain.enterprise.sync.invalid as protocolFailure

/*
 * Kotlin client of the signed attachment session (EMP-OFFL-005, B-BR-EMP-417, EDR-270). Parsing never grants
 * authority: the server declares, links and grants against current document and record permissions.
 */

/** The EMP-DOC-002 storage states, as the session and the ingress report them. */
enum class AttachmentStorage { UPLOADING, RECEIVED, STORED, QUARANTINED, FAILED, REPLACED, DELETED }

data class AttachmentRequestSnapshot(
    val requestId: String,
    val familyId: String,
    val accessToken: String,
    val membershipId: String,
    val enrollmentId: String,
    val declarationId: String,
    val documentId: String,
    val kind: String,
    val sha256: String,
    val bytes: Long,
    val mediaType: String,
    val linkId: String,
    val targetKind: String,
    val targetId: String,
) {
    override fun toString(): String = "AttachmentRequestSnapshot(redacted)"
}

/** The exact signed request body; its canonical bytes are what the device signs and sends. */
class AttachmentSessionRequest private constructor(private val value: AttachmentRequestSnapshot, raw: ByteArray) {
    private val encoded = raw.copyOf()
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    fun snapshot(): AttachmentRequestSnapshot = value
    override fun toString(): String = "AttachmentSessionRequest(redacted)"

    companion object {
        const val MAX_BYTES = 4096
        private val KEYS = setOf("schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id",
            "declaration_id", "document_id", "kind", "sha256", "bytes", "media_type", "link_id", "target_kind", "target_id")

        fun create(value: AttachmentRequestSnapshot): AttachmentSessionRequest {
            for (id in listOf(value.requestId, value.familyId, value.membershipId, value.enrollmentId, value.declarationId, value.documentId,
                value.linkId, value.targetId)) uuid(id)
            if (value.accessToken.length !in 1..512 || value.accessToken.any { it !in '!'..'~' }) throw protocolFailure()
            hash(value.sha256)
            if (value.kind != AttachmentPolicy.KIND || value.mediaType != AttachmentPolicy.MEDIA_TYPE || value.bytes !in 1..AttachmentPolicy.MAX_BYTES ||
                DocumentTargetKind.entries.none { it.name == value.targetKind } ||
                setOf(value.declarationId, value.documentId, value.linkId).size != 3) throw protocolFailure()
            val root = SyncJson.Object(mapOf("schema_version" to SyncJson.Integer("1"), "request_id" to SyncJson.Text(value.requestId),
                "family_id" to SyncJson.Text(value.familyId), "access_token" to SyncJson.Text(value.accessToken),
                "membership_id" to SyncJson.Text(value.membershipId), "enrollment_id" to SyncJson.Text(value.enrollmentId),
                "declaration_id" to SyncJson.Text(value.declarationId), "document_id" to SyncJson.Text(value.documentId),
                "kind" to SyncJson.Text(value.kind), "sha256" to SyncJson.Text(value.sha256), "bytes" to SyncJson.Integer(value.bytes.toString()),
                "media_type" to SyncJson.Text(value.mediaType), "link_id" to SyncJson.Text(value.linkId),
                "target_kind" to SyncJson.Text(value.targetKind), "target_id" to SyncJson.Text(value.targetId)))
            val bytes = CanonicalJson.encode(root)
            if (bytes.size > MAX_BYTES) throw protocolFailure(SyncFailure.LIMIT)
            return AttachmentSessionRequest(value, bytes)
        }

        fun parse(raw: ByteArray): AttachmentSessionRequest {
            val root = objectValue(CanonicalJson.parse(raw, MAX_BYTES))
            exactKeys(root, KEYS); version(root)
            val size = (root.fields["bytes"] as? SyncJson.Integer)?.lexeme?.toLongOrNull() ?: throw protocolFailure()
            return create(AttachmentRequestSnapshot(text(root, "request_id"), text(root, "family_id"), text(root, "access_token"),
                text(root, "membership_id"), text(root, "enrollment_id"), text(root, "declaration_id"), text(root, "document_id"),
                text(root, "kind"), text(root, "sha256"), size, text(root, "media_type"), text(root, "link_id"),
                text(root, "target_kind"), text(root, "target_id")))
        }
    }
}

/** A five-minute ingress capability; memory only, for one transfer (B-BR-EMP-420). */
class AttachmentUploadGrant(val capability: String, val expiresAt: Instant) {
    override fun toString(): String = "AttachmentUploadGrant(redacted)"
}

/** A complete response, bound to its request, the enrolled membership and enrollment and the declared document. */
class AttachmentSessionResponse private constructor(
    val requestId: String,
    val membershipId: String,
    val enrollmentId: String,
    val documentId: String,
    val storage: AttachmentStorage,
    val storageRevision: Long,
    val upload: AttachmentUploadGrant?,
    raw: ByteArray,
) {
    private val encoded = raw.copyOf()
    /** Canonical projection of the accepted wire (sorted keys). */
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    override fun toString(): String = "AttachmentSessionResponse(redacted)"

    companion object {
        const val MAX_BYTES = 8192
        const val MAX_CAPABILITY = 4096
        private val KEYS = setOf("schema_version", "request_id", "membership_id", "enrollment_id", "document_id", "storage", "storage_revision", "upload")

        /** The whole answer is checked before anything is exposed; a foreign one is `BINDING`. */
        fun parse(raw: ByteArray, request: AttachmentSessionRequest): AttachmentSessionResponse {
            val root = objectValue(CanonicalJson.parse(raw, MAX_BYTES))
            exactKeys(root, KEYS); version(root)
            val requestId = uuid(text(root, "request_id"))
            val membershipId = uuid(text(root, "membership_id"))
            val enrollmentId = uuid(text(root, "enrollment_id"))
            val documentId = uuid(text(root, "document_id"))
            val storage = text(root, "storage").let { raw -> AttachmentStorage.entries.firstOrNull { it.name == raw } ?: throw protocolFailure() }
            val revision = positive(root.fields["storage_revision"])
            val upload = root.fields["upload"].let { value ->
                if (value === SyncJson.Null) null else {
                    val grant = objectValue(value)
                    exactKeys(grant, setOf("capability", "expires_at"))
                    val capability = text(grant, "capability")
                    if (capability.length !in 1..MAX_CAPABILITY || capability.any { it !in '!'..'~' }) throw protocolFailure()
                    AttachmentUploadGrant(capability, canonicalTime(text(grant, "expires_at")))
                }
            }
            if (upload != null && storage != AttachmentStorage.UPLOADING) throw protocolFailure()
            val sent = request.snapshot()
            if (requestId != sent.requestId || membershipId != sent.membershipId || enrollmentId != sent.enrollmentId || documentId != sent.documentId)
                throw protocolFailure(SyncFailure.BINDING)
            val canonical = CanonicalJson.encode(SyncJson.Object(mapOf("schema_version" to SyncJson.Integer("1"),
                "request_id" to SyncJson.Text(requestId), "membership_id" to SyncJson.Text(membershipId), "enrollment_id" to SyncJson.Text(enrollmentId),
                "document_id" to SyncJson.Text(documentId), "storage" to SyncJson.Text(storage.name), "storage_revision" to SyncJson.Integer(revision.toString()),
                "upload" to (root.fields.getValue("upload")))))
            return AttachmentSessionResponse(requestId, membershipId, enrollmentId, documentId, storage, revision, upload, canonical)
        }
    }
}
