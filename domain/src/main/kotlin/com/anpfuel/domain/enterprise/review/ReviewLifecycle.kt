package com.anpfuel.domain.enterprise.review

/** Closed failure vocabulary of review-v1; the first failure wins in declaration order. */
enum class ReviewFailure { INVALID, REASON_REQUIRED, FORBIDDEN, SELF_APPROVAL, CONFLICT, INVALID_TRANSITION }

class ReviewDomainException(val failure: ReviewFailure) : IllegalArgumentException("review: ${failure.name.lowercase()}")

enum class ReviewBasis { COUNTED, PENDING, EXCLUDED }

enum class ReviewRole { AUTHOR, REVIEWER }

enum class ReasonPolicy { NONE, OPTIONAL, REQUIRED }

/** Server review states of review-v1 (BR-REV); a device-local readiness is never one of them. */
enum class ReviewState {
    DRAFT, SUBMITTED, IN_REVIEW, APPROVED, RETURNED, REJECTED, CANCELLED;

    val terminal: Boolean get() = transitions[this].isNullOrEmpty()
    val basis: ReviewBasis get() = when (this) {
        APPROVED -> ReviewBasis.COUNTED
        SUBMITTED, IN_REVIEW -> ReviewBasis.PENDING
        DRAFT, RETURNED, REJECTED, CANCELLED -> ReviewBasis.EXCLUDED
    }

    companion object {
        fun parse(raw: String): ReviewState = entries.firstOrNull { it.name == raw } ?: throw reviewFailure(ReviewFailure.INVALID)
    }
}

enum class ReviewCommandKind(val role: ReviewRole, val reason: ReasonPolicy) {
    SUBMIT(ReviewRole.AUTHOR, ReasonPolicy.NONE),
    CANCEL(ReviewRole.AUTHOR, ReasonPolicy.OPTIONAL),
    RESUBMIT(ReviewRole.AUTHOR, ReasonPolicy.NONE),
    OPEN(ReviewRole.REVIEWER, ReasonPolicy.NONE),
    APPROVE(ReviewRole.REVIEWER, ReasonPolicy.OPTIONAL),
    RETURN(ReviewRole.REVIEWER, ReasonPolicy.REQUIRED),
    REJECT(ReviewRole.REVIEWER, ReasonPolicy.REQUIRED);

    companion object {
        fun parse(raw: String): ReviewCommandKind = entries.firstOrNull { it.name == raw } ?: throw reviewFailure(ReviewFailure.INVALID)
    }
}

/** One canonical person resolved by the server at receipt (merge chain followed). */
data class ReviewIdentity(
    val accounts: List<String> = emptyList(),
    val memberships: List<String> = emptyList(),
    val employees: List<String> = emptyList(),
)

/** Facts supplied by the server at receipt; [permitted] is the authority port's answer. */
data class ReviewFacts(
    val actor: ReviewIdentity,
    val author: ReviewIdentity,
    val beneficiary: ReviewIdentity? = null,
    val permitted: Boolean = false,
)

data class ReviewCommand(val kind: ReviewCommandKind, val revision: Long, val reason: String = "")

data class ReviewEntry(val kind: ReviewCommandKind, val revision: Long, val state: ReviewState, val reason: String = "")

/** Immutable review case; only [draft], [replay] and [apply] build one. */
class ReviewCase private constructor(val state: ReviewState, val revision: Long, private val entries: List<ReviewEntry>) {
    /** A copy of the append-only history. */
    val history: List<ReviewEntry> get() = entries.toList()

    /** Validates one command at receipt; a refusal throws and this case is unchanged. */
    fun apply(command: ReviewCommand, facts: ReviewFacts): ReviewCase {
        if (command.revision < 1 || !facts.valid()) throw reviewFailure(ReviewFailure.INVALID)
        checkReason(command.kind, command.reason)
        if (!facts.permitted) throw reviewFailure(ReviewFailure.FORBIDDEN)
        when (command.kind.role) {
            ReviewRole.AUTHOR -> if (!facts.actor.shares(facts.author)) throw reviewFailure(ReviewFailure.FORBIDDEN)
            ReviewRole.REVIEWER -> if (facts.actor.shares(facts.author) || facts.beneficiary?.let(facts.actor::shares) == true) {
                throw reviewFailure(ReviewFailure.SELF_APPROVAL)
            }
        }
        if (command.revision != revision) throw reviewFailure(ReviewFailure.CONFLICT)
        val next = nextEntry(state, revision, command.kind) ?: throw reviewFailure(ReviewFailure.INVALID_TRANSITION)
        return ReviewCase(next.state, next.revision, entries + next.copy(reason = command.reason))
    }

