package com.anpfuel.domain.enterprise.maintenance.checklist

import java.time.Instant

/** EMP-CHECK-001 (B-BR-EMP-330..334, EDR-213): the pure checklist rule shared with Go through checklist-v1. */
enum class ChecklistFailure { INVALID, ANSWER_NOT_ALLOWED, PHOTO_REQUIRED, INCOMPLETE }

class ChecklistDomainException(val failure: ChecklistFailure) : IllegalArgumentException("checklist: ${failure.name.lowercase()}")

private fun invalid() = ChecklistDomainException(ChecklistFailure.INVALID)

enum class ChecklistContext {
    DEPARTURE, RETURN, AD_HOC;
    companion object { fun parse(raw: String): ChecklistContext = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}

enum class TargetKind {
    ORGANIZATION, CATEGORIES, ASSETS;
    companion object { fun parse(raw: String): TargetKind = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}

enum class PolicyMode {
    BLOCK, WARN;
    companion object { fun parse(raw: String): PolicyMode = entries.firstOrNull { it.name == raw } ?: throw invalid() }
}

/** The organization default (no identifier) or 1..100 distinct canonical identifiers. */
data class ChecklistTarget(val kind: String, val ids: List<String>?)

/** One ordered question; its identity is its 1-based position. */
data class ChecklistItem(val label: String, val critical: Boolean = false, val optional: Boolean = false, val photoOnNotOk: Boolean = false)

/** The validated, canonical content of one template version (target identifiers sorted). */
class ChecklistDefinition private constructor(val name: String, val context: ChecklistContext, val targetKind: TargetKind, val targetIds: List<String>, val items: List<ChecklistItem>) {
    companion object {
        const val MAX_ITEMS = 40
        const val MAX_TARGETS = 100
        const val MAX_NAME = 80
        const val MAX_LABEL = 120
        const val MAX_NOTE = 140
        fun of(name: String, context: String, target: ChecklistTarget, items: List<ChecklistItem>): ChecklistDefinition {
            if (!validText(name, MAX_NAME, true)) throw invalid()
            val parsed = ChecklistContext.parse(context)
            val (kind, ids) = canonicalTarget(target)
            return ChecklistDefinition(name, parsed, kind, ids, validItems(items))
        }

        internal fun canonicalTarget(target: ChecklistTarget): Pair<TargetKind, List<String>> {
            val kind = TargetKind.parse(target.kind)
            val ids = target.ids.orEmpty()
            if (kind == TargetKind.ORGANIZATION) {
                if (ids.isNotEmpty()) throw invalid()
                return kind to emptyList()
            }
            if (ids.isEmpty() || ids.size > MAX_TARGETS || ids.any { !Checklist.canonicalId(it) } || ids.toSet().size != ids.size) throw invalid()
            return kind to ids.sorted()
        }

        internal fun validItems(items: List<ChecklistItem>): List<ChecklistItem> {
            if (items.isEmpty() || items.size > MAX_ITEMS) throw invalid()
            if (items.any { !validText(it.label, MAX_LABEL, true) || it.critical && it.optional }) throw invalid()
            return items.toList()
        }
    }
}

/** Valid UTF-16 (no lone surrogate), at most [maximum] scalars, no C0/C1 control; a required or non-empty text says something besides white space. */
internal fun validText(raw: String, maximum: Int, required: Boolean): Boolean {
    var says = false
    var scalars = 0
    var index = 0
    while (index < raw.length) {
        val point = raw.codePointAt(index)
        if (point in 0xd800..0xdfff || Character.isISOControl(point)) return false
        if (!Character.isWhitespace(point) && !Character.isSpaceChar(point)) says = true
        scalars++
        index += Character.charCount(point)
    }
    return scalars <= maximum && (says || !required && raw.isEmpty())
}

/** A critical-item policy value: BLOCK (default) or WARN and the run validity in seconds. */
class ChecklistPolicy private constructor(val mode: PolicyMode, val validitySeconds: Long) {
    companion object {
        const val MIN_VALIDITY = 3_600L
        const val MAX_VALIDITY = 259_200L
        const val DEFAULT_VALIDITY = 43_200L
        fun of(mode: String, validitySeconds: Long): ChecklistPolicy {
            val parsed = PolicyMode.parse(mode)
            if (validitySeconds !in MIN_VALIDITY..MAX_VALIDITY) throw invalid()
            return ChecklistPolicy(parsed, validitySeconds)
        }
        fun default(): ChecklistPolicy = ChecklistPolicy(PolicyMode.BLOCK, DEFAULT_VALIDITY)
    }
}

/** Raw answer values; null is unanswered. Other text is malformed input. */
object ChecklistAnswer {
    const val OK = "OK"
    const val NOT_OK = "NOT_OK"
    const val NOT_APPLICABLE = "NOT_APPLICABLE"
}

/** One entry of an answer set; the note is private field text and never rendered. */
class ChecklistResponse(val answer: String?, val note: String = "", val photo: Boolean = false) {
    override fun toString(): String = "ChecklistResponse(redacted)"
}

enum class ChecklistOutcome { COMPLETED_OK, COMPLETED_WITH_ISSUES, BLOCKING }

data class ChecklistEvaluation(val outcome: ChecklistOutcome, val issues: List<Int>, val critical: List<Int>, val acknowledgementRequired: Boolean)

data class ChecklistCandidate(val templateId: String, val context: String, val target: ChecklistTarget)

enum class ChecklistMatch { ASSET, CATEGORY, ORGANIZATION, NONE }

data class ChecklistResolution(val templateId: String?, val match: ChecklistMatch)

data class ChecklistPolicyRevision(val revision: Long, val mode: String, val validitySeconds: Long, val effectiveAt: Instant)

object Checklist {
    private val minInstant: Instant = Instant.parse("0001-01-01T00:00:00Z")
    private val maxInstant: Instant = Instant.parse("9999-12-31T23:59:59.999999999Z")

