package com.anpfuel.domain.enterprise.maintenance.incident

/**
 * EMP-CHECK-003 (B-BR-EMP-369, B-BR-EMP-373, EDR-241): the pure damage and incident report rule shared with Go
 * (backend/internal/modules/maintenance/domain/incident) through incident-v1. The app records reports offline and
 * shows their handling state; a report never holds a cost, a document or a person's health.
 */
enum class IncidentFailure { INVALID, CONFLICT, TRANSITION, SELF_APPROVAL }

class IncidentDomainException(val failure: IncidentFailure) : IllegalArgumentException("incident: ${failure.name.lowercase()}")

private fun failure(kind: IncidentFailure = IncidentFailure.INVALID) = IncidentDomainException(kind)

enum class IncidentReportType {
    DAMAGE, INCIDENT;
    companion object { fun parse(raw: String): IncidentReportType = entries.firstOrNull { it.name == raw } ?: throw failure() }
}

enum class IncidentKind {
    BREAKDOWN, ACCIDENT, THEFT, FINE_NOTICE, OTHER;
    companion object { fun parse(raw: String): IncidentKind = entries.firstOrNull { it.name == raw } ?: throw failure() }
}

enum class IncidentState {
    REPORTED, ACKNOWLEDGED, IN_TREATMENT, RESOLVED, DISMISSED;
    val terminal: Boolean get() = this == RESOLVED || this == DISMISSED
    companion object { fun parse(raw: String): IncidentState = entries.firstOrNull { it.name == raw } ?: throw failure() }
}

enum class IncidentAction(val target: IncidentState, val needsNote: Boolean) {
    ACKNOWLEDGE(IncidentState.ACKNOWLEDGED, false),
    TREAT(IncidentState.IN_TREATMENT, true),
    RESOLVE(IncidentState.RESOLVED, true),
    DISMISS(IncidentState.DISMISSED, true);
    companion object { fun parse(raw: String): IncidentAction = entries.firstOrNull { it.name == raw } ?: throw failure() }
}

/** A validated report at its initial state; free text is never part of [toString]. */
class IncidentReport private constructor(
    val type: IncidentReportType,
    val kind: IncidentKind?,
    val description: String,
    val location: String?,
    val blocksUse: Boolean,
    val state: IncidentState,
    val revision: Long,
) {
    override fun toString(): String = "IncidentReport(redacted)"

    companion object {
        /** Validates one draft: the type, the kind (required for an incident, absent for a damage), the description and the optional location. */
        fun draft(type: String, kind: String?, description: String, location: String?, blocksUse: Boolean): IncidentReport {
            val parsedType = IncidentReportType.parse(type)
            val parsedKind = when (parsedType) {
                IncidentReportType.INCIDENT -> IncidentKind.parse(kind ?: "")
                IncidentReportType.DAMAGE -> if (kind.isNullOrEmpty()) null else throw failure()
            }
            if (!Incident.validText(description, Incident.MAX_DESCRIPTION)) throw failure()
            if (location != null && !Incident.validText(location, Incident.MAX_LOCATION)) throw failure()
            return IncidentReport(parsedType, parsedKind, description, location, blocksUse, IncidentState.REPORTED, 1)
        }
    }
}

/** The stored fact a handling command is decided against; [reporter] is the set of its identity keys. */
data class IncidentCurrent(val state: String, val revision: Long, val blocksUse: Boolean, val reportId: String, val reporter: List<String>)

/** One handling command; [note] and [duplicateOf] are null when absent; [handler] is the set of the handler's identity keys. */
class IncidentCommand(val action: String, val expectedRevision: Long, val note: String?, val duplicateOf: String?, val handler: List<String>) {
    override fun toString(): String = "IncidentCommand(redacted)"
}

data class IncidentStep(val action: IncidentAction, val from: IncidentState, val to: IncidentState, val revision: Long)

object Incident {
    const val MAX_DESCRIPTION = 1000
    const val MAX_LOCATION = 140
    const val MAX_NOTE = 500
    /** The longest handling path: REPORTED (1), ACKNOWLEDGED (2), IN_TREATMENT (3), RESOLVED or DISMISSED (4). */
    const val MAX_REVISION = 4L

    /** The 08-state-model transitions. */
    fun allowed(from: IncidentState, action: IncidentAction): Boolean = when (action) {
        IncidentAction.ACKNOWLEDGE -> from == IncidentState.REPORTED
        IncidentAction.TREAT -> from == IncidentState.REPORTED || from == IncidentState.ACKNOWLEDGED
        IncidentAction.RESOLVE, IncidentAction.DISMISS -> !from.terminal
    }

    /** A lowercase, dashed, non-nil UUID. */
    fun canonicalId(raw: String): Boolean {
        if (raw.length != 36 || raw == "00000000-0000-0000-0000-000000000000") return false
        return raw.withIndex().all { (i, c) -> if (i == 8 || i == 13 || i == 18 || i == 23) c == '-' else c in '0'..'9' || c in 'a'..'f' }
    }

    /** No lone surrogate, 1..[maximum] scalars, no C0/C1 control and not only white space (SQL org_asset_text_valid). */
    fun validText(raw: String, maximum: Int): Boolean {
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
        return scalars <= maximum && says
    }

    private fun validIdentity(keys: List<String>) = keys.isNotEmpty() && keys.all(::canonicalId)

    /**
     * Applies one handling command, first failure first: INVALID (current fact, action, expected revision, handler,
     * note policy, duplicate), CONFLICT (expected revision differs), TRANSITION (not allowed from the current state),
     * SELF_APPROVAL (the reporter resolves or dismisses a blocking report).
     */
    fun decide(current: IncidentCurrent, command: IncidentCommand): IncidentStep {
        val state = IncidentState.parse(current.state)
        if (current.revision < 1 || current.revision > MAX_REVISION || (state == IncidentState.REPORTED) != (current.revision == 1L) ||
            !canonicalId(current.reportId) || !validIdentity(current.reporter)) throw failure()
        val action = IncidentAction.parse(command.action)
        if (command.expectedRevision < 1 || !validIdentity(command.handler)) throw failure()
        val note = command.note
        if (action.needsNote != (note != null) || note != null && !validText(note, MAX_NOTE)) throw failure()
        val duplicate = command.duplicateOf
        if (!duplicate.isNullOrEmpty() && (action != IncidentAction.DISMISS || !canonicalId(duplicate) || duplicate == current.reportId)) throw failure()
        if (command.expectedRevision != current.revision) throw failure(IncidentFailure.CONFLICT)
        if (!allowed(state, action)) throw failure(IncidentFailure.TRANSITION)
        if (current.blocksUse && (action == IncidentAction.RESOLVE || action == IncidentAction.DISMISS) && command.handler.any { it in current.reporter }) {
            throw failure(IncidentFailure.SELF_APPROVAL)
        }
        return IncidentStep(action, state, action.target, current.revision + 1)
    }
}