    companion object {
        /** Existing private reason bound (B-BR-EMP-163). */
        const val MAX_REASON_SCALARS = 280

        fun draft(): ReviewCase = ReviewCase(ReviewState.DRAFT, 1, emptyList())

        /** Rebuilds a case from stored history; any entry the lifecycle could not produce is INVALID. */
        fun replay(history: List<ReviewEntry>): ReviewCase {
            var state = ReviewState.DRAFT
            var revision = 1L
            val built = ArrayList<ReviewEntry>(history.size)
            for (entry in history.toList()) {
                try { checkReason(entry.kind, entry.reason) } catch (_: ReviewDomainException) { throw reviewFailure(ReviewFailure.INVALID) }
                val next = nextEntry(state, revision, entry.kind)
                if (next == null || next.state != entry.state || next.revision != entry.revision) throw reviewFailure(ReviewFailure.INVALID)
                built += next.copy(reason = entry.reason)
                state = next.state
                revision = next.revision
            }
            return ReviewCase(state, revision, built.toList())
        }
    }
}

internal fun reviewFailure(failure: ReviewFailure) = ReviewDomainException(failure)

/** The BR-REV diagram for one level (EDR-128); decisions only from IN_REVIEW, terminal states absent. */
private val transitions: Map<ReviewState, Map<ReviewCommandKind, ReviewState>> = mapOf(
    ReviewState.DRAFT to mapOf(ReviewCommandKind.SUBMIT to ReviewState.SUBMITTED, ReviewCommandKind.CANCEL to ReviewState.CANCELLED),
    ReviewState.SUBMITTED to mapOf(ReviewCommandKind.OPEN to ReviewState.IN_REVIEW, ReviewCommandKind.CANCEL to ReviewState.CANCELLED),
    ReviewState.IN_REVIEW to mapOf(
        ReviewCommandKind.APPROVE to ReviewState.APPROVED,
        ReviewCommandKind.RETURN to ReviewState.RETURNED,
        ReviewCommandKind.REJECT to ReviewState.REJECTED,
    ),
    ReviewState.RETURNED to mapOf(ReviewCommandKind.RESUBMIT to ReviewState.SUBMITTED, ReviewCommandKind.CANCEL to ReviewState.CANCELLED),
)

/** Only RESUBMIT creates a revision; it is bounded by the history length and cannot overflow. */
private fun nextEntry(state: ReviewState, revision: Long, kind: ReviewCommandKind): ReviewEntry? {
    val target = transitions[state]?.get(kind) ?: return null
    return ReviewEntry(kind, if (kind == ReviewCommandKind.RESUBMIT) revision + 1 else revision, target)
}

/** Reason policy with the existing 280-scalar control-free rule; reasons are kept verbatim. */
private fun checkReason(kind: ReviewCommandKind, raw: String) {
    var says = false
    var scalars = 0
    var index = 0
    while (index < raw.length) {
        val point = raw.codePointAt(index)
        if (point in 0xd800..0xdfff || Character.isISOControl(point)) throw reviewFailure(ReviewFailure.INVALID)
        if (!Character.isWhitespace(point) && !Character.isSpaceChar(point)) says = true
        scalars++
        index += Character.charCount(point)
    }
    if (scalars > ReviewCase.MAX_REASON_SCALARS) throw reviewFailure(ReviewFailure.INVALID)
    when (kind.reason) {
        ReasonPolicy.REQUIRED -> if (!says) throw reviewFailure(ReviewFailure.REASON_REQUIRED)
        ReasonPolicy.OPTIONAL -> if (raw.isNotEmpty() && !says) throw reviewFailure(ReviewFailure.INVALID)
        ReasonPolicy.NONE -> if (raw.isNotEmpty()) throw reviewFailure(ReviewFailure.INVALID)
    }
}

private fun ReviewIdentity.all(): List<String> = accounts + memberships + employees

private fun canonicalUuid(raw: String): Boolean =
    raw.length == 36 && raw != "00000000-0000-0000-0000-000000000000" &&
        raw.withIndex().all { (i, c) -> if (i == 8 || i == 13 || i == 18 || i == 23) c == '-' else c in '0'..'9' || c in 'a'..'f' }

private fun ReviewIdentity.valid(): Boolean = all().let { ids -> ids.isNotEmpty() && ids.all(::canonicalUuid) }

/** Any shared canonical identifier, whatever its kind, is the same person (EDR-129). */
private fun ReviewIdentity.shares(other: ReviewIdentity): Boolean = other.all().toSet().let { seen -> all().any { it in seen } }

private fun ReviewFacts.valid(): Boolean = actor.valid() && author.valid() && (beneficiary?.valid() ?: true)