    fun evaluate(items: List<ChecklistItem>, mode: String, validitySeconds: Long, responses: List<ChecklistResponse>): ChecklistEvaluation {
        ChecklistDefinition.validItems(items)
        val policy = ChecklistPolicy.of(mode, validitySeconds)
        if (responses.size != items.size) throw invalid()
        val known = setOf(null, ChecklistAnswer.OK, ChecklistAnswer.NOT_OK, ChecklistAnswer.NOT_APPLICABLE)
        if (responses.any { it.answer !in known || !validText(it.note, ChecklistDefinition.MAX_NOTE, false) }) throw invalid()
        var notAllowed = false
        var photo = false
        var incomplete = false
        val issues = mutableListOf<Int>()
        val critical = mutableListOf<Int>()
        responses.forEachIndexed { i, r ->
            val item = items[i]
            when (r.answer) {
                ChecklistAnswer.NOT_APPLICABLE -> if (!item.optional) notAllowed = true
                ChecklistAnswer.NOT_OK -> {
                    if (item.photoOnNotOk && !r.photo) photo = true
                    issues += i + 1
                    if (item.critical) critical += i + 1
                }
                null -> incomplete = true
            }
        }
        when {
            notAllowed -> throw ChecklistDomainException(ChecklistFailure.ANSWER_NOT_ALLOWED)
            photo -> throw ChecklistDomainException(ChecklistFailure.PHOTO_REQUIRED)
            incomplete -> throw ChecklistDomainException(ChecklistFailure.INCOMPLETE)
        }
        val outcome = when {
            issues.isEmpty() -> ChecklistOutcome.COMPLETED_OK
            critical.isNotEmpty() && policy.mode == PolicyMode.BLOCK -> ChecklistOutcome.BLOCKING
            else -> ChecklistOutcome.COMPLETED_WITH_ISSUES
        }
        return ChecklistEvaluation(outcome, issues.toList(), critical.toList(), outcome == ChecklistOutcome.COMPLETED_WITH_ISSUES && critical.isNotEmpty())
    }

    fun resolve(candidates: List<ChecklistCandidate>, context: String, assetId: String, categoryId: String): ChecklistResolution {
        val wanted = ChecklistContext.parse(context)
        if (!canonicalId(assetId) || !canonicalId(categoryId)) throw invalid()
        val templates = mutableSetOf<String>()
        val named = mutableMapOf<Triple<ChecklistContext, TargetKind, String>, String>()
        for (c in candidates) {
            if (!canonicalId(c.templateId) || !templates.add(c.templateId)) throw invalid()
            val ctx = ChecklistContext.parse(c.context)
            val (kind, ids) = ChecklistDefinition.canonicalTarget(c.target)
            val keys = if (kind == TargetKind.ORGANIZATION) listOf(Triple(ctx, kind, "")) else ids.map { Triple(ctx, kind, it) }
            for (key in keys) if (named.putIfAbsent(key, c.templateId) != null) throw invalid()
        }
        named[Triple(wanted, TargetKind.ASSETS, assetId)]?.let { return ChecklistResolution(it, ChecklistMatch.ASSET) }
        named[Triple(wanted, TargetKind.CATEGORIES, categoryId)]?.let { return ChecklistResolution(it, ChecklistMatch.CATEGORY) }
        named[Triple(wanted, TargetKind.ORGANIZATION, "")]?.let { return ChecklistResolution(it, ChecklistMatch.ORGANIZATION) }
        return ChecklistResolution(null, ChecklistMatch.NONE)
    }

    /** The revision in force at the instant, or null when none is. */
    fun policyAt(history: List<ChecklistPolicyRevision>, at: Instant): ChecklistPolicyRevision? {
        if (!calendar(at)) throw invalid()
        var found: ChecklistPolicyRevision? = null
        history.forEachIndexed { i, r ->
            ChecklistPolicy.of(r.mode, r.validitySeconds)
            if (r.revision != (i + 1).toLong() || !calendar(r.effectiveAt) || i > 0 && r.effectiveAt.isBefore(history[i - 1].effectiveAt)) throw invalid()
            if (!r.effectiveAt.isAfter(at)) found = r
        }
        return found
    }

    private fun calendar(at: Instant) = !at.isBefore(minInstant) && !at.isAfter(maxInstant)

    fun canonicalId(raw: String): Boolean {
        if (raw.length != 36 || raw == "00000000-0000-0000-0000-000000000000") return false
        return raw.withIndex().all { (i, c) -> if (i == 8 || i == 13 || i == 18 || i == 23) c == '-' else c in '0'..'9' || c in 'a'..'f' }
    }
}
