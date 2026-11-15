package com.anpfuel.domain.enterprise.document

import com.anpfuel.domain.enterprise.AllocationLine
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseValues
import com.anpfuel.domain.premium.PremiumBounds
import java.time.Instant
import java.util.Collections

private val digestPattern = Regex("^[0-9a-f]{64}$")
private val minimumInstant = Instant.parse("0001-01-01T00:00:00Z")
private val maximumInstant = Instant.parse("9999-12-31T23:59:59.999999999Z")

private fun identifier(raw: String) { try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { fail() } }
private fun organization(raw: String) {
    try { EnterprisePartition.requireId(raw) } catch (_: IllegalArgumentException) { fail(DocumentFailure.SCOPE) }
}
private fun <T> frozen(values: List<T>): List<T> = Collections.unmodifiableList(ArrayList(values))

/** A same-organization link target. Its shape proves neither existence nor read authority. */
class DocumentTarget(val organizationId: String, val kind: DocumentTargetKind, val id: String) {
    init { organization(organizationId); identifier(id) }
    override fun equals(other: Any?): Boolean =
        other is DocumentTarget && organizationId == other.organizationId && kind == other.kind && id == other.id
    override fun hashCode(): Int = 31 * (31 * organizationId.hashCode() + kind.hashCode()) + id.hashCode()
    override fun toString(): String = "EnterpriseDocumentTarget(redacted)"
}

/** Exact centavos of the document amount assigned to one target. */
class DocumentAllocationLine(val target: DocumentTarget, val centavos: Long) {
    override fun equals(other: Any?): Boolean = other is DocumentAllocationLine && target == other.target && centavos == other.centavos
    override fun hashCode(): Int = 31 * target.hashCode() + centavos.hashCode()
    override fun toString(): String = "EnterpriseDocumentAllocationLine(redacted)"
}

data class DocumentStatuses(
    val storage: DocumentState,
    val extraction: DocumentState,
    val structure: DocumentState,
    val conference: DocumentState,
    val reconciliation: DocumentState,
    val fiscal: DocumentState,
)

/** One command on one dimension; [from] is the expected current state. */
class DocumentTransition(val dimension: DocumentDimension, val from: DocumentState, val to: DocumentState, val consultedAt: Instant? = null)

/**
 * Immutable private document container: original identity, six independent statuses, owned links and
 * exact allocations. Commands return a new value or throw; content bytes and personal data never enter.
 */
