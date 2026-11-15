package com.anpfuel.domain.enterprise.document

enum class DocumentFailure { INVALID, SCOPE, INVALID_TRANSITION, CONFLICT, DERIVED, DUPLICATE, NOT_LINKED, NOT_ALLOCATABLE, EXCEEDS_AMOUNT, AMOUNT_MISSING }
class DocumentDomainException(val failure: DocumentFailure = DocumentFailure.INVALID) :
    IllegalArgumentException("document: ${failure.name.lowercase()}")
@Suppress("NOTHING_TO_INLINE") // Inlined so no unreachable post-call throw is generated.
internal inline fun fail(failure: DocumentFailure = DocumentFailure.INVALID): Nothing = throw DocumentDomainException(failure)

/** Six independent answers; only reconciliation is derived, from exact amounts. */
enum class DocumentDimension { STORAGE, EXTRACTION, STRUCTURE, CONFERENCE, RECONCILIATION, FISCAL;
    val derived: Boolean get() = this == RECONCILIATION
    companion object { fun parse(raw: String): DocumentDimension = entries.firstOrNull { it.name == raw } ?: fail() }
}

/** Closed tokens per dimension; the first entry of each dimension is its initial state. */
enum class DocumentState(val dimension: DocumentDimension, val token: String) {
    UPLOADING(DocumentDimension.STORAGE, "UPLOADING"), RECEIVED(DocumentDimension.STORAGE, "RECEIVED"),
    STORED(DocumentDimension.STORAGE, "STORED"), QUARANTINED(DocumentDimension.STORAGE, "QUARANTINED"),
    STORAGE_FAILED(DocumentDimension.STORAGE, "FAILED"), REPLACED(DocumentDimension.STORAGE, "REPLACED"),
    DELETED(DocumentDimension.STORAGE, "DELETED"),
    EXTRACTION_NONE(DocumentDimension.EXTRACTION, "NONE"), EXTRACTION_PENDING(DocumentDimension.EXTRACTION, "PENDING"),
    EXTRACTION_DONE(DocumentDimension.EXTRACTION, "DONE"), EXTRACTION_FAILED(DocumentDimension.EXTRACTION, "FAILED"),
    NOT_APPLICABLE(DocumentDimension.STRUCTURE, "NOT_APPLICABLE"), UNCHECKED(DocumentDimension.STRUCTURE, "UNCHECKED"),
    STRUCTURE_OK(DocumentDimension.STRUCTURE, "STRUCTURE_OK"), STRUCTURE_INVALID(DocumentDimension.STRUCTURE, "STRUCTURE_INVALID"),
    UNCONFERRED(DocumentDimension.CONFERENCE, "UNCONFERRED"), CONFERRED_OK(DocumentDimension.CONFERENCE, "CONFERRED_OK"),
    DIVERGENT(DocumentDimension.CONFERENCE, "DIVERGENT"), ILLEGIBLE(DocumentDimension.CONFERENCE, "ILLEGIBLE"),
    UNALLOCATED(DocumentDimension.RECONCILIATION, "UNALLOCATED"), PARTIALLY_ALLOCATED(DocumentDimension.RECONCILIATION, "PARTIALLY_ALLOCATED"),
    RECONCILED(DocumentDimension.RECONCILIATION, "RECONCILED"), OVER_ALLOCATED(DocumentDimension.RECONCILIATION, "OVER_ALLOCATED"),
    NOT_CONSULTED(DocumentDimension.FISCAL, "NOT_CONSULTED"), AUTHORIZED_AS_OF(DocumentDimension.FISCAL, "AUTHORIZED_AS_OF"),
    CANCELLED_AS_OF(DocumentDimension.FISCAL, "CANCELLED_AS_OF"), UNAVAILABLE(DocumentDimension.FISCAL, "UNAVAILABLE");

    companion object {
        private val fiscalResults = listOf(AUTHORIZED_AS_OF, CANCELLED_AS_OF, UNAVAILABLE)
        private val conference = listOf(CONFERRED_OK, DIVERGENT, ILLEGIBLE, UNCONFERRED)

        /** Generic commands (EDR-134). REPLACED is entered only by replacement; reconciliation has none. */
        private val transitions: Map<DocumentState, List<DocumentState>> = mapOf(
            UPLOADING to listOf(STORAGE_FAILED, RECEIVED),
            RECEIVED to listOf(STORAGE_FAILED, QUARANTINED, STORED),
            STORED to listOf(DELETED), QUARANTINED to listOf(DELETED), REPLACED to listOf(DELETED),
            EXTRACTION_NONE to listOf(EXTRACTION_PENDING),
            EXTRACTION_PENDING to listOf(EXTRACTION_DONE, EXTRACTION_FAILED),
            EXTRACTION_FAILED to listOf(EXTRACTION_PENDING),
            NOT_APPLICABLE to listOf(UNCHECKED),
            UNCHECKED to listOf(STRUCTURE_INVALID, STRUCTURE_OK),
            STRUCTURE_INVALID to listOf(UNCHECKED),
        ) + conference.associateWith { from -> conference - from } +
            entries.filter { it.dimension == DocumentDimension.FISCAL }.associateWith { fiscalResults }

        fun parse(dimension: DocumentDimension, raw: String): DocumentState =
            entries.firstOrNull { it.dimension == dimension && it.token == raw } ?: fail()

        fun initial(dimension: DocumentDimension): DocumentState = entries.first { it.dimension == dimension }

        /** Lexicographically sorted generic targets of a state of the named dimension. */
        fun allowed(dimension: DocumentDimension, from: DocumentState): List<DocumentState> {
            if (from.dimension != dimension) fail()
            if (dimension.derived) fail(DocumentFailure.DERIVED)
            return transitions[from].orEmpty().sortedBy { it.token }
        }
    }
}

/** Closed link targets (EDR-135); only an expense is allocatable (EDR-013). */
enum class DocumentTargetKind { FUELING, EXPENSE, WORK_ORDER, TRIP, ASSET, EMPLOYEE, METER_READING;
    val allocatable: Boolean get() = this == EXPENSE
    companion object { fun parse(raw: String): DocumentTargetKind = entries.firstOrNull { it.name == raw } ?: fail() }
}
