package com.anpfuel.domain.enterprise.sync

data class ReviewSelector(val operationId: String, val payloadDigest: String) {
    override fun toString(): String = "ReviewSelector(redacted)"
}
data class ReviewExpectation(val selector: ReviewSelector, val reviewItemId: String) {
    override fun toString(): String = "ReviewExpectation(redacted)"
}
class ReviewSourceBinding(val binding: SyncBinding, val membershipId: String, val enrollmentId: String) {
    override fun toString(): String = "ReviewSourceBinding(redacted)"
}
data class ReviewResultsRequestSnapshot(val requestId: String, val familyId: String, val accessToken: String,
    val membershipId: String, val enrollmentId: String, val selectors: List<ReviewSelector>) {
    override fun toString(): String = "ReviewResultsRequestSnapshot(redacted)"
}
class ReviewResultsRequest private constructor(private val value: ReviewResultsRequestSnapshot, raw: ByteArray) {
    private val encoded = raw.copyOf()
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    fun snapshot(): ReviewResultsRequestSnapshot = value.copy(selectors = value.selectors.toList())
    fun validateSource(source: ReviewSourceBinding) {
        try { uuid(source.membershipId); uuid(source.enrollmentId) } catch (_: SyncProtocolException) { throw invalid(SyncFailure.BINDING) }
        if (value.membershipId != source.membershipId || value.enrollmentId != source.enrollmentId) throw invalid(SyncFailure.BINDING)
    }
    override fun toString(): String = "ReviewResultsRequest(redacted)"
    companion object {
        const val MAX_BYTES = 65536
        const val MAX_SELECTORS = 50
        fun create(value: ReviewResultsRequestSnapshot): ReviewResultsRequest {
            uuid(value.requestId); uuid(value.familyId); uuid(value.membershipId); uuid(value.enrollmentId)
            if (value.accessToken.length !in 1..512 || value.accessToken.any { it !in '!'..'~' }) throw invalid()
            val selectors = value.selectors.toList()
            if (selectors.size > MAX_SELECTORS) throw invalid(SyncFailure.LIMIT)
            val seen = mutableSetOf<String>()
            for (selector in selectors) { checkSelector(selector); if (!seen.add(selector.operationId)) throw invalid() }
            val root = SyncJson.Object(mapOf("schema_version" to SyncJson.Integer("1"), "request_id" to SyncJson.Text(value.requestId),
                "family_id" to SyncJson.Text(value.familyId), "access_token" to SyncJson.Text(value.accessToken),
                "membership_id" to SyncJson.Text(value.membershipId), "enrollment_id" to SyncJson.Text(value.enrollmentId),
                "selectors" to SyncJson.Array(selectors.map(::selectorJson))))
            val bytes = CanonicalJson.encode(root)
            if (bytes.size > MAX_BYTES) throw invalid(SyncFailure.LIMIT)
            return ReviewResultsRequest(value.copy(selectors = selectors), bytes)
        }
        fun parse(raw: ByteArray): ReviewResultsRequest {
            val root = objectValue(CanonicalJson.parse(raw, MAX_BYTES))
            exactKeys(root, setOf("schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id", "selectors")); version(root)
            val selectors = array(root, "selectors")
            if (selectors.size > MAX_SELECTORS) throw invalid(SyncFailure.LIMIT)
            return create(ReviewResultsRequestSnapshot(text(root, "request_id"), text(root, "family_id"), text(root, "access_token"),
                text(root, "membership_id"), text(root, "enrollment_id"), selectors.map {
                    val item = objectValue(it); exactKeys(item, setOf("operation_id", "payload_digest")); selectorValue(item)
                }))
        }
    }
}
data class ReviewResult(val selector: ReviewSelector, val resolution: ReviewResolution?) {
    override fun toString(): String = "ReviewResult(redacted)"
}
class ReviewResultsResponse private constructor(results: List<ReviewResult>, raw: ByteArray) {
    private val values = results.toList()
    private val encoded = raw.copyOf()
    val results: List<ReviewResult> get() = values.toList()
    val canonicalBytes: ByteArray get() = encoded.copyOf()
    override fun toString(): String = "ReviewResultsResponse(redacted)"
    companion object {
        const val MAX_BYTES = 131072
        fun create(request: ReviewResultsRequest, source: ReviewSourceBinding, results: List<ReviewResult>): ReviewResultsResponse {
            request.validateSource(source)
            val expected = request.snapshot()
            val values = results.toList()
            if (values.size != expected.selectors.size) throw invalid(SyncFailure.BINDING)
            val items = values.mapIndexed { index, result ->
                matches(result.selector, expected.selectors[index])
                val resolution = result.resolution?.let {
                    val v = it.snapshot(); matches(ReviewSelector(v.operationId, v.payloadDigest), result.selector)
                    CanonicalJson.parse(it.canonicalBytes, ReviewResolution.MAX_BYTES)
                } ?: SyncJson.Null
                SyncJson.Object(selectorJson(result.selector).fields + ("resolution" to resolution))
            }
            val root = SyncJson.Object(mapOf("schema_version" to SyncJson.Integer("1"), "request_id" to SyncJson.Text(expected.requestId),
                "owner_account_id" to SyncJson.Text(source.binding.ownerAccountId), "api_origin" to SyncJson.Text(source.binding.apiOrigin),
                "org_id" to SyncJson.Text(source.binding.organizationId), "membership_id" to SyncJson.Text(source.membershipId),
                "enrollment_id" to SyncJson.Text(source.enrollmentId), "results" to SyncJson.Array(items)))
            val bytes = CanonicalJson.encode(root)
            if (bytes.size > MAX_BYTES) throw invalid(SyncFailure.LIMIT)
            return ReviewResultsResponse(values, bytes)
        }
        fun parse(raw: ByteArray, request: ReviewResultsRequest, source: ReviewSourceBinding,
            expected: List<ReviewExpectation>): ReviewResultsResponse {
            request.validateSource(source)
            val targets = expected.toList(); val captured = request.snapshot()
            if (targets.size != captured.selectors.size) throw invalid(SyncFailure.BINDING)
            targets.forEachIndexed { index, target ->
                matches(target.selector, captured.selectors[index])
                try { uuid(target.reviewItemId) } catch (_: SyncProtocolException) { throw invalid(SyncFailure.BINDING) }
            }
            val node = CanonicalJson.parseReviewResultsSpanned(raw)
            val root = objectValue(node.value)
            exactKeys(root, setOf("schema_version", "request_id", "owner_account_id", "api_origin", "org_id", "membership_id", "enrollment_id", "results")); version(root)
            for ((key, want) in mapOf("request_id" to captured.requestId, "owner_account_id" to source.binding.ownerAccountId,
                "api_origin" to source.binding.apiOrigin, "org_id" to source.binding.organizationId,
                "membership_id" to source.membershipId, "enrollment_id" to source.enrollmentId)) {
                if (text(root, key) != want) throw invalid(SyncFailure.BINDING)
            }
            val array = array(root, "results")
            if (array.size != targets.size) throw invalid(SyncFailure.BINDING)
            val resultNode = node.children.first { it.value === root["results"] }
            val decoded = String(raw, Charsets.UTF_8) // Strict decoding already succeeded.
            val values = array.mapIndexed { index, value ->
                val item = objectValue(value); exactKeys(item, setOf("operation_id", "payload_digest", "resolution"))
                val selector = selectorValue(item); matches(selector, targets[index].selector)
                val resolution = if (item["resolution"] === SyncJson.Null) null else {
                    val span = resultNode.children[index].children.first { it.value === item["resolution"] }
                    if (decoded.substring(span.start, span.end).toByteArray(Charsets.UTF_8).size > ReviewResolution.MAX_BYTES) throw invalid(SyncFailure.LIMIT)
                    ReviewResolution.decode(objectValue(item["resolution"])).also {
                        val v = it.snapshot(); matches(ReviewSelector(v.operationId, v.payloadDigest), selector)
                        if (v.reviewItemId != targets[index].reviewItemId) throw invalid(SyncFailure.BINDING)
                    }
                }
                ReviewResult(selector, resolution)
            }
            return create(request, source, values)
        }
    }
}

private fun checkSelector(value: ReviewSelector) { uuid(value.operationId); digest(value.payloadDigest) }
private fun selectorJson(value: ReviewSelector) = SyncJson.Object(mapOf("operation_id" to SyncJson.Text(value.operationId), "payload_digest" to SyncJson.Text(value.payloadDigest)))
private fun selectorValue(root: SyncJson.Object): ReviewSelector = ReviewSelector(uuid(text(root, "operation_id")), text(root, "payload_digest").also(::digest))
private fun matches(value: ReviewSelector, expected: ReviewSelector) {
    if (value.operationId != expected.operationId) throw invalid(SyncFailure.BINDING)
    if (value.payloadDigest != expected.payloadDigest) throw invalid(SyncFailure.DIGEST)
}