class EnterpriseDocument private constructor(
    val organizationId: String,
    val id: String,
    val digest: String,
    val replaces: String?,
    val replacedBy: String?,
    private val states: Map<DocumentDimension, DocumentState>,
    val consultedAt: Instant?,
    val amount: Long?,
    links: List<DocumentTarget>,
    allocations: List<DocumentAllocationLine>,
) {
    val links: List<DocumentTarget> = frozen(links)
    val allocations: List<DocumentAllocationLine> = frozen(allocations)

    /** Exact sum; at most MAX_ALLOCATION_LINES × MAX_TOTAL_CENTAVOS, far below Long overflow. */
    val allocated: Long get() = allocations.sumOf { it.centavos }

    /** amount − allocated, negative only when over-allocated. */
    val remaining: Long? get() = amount?.let { it - allocated }

    val statuses: DocumentStatuses get() = DocumentStatuses(
        states.getValue(DocumentDimension.STORAGE), states.getValue(DocumentDimension.EXTRACTION),
        states.getValue(DocumentDimension.STRUCTURE), states.getValue(DocumentDimension.CONFERENCE),
        reconciliation(), states.getValue(DocumentDimension.FISCAL),
    )

    /** Derived with zero tolerance from exact sums (EDR-136). */
    private fun reconciliation(): DocumentState {
        val sum = allocated
        val total = amount ?: 0L
        return when {
            sum == 0L -> DocumentState.UNALLOCATED
            sum < total -> DocumentState.PARTIALLY_ALLOCATED
            sum == total -> DocumentState.RECONCILED
            else -> DocumentState.OVER_ALLOCATED
        }
    }

    private fun copy(
        replaces: String? = this.replaces,
        replacedBy: String? = this.replacedBy,
        states: Map<DocumentDimension, DocumentState> = this.states,
        consultedAt: Instant? = this.consultedAt,
        amount: Long? = this.amount,
        links: List<DocumentTarget> = this.links,
        allocations: List<DocumentAllocationLine> = this.allocations,
    ) = EnterpriseDocument(organizationId, id, digest, replaces, replacedBy, states, consultedAt, amount, links, allocations)

    /** Only compares a supplied digest; a match proves no fiscal, conference or other status (BR-TRUTH-002). */
    fun matchesOriginal(candidate: String): Boolean {
        if (!digestPattern.matches(candidate)) fail()
        return candidate == digest
    }

    /** Runs one transition command and changes exactly one dimension. */
    fun apply(transition: DocumentTransition): EnterpriseDocument {
        val dimension = transition.dimension
        if (transition.from.dimension != dimension || transition.to.dimension != dimension) fail()
        if (dimension.derived) fail(DocumentFailure.DERIVED)
        val at = transition.consultedAt
        if ((dimension == DocumentDimension.FISCAL) != (at != null)) fail()
        if (at != null && (at < minimumInstant || at > maximumInstant)) fail()
        if (states.getValue(dimension) != transition.from) fail(DocumentFailure.CONFLICT)
        if (transition.to !in DocumentState.allowed(dimension, transition.from)) fail(DocumentFailure.INVALID_TRANSITION)
        if (at != null && consultedAt != null && at <= consultedAt) fail(DocumentFailure.CONFLICT)
        return copy(states = states + (dimension to transition.to), consultedAt = at ?: consultedAt)
    }

    /** Records or corrects the exact amount; allocations are kept (a lower amount is observed as OVER_ALLOCATED). */
    fun withAmount(centavos: Long): EnterpriseDocument {
        if (centavos !in 0..PremiumBounds.MAX_TOTAL_CENTAVOS) fail()
        return copy(amount = centavos)
    }

    private fun scoped(target: DocumentTarget) { if (target.organizationId != organizationId) fail(DocumentFailure.SCOPE) }

    /** Adds an owned same-organization link; it grants no access and recognizes no cost. */
    fun link(target: DocumentTarget): EnterpriseDocument {
        scoped(target)
        if (target in links) fail(DocumentFailure.DUPLICATE)
        return copy(links = links + target)
    }

    private fun claim(target: DocumentTarget, seen: MutableSet<DocumentTarget>) {
        if (!seen.add(target)) fail(DocumentFailure.DUPLICATE)
        if (!target.kind.allocatable) fail(DocumentFailure.NOT_ALLOCATABLE)
        if (target !in links) fail(DocumentFailure.NOT_LINKED)
    }

    /** Replaces the allocation set; the whole set is refused on any invalid line or an exact sum above the amount. */
    fun allocate(lines: List<DocumentAllocationLine>): EnterpriseDocument {
        val total = amount ?: fail(DocumentFailure.AMOUNT_MISSING)
        val copied = lines.toList()
        if (copied.size > EnterpriseValues.MAX_ALLOCATION_LINES) fail()
        val seen = mutableSetOf<DocumentTarget>()
        var sum = 0L
        for (line in copied) {
            scoped(line.target)
            if (line.centavos !in 1..PremiumBounds.MAX_TOTAL_CENTAVOS) fail()
            claim(line.target, seen)
            sum += line.centavos
        }
        if (sum > total) fail(DocumentFailure.EXCEEDS_AMOUNT)
        return copy(allocations = copied)
    }

    /** Allocates the whole amount with the BR-MONEY-007 largest-remainder allocator; zero shares are omitted. */
    fun splitEvenly(targets: List<DocumentTarget>): EnterpriseDocument {
        val total = amount ?: fail(DocumentFailure.AMOUNT_MISSING)
        val copied = targets.toList()
        if (copied.isEmpty() || copied.size > EnterpriseValues.MAX_ALLOCATION_LINES) fail()
        val seen = mutableSetOf<DocumentTarget>()
        for (target in copied) { scoped(target); claim(target, seen) }
        val parts = EnterpriseValues.allocate(total, copied.indices.map { AllocationLine(it.toLong(), 1) }).parts ?: fail()
        return allocate(parts.filter { it.centavos > 0 }.map { DocumentAllocationLine(copied[it.order.toInt()], it.centavos) })
    }

    override fun toString(): String = "EnterpriseDocument(redacted)"

    companion object {
        /** Creates the container for one original (canonical UUID and 64-lowercase-hex digest) at the initial states. */
        fun create(organizationId: String, id: String, digest: String): EnterpriseDocument {
            organization(organizationId)
            identifier(id)
            if (!digestPattern.matches(digest)) fail()
            val states = DocumentDimension.entries.filterNot { it.derived }.associateWith { DocumentState.initial(it) }
            return EnterpriseDocument(organizationId, id, digest, null, null, states, null, null, emptyList(), emptyList())
        }

        /** Records a stored same-organization successor; the old original is kept and becomes REPLACED. */
        fun replace(old: EnterpriseDocument, successor: EnterpriseDocument): Pair<EnterpriseDocument, EnterpriseDocument> {
            if (old.organizationId != successor.organizationId) fail(DocumentFailure.SCOPE)
            if (old.id == successor.id) fail()
            if (successor.replaces != null) fail(DocumentFailure.DUPLICATE)
            val storage = old.states.getValue(DocumentDimension.STORAGE)
            if ((storage != DocumentState.STORED && storage != DocumentState.QUARANTINED) ||
                successor.states.getValue(DocumentDimension.STORAGE) != DocumentState.STORED
            ) fail(DocumentFailure.INVALID_TRANSITION)
            return old.copy(replacedBy = successor.id, states = old.states + (DocumentDimension.STORAGE to DocumentState.REPLACED)) to
                successor.copy(replaces = old.id)
        }
    }
}
