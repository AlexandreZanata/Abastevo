package com.anpfuel.domain.enterprise.maintenance.checklist

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncProjection
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncJson
import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import java.time.Instant

/*
 * EMP-CHECK-002 (B-BR-EMP-438/439/443, EDR-283/286): the Android producer's signed `check.run.complete` schema 1
 * operation pinned by contracts/testdata/enterprise/checklist/run-v1.psv, the sealed `checklist-book` v1 index of one
 * partition and what the execution screens may state. The device decides the outcome it shows from the cached
 * template version and policy revision; the server's recheck governs and is read back, never assumed.
 */

/** The intake refusal class a builder or decoder refusal corresponds to (Go `ParseRun`). */
enum class ChecklistRunOperationFailure { MALFORMED, UNSUPPORTED }

class ChecklistRunOperationException(val failure: ChecklistRunOperationFailure = ChecklistRunOperationFailure.MALFORMED) :
    IllegalArgumentException("checklist run operation: ${failure.name.lowercase()}")

/** The envelope facts of one device-produced run completion; the binding is the creation partition (BR-OFFL-003). */
class ChecklistRunEnvelope(val binding: SyncBinding, val membershipId: String, val enrollmentId: String, val operationId: String,
    val declared: TimeDeclaration) {
    override fun toString(): String = "EnterpriseChecklistRunEnvelope(redacted)"
}

/** One answer in wire form; the builder validates it. A photo claim is never produced (EDR-286). */
class ChecklistRunAnswerInput(val answer: String, val note: String?, val photo: Boolean = false) {
    override fun toString(): String = "EnterpriseChecklistRunAnswerInput(redacted)"
}

/** One decoded answer; the note is private field text and never rendered by a log. */
class ChecklistRunAnswer(val answer: String, val note: String?) {
    override fun equals(other: Any?): Boolean = other is ChecklistRunAnswer && answer == other.answer && note == other.note
    override fun hashCode(): Int = 31 * answer.hashCode() + (note?.hashCode() ?: 0)
    override fun toString(): String = "EnterpriseChecklistRunAnswer(redacted)"
}

/** One decoded `check.run.complete`; it proves only its own shape. */
class ChecklistRunFact(val operationId: String, val declared: TimeDeclaration, val runId: String, val assetId: String,
    val context: ChecklistContext, val templateId: String, val templateVersion: Long, val policyRevision: Long, val startedAt: Instant,
    answers: List<ChecklistRunAnswer>, val deviceOutcome: ChecklistOutcome, val acknowledged: Boolean) {
    private val values = answers.toList()
    val answers: List<ChecklistRunAnswer> get() = values.toList()
    override fun toString(): String = "EnterpriseChecklistRunFact(redacted)"
}

private fun malformed() = ChecklistRunOperationException(ChecklistRunOperationFailure.MALFORMED)
private fun unsupported() = ChecklistRunOperationException(ChecklistRunOperationFailure.UNSUPPORTED)
private val ANSWERS = setOf(ChecklistAnswer.OK, ChecklistAnswer.NOT_OK, ChecklistAnswer.NOT_APPLICABLE)
private val RUN_KEYS = setOf("asset_id", "context", "template_id", "template_version", "policy_revision", "started_at", "answers",
    "device_outcome", "acknowledged")
private val ANSWER_KEYS = setOf("answer", "note", "photo")

private fun validNote(note: String) = validText(note, ChecklistDefinition.MAX_NOTE, true)
private fun context(raw: String): ChecklistContext = ChecklistContext.entries.firstOrNull { it.name == raw } ?: throw malformed()
private fun outcome(raw: String): ChecklistOutcome = ChecklistOutcome.entries.firstOrNull { it.name == raw } ?: throw malformed()

/** A canonical UTC instant with at most microsecond precision, the form the run stores (Go `canonicalInstant`). */
private fun startedInstant(raw: String): Instant {
    val at = try { EnterpriseTime.parseInstant(raw) } catch (_: IllegalArgumentException) { throw malformed() }
    if (EnterpriseTime.formatInstant(at) != raw || at.nano % 1_000 != 0) throw malformed()
    return at
}

private fun declaration(declared: TimeDeclaration): TimeDeclaration =
    try { EnterpriseTime.validateDeclaration(declared) } catch (_: IllegalArgumentException) { throw malformed() }

private fun text(value: String) = SyncJson.Text(value)

object ChecklistRunOperations {
    const val COMPLETE = "check.run.complete"
    const val ENTITY = "checklist_run"
    const val MAX_COUNTER = 2_147_483_647L

    /**
     * `check.run.complete` schema 1 (B-BR-EMP-439): no expected revision, dependency, soft reference or attachment.
     * Every malformed value is refused first; a photo claim on a NOT_OK answer is then UNSUPPORTED, like the intake.
     */
    fun complete(envelope: ChecklistRunEnvelope, runId: String, assetId: String, context: String, templateId: String, templateVersion: Long,
        policyRevision: Long, startedAt: String, answers: List<ChecklistRunAnswerInput>, deviceOutcome: String,
        acknowledged: Boolean): EnterpriseOperation {
        if (!Checklist.canonicalId(runId) || !Checklist.canonicalId(assetId) || !Checklist.canonicalId(templateId)) throw malformed()
        val moment = context(context)
        if (templateVersion !in 1..MAX_COUNTER || policyRevision !in 1..MAX_COUNTER) throw malformed()
        if (answers.isEmpty() || answers.size > ChecklistDefinition.MAX_ITEMS) throw malformed()
        var photo = false
        for (answer in answers) {
            if (answer.answer !in ANSWERS || answer.note != null && !validNote(answer.note)) throw malformed()
            if (answer.photo) { if (answer.answer != ChecklistAnswer.NOT_OK) throw malformed(); photo = true }
        }
        val claimed = outcome(deviceOutcome)
        if (acknowledged && claimed != ChecklistOutcome.COMPLETED_WITH_ISSUES) throw malformed()
        val started = startedInstant(startedAt)
        val declared = declaration(envelope.declared)
        if (started.isAfter(declared.occurredAt)) throw malformed()
        if (photo) throw unsupported()
        val payload = mapOf(
            "asset_id" to text(assetId), "context" to text(moment.name), "template_id" to text(templateId),
            "template_version" to SyncJson.Integer(templateVersion.toString()), "policy_revision" to SyncJson.Integer(policyRevision.toString()),
            "started_at" to text(startedAt),
            "answers" to SyncJson.Array(answers.map {
                SyncJson.Object(mapOf("answer" to text(it.answer), "note" to (it.note?.let(::text) ?: SyncJson.Null), "photo" to SyncJson.Bool(false)))
            }),
            "device_outcome" to text(claimed.name), "acknowledged" to SyncJson.Bool(acknowledged),
        )
        return try {
            val root = SyncJson.Object(mapOf(
                "schema_version" to SyncJson.Integer("1"), "operation_id" to text(envelope.operationId),
                "owner_account_id" to text(envelope.binding.ownerAccountId), "api_origin" to text(envelope.binding.apiOrigin),
                "org_id" to text(envelope.binding.organizationId), "membership_id" to text(envelope.membershipId),
                "enrollment_id" to text(envelope.enrollmentId), "type" to text(COMPLETE), "operation_schema" to SyncJson.Integer("1"),
                "entity" to SyncJson.Object(mapOf("type" to text(ENTITY), "id" to text(runId), "expected_revision" to SyncJson.Null)),
                "occurred_at" to text(EnterpriseTime.formatInstant(declared.occurredAt)), "entry_zone" to text(declared.entryZone),
                "device_offset_seconds" to SyncJson.Integer(declared.deviceOffsetSeconds.toString()),
                "depends_on" to SyncJson.Array(emptyList()), "soft_references" to SyncJson.Array(emptyList()),
                "attachments" to SyncJson.Array(emptyList()), "payload" to SyncJson.Object(payload),
            ))
            EnterpriseOperation.seal(CanonicalJson.encode(root, CanonicalJson.MAX_OPERATION_DEPTH), envelope.binding)
        } catch (_: SyncProtocolException) { throw malformed() }
    }

    private fun textOf(fields: Map<String, SyncJson>, key: String): String = (fields[key] as? SyncJson.Text)?.value ?: throw malformed()
    private fun counter(value: SyncJson?): Long {
        val lexeme = (value as? SyncJson.Integer)?.lexeme ?: throw malformed()
        val parsed = lexeme.toLongOrNull() ?: throw malformed()
        if (parsed !in 1..MAX_COUNTER) throw malformed()
        return parsed
    }
    private fun flag(value: SyncJson?): Boolean = (value as? SyncJson.Bool)?.value ?: throw malformed()

    /** Strict decode mirroring Go `ParseRun`; anything else is refused with its intake class. */
    fun read(operation: EnterpriseOperation): ChecklistRunFact {
        val entity = operation.entity
        if (operation.type != COMPLETE || !operation.supportsSchema || entity.reference.type != ENTITY || entity.expectedRevision != null ||
            !Checklist.canonicalId(entity.reference.id) || operation.dependsOn.isNotEmpty()) throw malformed()
        val fields = operation.payload.fields
        if (fields.keys != RUN_KEYS) throw malformed()
        val asset = textOf(fields, "asset_id")
        val template = textOf(fields, "template_id")
        val rawContext = textOf(fields, "context")
        val rawStarted = textOf(fields, "started_at")
        val rawOutcome = textOf(fields, "device_outcome")
        val version = counter(fields["template_version"])
        val policy = counter(fields["policy_revision"])
        val acknowledged = flag(fields["acknowledged"])
        val items = (fields["answers"] as? SyncJson.Array)?.values ?: throw malformed()
        if (items.isEmpty() || items.size > ChecklistDefinition.MAX_ITEMS) throw malformed()
        var photo = false
        val answers = items.map { item ->
            val answer = (item as? SyncJson.Object)?.fields ?: throw malformed()
            if (answer.keys != ANSWER_KEYS) throw malformed()
            val token = textOf(answer, "answer")
            if (token !in ANSWERS) throw malformed()
            val note = when (val raw = answer["note"]) {
                SyncJson.Null -> null
                is SyncJson.Text -> raw.value.also { if (!validNote(it)) throw malformed() }
                else -> throw malformed()
            }
            val declared = flag(answer["photo"])
            if (declared && token != ChecklistAnswer.NOT_OK) throw malformed()
            photo = photo || declared
            ChecklistRunAnswer(token, note)
        }
        if (!Checklist.canonicalId(asset) || !Checklist.canonicalId(template)) throw malformed()
        val moment = context(rawContext)
        val claimed = outcome(rawOutcome)
        if (acknowledged && claimed != ChecklistOutcome.COMPLETED_WITH_ISSUES) throw malformed()
        val started = startedInstant(rawStarted)
        val declared = declaration(operation.declaration)
        if (started.isAfter(declared.occurredAt)) throw malformed()
        if (photo || operation.attachments.isNotEmpty() || operation.softReferences.isNotEmpty()) throw unsupported()
        return ChecklistRunFact(operation.operationId, declared, entity.reference.id, asset, moment, template, version, policy, started, answers,
            claimed, acknowledged)
    }
}

/** Local refusals before anything is saved or queued (B-BR-EMP-438); never a server outcome. */
enum class ChecklistLocalFailure {
    /** No cached applicable template requires a checklist for this asset and moment. */
    NO_CHECKLIST,
    /** The run is not in this partition's book. */
    UNKNOWN_RUN,
    /** The run was already completed. */
    RUN_NOT_OPEN,
    /** The asset already has a run in progress for this moment. */
    RUN_IN_PROGRESS,
    /** The run is older than its cached policy validity; a new run is started. */
    CHECKLIST_EXPIRED,
    /** Not a position, an answer or a note this template accepts. */
    INVALID_ANSWER,
    /** The shared EMP-CHECK-001 rule: NOT_APPLICABLE on an item that is not optional. */
    ANSWER_NOT_ALLOWED,
    /** The shared rule: a NOT_OK item that requires a photo; run photos are not available on the device yet (EDR-286). */
    PHOTO_REQUIRED,
    /** The shared rule: an unanswered item. */
    INCOMPLETE,
    /** WARN with a critical issue: "Estou ciente das pendências" is required. */
    ACKNOWLEDGEMENT_REQUIRED,
    /** The device clock is earlier than the run's start. */
    BEFORE_START,
    /** The identifier already exists in the book. */
    DUPLICATE,
    /** The book reached its bound. */
    FULL,
    /** The book belongs to another membership or enrollment. */
    FOREIGN,
}

class ChecklistLocalException(val failure: ChecklistLocalFailure) : IllegalStateException("checklist local: ${failure.name.lowercase()}")

private fun refuse(failure: ChecklistLocalFailure): Nothing = throw ChecklistLocalException(failure)
private fun micro(at: Instant) = at.nano % 1_000 == 0

/** The template version a run pins: what the applicable read returned (never its targets). */
class ChecklistCachedTemplate(val templateId: String, val name: String, val version: Long, val publishedAt: Instant, items: List<ChecklistItem>) {
    private val values = ChecklistDefinition.validItems(items)
    val items: List<ChecklistItem> get() = values.toList()
    init {
        require(Checklist.canonicalId(templateId) && validText(name, ChecklistDefinition.MAX_NAME, true) &&
            version in 1..ChecklistRunOperations.MAX_COUNTER && micro(publishedAt))
    }
    override fun toString(): String = "EnterpriseChecklistCachedTemplate(redacted)"
}

/** The policy revision a run pins. */
class ChecklistCachedPolicy(val revision: Long, val mode: PolicyMode, val validitySeconds: Long, val effectiveAt: Instant) {
    init {
        ChecklistPolicy.of(mode.name, validitySeconds)
        require(revision in 1..ChecklistRunOperations.MAX_COUNTER && micro(effectiveAt))
    }
    override fun toString(): String = "EnterpriseChecklistCachedPolicy(redacted)"
}

/** One applicable read of an asset and moment with the server instant that confirmed it (EDR-283). */
class ChecklistApplicable(val assetId: String, val context: ChecklistContext, val match: ChecklistMatch, val template: ChecklistCachedTemplate?,
    val policy: ChecklistCachedPolicy?, val evaluatedAt: Instant) {
    val required: Boolean get() = match != ChecklistMatch.NONE
    init { require(Checklist.canonicalId(assetId) && (template != null) == required && (policy != null) == required && micro(evaluatedAt)) }
    override fun toString(): String = "EnterpriseChecklistApplicable(redacted)"
}

/** One saved answer of a run in progress; null is unanswered. */
class ChecklistSavedAnswer(val answer: String?, val note: String?) {
    init { require((answer == null || answer in ANSWERS) && (note == null || answer != null && validNote(note))) }
    override fun equals(other: Any?): Boolean = other is ChecklistSavedAnswer && answer == other.answer && note == other.note
    override fun hashCode(): Int = 31 * (answer?.hashCode() ?: 0) + (note?.hashCode() ?: 0)
    override fun toString(): String = "EnterpriseChecklistSavedAnswer(redacted)"
}

private fun positions(values: List<Int>) = values.all { it in 1..ChecklistDefinition.MAX_ITEMS } && values.zipWithNext().all { (a, b) -> a < b }

/** The server's recheck of a stored run as the run read returns it (B-BR-EMP-441); never a note or an author. */
class ChecklistServerResult(val revision: Long, val serverPolicyRevision: Long, val receivedAt: Instant, val deviceOutcome: ChecklistOutcome,
    val recheckOutcome: ChecklistOutcome, val outcome: ChecklistOutcome, val outcomeDiffers: Boolean, val reviewRequired: Boolean,
    val templateSuperseded: Boolean, issues: List<Int>, criticalIssues: List<Int>, val releasedAt: Instant?) {
    private val issueValues = issues.toList()
    private val criticalValues = criticalIssues.toList()
    val issues: List<Int> get() = issueValues.toList()
    val criticalIssues: List<Int> get() = criticalValues.toList()
    init {
        require(revision in 1..2 && (releasedAt != null) == (revision == 2L) && serverPolicyRevision in 1..ChecklistRunOperations.MAX_COUNTER &&
            positions(issueValues) && positions(criticalValues) && issueValues.containsAll(criticalValues) && micro(receivedAt) &&
            (releasedAt == null || micro(releasedAt)))
    }
    override fun toString(): String = "EnterpriseChecklistServerResult(redacted)"
}

/** The run read: the stored run's identity as the server holds it and its recheck. */
class ChecklistServerRun(val runId: String, val assetId: String, val templateId: String, val context: ChecklistContext, val templateVersion: Long,
    val policyRevision: Long, val startedAt: Instant, val completedAt: Instant, val acknowledged: Boolean, val result: ChecklistServerResult) {
    init { require(Checklist.canonicalId(runId) && Checklist.canonicalId(assetId) && Checklist.canonicalId(templateId)) }
    override fun toString(): String = "EnterpriseChecklistServerRun(redacted)"
}

/** One run of the book: what it pins, every saved answer, and once completed its operation and outcomes. */
class LocalChecklistRun(
    val runId: String,
    val assetId: String,
    val context: ChecklistContext,
    val template: ChecklistCachedTemplate,
    val policy: ChecklistCachedPolicy,
    val startedAt: Instant,
    answers: List<ChecklistSavedAnswer>,
    val operationId: String? = null,
    val completedAt: Instant? = null,
    val deviceOutcome: ChecklistOutcome? = null,
    val acknowledged: Boolean = false,
    val server: ChecklistServerResult? = null,
) {
    private val values = answers.toList()
    val answers: List<ChecklistSavedAnswer> get() = values.toList()
    val completed: Boolean get() = operationId != null
    val answered: Int get() = values.count { it.answer != null }
    init {
        require(Checklist.canonicalId(runId) && Checklist.canonicalId(assetId) && micro(startedAt) && values.size == template.items.size)
        if (operationId != null) EnterprisePartition.requireId(operationId)
        require((operationId == null) == (completedAt == null) && (operationId == null) == (deviceOutcome == null))
        require(!acknowledged || deviceOutcome == ChecklistOutcome.COMPLETED_WITH_ISSUES)
        require(server == null || operationId != null && server.deviceOutcome == deviceOutcome)
        require(completedAt == null || !completedAt.isBefore(startedAt) && micro(completedAt) && values.all { it.answer != null })
    }
    /** Older than the pinned policy validity at [at] (B-BR-EMP-438). */
    fun expiredAt(at: Instant): Boolean = at.isAfter(startedAt.plusSeconds(policy.validitySeconds))
    internal fun copy(answers: List<ChecklistSavedAnswer> = values, operationId: String? = this.operationId, completedAt: Instant? = this.completedAt,
        deviceOutcome: ChecklistOutcome? = this.deviceOutcome, acknowledged: Boolean = this.acknowledged, server: ChecklistServerResult? = this.server) =
        LocalChecklistRun(runId, assetId, context, template, policy, startedAt, answers, operationId, completedAt, deviceOutcome, acknowledged, server)
    override fun toString(): String = "EnterpriseLocalChecklistRun(redacted)"
}

/** The versioned `checklist-book` index of one partition (EDR-283): cached applicable reads and the runs of this device. */
class ChecklistBook private constructor(val membershipId: String, val enrollmentId: String, applicables: List<ChecklistApplicable>,
    runs: List<LocalChecklistRun>) {
    private val applicableValues = applicables.toList()
    private val runValues = runs.toList()
    val applicables: List<ChecklistApplicable> get() = applicableValues.toList()
    val runs: List<LocalChecklistRun> get() = runValues.toList()

    init {
        EnterprisePartition.requireId(membershipId); EnterprisePartition.requireId(enrollmentId)
        require(applicableValues.size <= MAX_APPLICABLES && runValues.size <= MAX_RUNS)
        require(applicableValues.map { it.assetId to it.context }.distinct().size == applicableValues.size)
        require(runValues.map { it.runId }.distinct().size == runValues.size)
        val operations = runValues.mapNotNull { it.operationId }
        require(operations.distinct().size == operations.size && operations.none { op -> runValues.any { it.runId == op } })
        val open = runValues.filter { !it.completed }.map { it.assetId to it.context }
        require(open.distinct().size == open.size)
    }

    fun applicable(assetId: String, context: ChecklistContext): ChecklistApplicable? =
        applicableValues.firstOrNull { it.assetId == assetId && it.context == context }
    fun run(runId: String): LocalChecklistRun? = runValues.firstOrNull { it.runId == runId }
    fun openRun(assetId: String, context: ChecklistContext): LocalChecklistRun? =
        runValues.firstOrNull { !it.completed && it.assetId == assetId && it.context == context }

    private fun with(applicables: List<ChecklistApplicable> = applicableValues, runs: List<LocalChecklistRun> = runValues): ChecklistBook {
        val next = ChecklistBook(membershipId, enrollmentId, applicables, runs)
        if (next.canonicalBytes.size > MAX_BYTES) refuse(ChecklistLocalFailure.FULL)
        return next
    }
    private fun replace(run: LocalChecklistRun) = with(runs = runValues.map { if (it.runId == run.runId) run else it })

    /** A newer or equal confirmation replaces the cached read of its asset and moment; an older one changes nothing. */
    fun withApplicable(value: ChecklistApplicable): ChecklistBook {
        val existing = applicable(value.assetId, value.context)
        if (existing != null && value.evaluatedAt.isBefore(existing.evaluatedAt)) return this
        val others = applicableValues.filterNot { it.assetId == value.assetId && it.context == value.context }
        // A derived cache: past its bound the oldest confirmation leaves first.
        val kept = others.sortedBy { it.evaluatedAt }.drop(maxOf(0, others.size + 1 - MAX_APPLICABLES)).toSet()
        return with(applicables = others.filter { it in kept } + value)
    }

    /** Delivered and read runs may leave the index, keeping the latest completed run of each asset and moment. */
    private fun pruned(runs: List<LocalChecklistRun>): List<LocalChecklistRun> {
        if (runs.size < MAX_RUNS) return runs
        val latest = runs.filter { it.completed }.groupBy { it.assetId to it.context }.values.map { group -> group.maxBy { it.completedAt!! }.runId }.toSet()
        val removable = runs.filter { it.completed && it.server != null && it.runId !in latest }.sortedBy { it.completedAt }
        val drop = removable.take(runs.size + 1 - MAX_RUNS).map { it.runId }.toSet()
        return runs.filterNot { it.runId in drop }
    }

    /** A new run from the cached template; an expired run of the same asset and moment is replaced, never completed. */
    fun withStart(runId: String, assetId: String, context: ChecklistContext, at: Instant): ChecklistBook {
        require(Checklist.canonicalId(runId) && micro(at))
        if (run(runId) != null || runValues.any { it.operationId == runId }) refuse(ChecklistLocalFailure.DUPLICATE)
        val cached = applicable(assetId, context)?.takeIf { it.required } ?: refuse(ChecklistLocalFailure.NO_CHECKLIST)
        val open = openRun(assetId, context)
        if (open != null && !open.expiredAt(at)) refuse(ChecklistLocalFailure.RUN_IN_PROGRESS)
        val remaining = pruned(runValues.filterNot { it.runId == open?.runId })
        if (remaining.size >= MAX_RUNS) refuse(ChecklistLocalFailure.FULL)
        val template = cached.template!!
        val run = LocalChecklistRun(runId, assetId, context, template, cached.policy!!, at, template.items.map { ChecklistSavedAnswer(null, null) })
        return with(runs = remaining + run)
    }

    private fun open(runId: String): LocalChecklistRun {
        val run = run(runId) ?: refuse(ChecklistLocalFailure.UNKNOWN_RUN)
        if (run.completed) refuse(ChecklistLocalFailure.RUN_NOT_OPEN)
        return run
    }

    /** Keeps one answer of a run in progress; the latest answer of a position replaces the earlier one. */
    fun withAnswer(runId: String, position: Int, answer: String, note: String?, at: Instant): ChecklistBook {
        val run = open(runId)
        if (position !in 1..run.answers.size || answer !in ANSWERS || note != null && !validNote(note)) refuse(ChecklistLocalFailure.INVALID_ANSWER)
        if (run.expiredAt(at)) refuse(ChecklistLocalFailure.CHECKLIST_EXPIRED)
        val answers = run.answers.toMutableList().also { it[position - 1] = ChecklistSavedAnswer(answer, note) }
        return replace(run.copy(answers = answers))
    }

    /** The local outcome of a run in progress at [at] with the shared rule and its pinned policy (B-BR-EMP-334/438). */
    fun evaluate(runId: String, at: Instant): ChecklistEvaluation {
        val run = open(runId)
        if (at.isBefore(run.startedAt)) refuse(ChecklistLocalFailure.BEFORE_START)
        if (run.expiredAt(at)) refuse(ChecklistLocalFailure.CHECKLIST_EXPIRED)
        return try {
            Checklist.evaluate(run.template.items, run.policy.mode.name, run.policy.validitySeconds,
                run.answers.map { ChecklistResponse(it.answer, it.note ?: "", photo = false) })
        } catch (refused: ChecklistDomainException) {
            refuse(when (refused.failure) {
                ChecklistFailure.ANSWER_NOT_ALLOWED -> ChecklistLocalFailure.ANSWER_NOT_ALLOWED
                ChecklistFailure.PHOTO_REQUIRED -> ChecklistLocalFailure.PHOTO_REQUIRED
                ChecklistFailure.INCOMPLETE -> ChecklistLocalFailure.INCOMPLETE
                ChecklistFailure.INVALID -> ChecklistLocalFailure.INVALID_ANSWER
            })
        }
    }

    /** Completes the run with its operation; the acknowledgement is recorded only where the rule asks for it. */
    fun withCompletion(runId: String, operationId: String, at: Instant, acknowledged: Boolean): ChecklistBook {
        EnterprisePartition.requireId(operationId)
        val evaluation = evaluate(runId, at)
        if (run(operationId) != null || runValues.any { it.operationId == operationId }) refuse(ChecklistLocalFailure.DUPLICATE)
        if (evaluation.acknowledgementRequired && !acknowledged) refuse(ChecklistLocalFailure.ACKNOWLEDGEMENT_REQUIRED)
        require(micro(at))
        return replace(run(runId)!!.copy(operationId = operationId, completedAt = at, deviceOutcome = evaluation.outcome,
            acknowledged = evaluation.acknowledgementRequired))
    }

    /** Stores the run read of a completed run; a read of anything else is refused, never merged. */
    fun withServerResult(server: ChecklistServerRun): ChecklistBook {
        val run = requireNotNull(run(server.runId)) { "Unknown checklist run" }
        require(run.completed && server.assetId == run.assetId && server.templateId == run.template.templateId && server.context == run.context &&
            server.templateVersion == run.template.version && server.policyRevision == run.policy.revision && server.startedAt == run.startedAt &&
            server.completedAt == run.completedAt && server.acknowledged == run.acknowledged && server.result.deviceOutcome == run.deviceOutcome) {
            "Foreign checklist run read"
        }
        return replace(run.copy(server = server.result))
    }

    /** Refuses a book bound to another membership or enrollment. */
    fun requireBinding(membershipId: String, enrollmentId: String): ChecklistBook {
        if (membershipId != this.membershipId || enrollmentId != this.enrollmentId) refuse(ChecklistLocalFailure.FOREIGN)
        return this
    }

    val canonicalBytes: ByteArray get() = CanonicalJson.encode(SyncJson.Object(mapOf(
        "format" to SyncJson.Text(FORMAT), "version" to SyncJson.Integer(VERSION.toString()),
        "membership_id" to SyncJson.Text(membershipId), "enrollment_id" to SyncJson.Text(enrollmentId),
        "applicables" to SyncJson.Array(applicableValues.map(Codec::applicable)),
        "runs" to SyncJson.Array(runValues.map(Codec::run)),
    )))
    override fun toString(): String = "EnterpriseChecklistBook(redacted)"

    companion object {
        const val FORMAT = "checklist-book"
        const val VERSION = 1L
        const val MAX_APPLICABLES = 200
        const val MAX_RUNS = 200
        const val MAX_BYTES = 2 * 1024 * 1024
        fun empty(membershipId: String, enrollmentId: String): ChecklistBook = ChecklistBook(membershipId, enrollmentId, emptyList(), emptyList())

        /** Strict decode; an unknown format, version or spelling is refused, never guessed. */
        fun decode(raw: ByteArray): ChecklistBook = try {
            val root = Codec.obj(CanonicalJson.parseLarge(raw, MAX_BYTES), setOf("format", "version", "membership_id", "enrollment_id", "applicables", "runs"))
            require(Codec.text(root, "format") == FORMAT && Codec.integer(root, "version") == VERSION)
            val book = ChecklistBook(Codec.text(root, "membership_id"), Codec.text(root, "enrollment_id"),
                Codec.array(root, "applicables").map(Codec::applicable), Codec.array(root, "runs").map(Codec::run))
            require(book.canonicalBytes.contentEquals(raw))
            book
        } catch (invalid: IllegalArgumentException) { throw invalid }
        catch (_: Exception) { throw IllegalArgumentException("Invalid checklist book") }
    }

    /** The canonical v1 spelling of the book's values. */
    private object Codec {
        fun obj(value: SyncJson?, keys: Set<String>): SyncJson.Object {
            val result = value as? SyncJson.Object ?: throw IllegalArgumentException("Invalid checklist book")
            require(result.fields.keys == keys) { "Invalid checklist book" }
            return result
        }
        fun text(value: SyncJson.Object, key: String): String = (value.fields[key] as? SyncJson.Text)?.value ?: throw IllegalArgumentException()
        fun nullableText(value: SyncJson.Object, key: String): String? = if (value.fields[key] === SyncJson.Null) null else text(value, key)
        fun bool(value: SyncJson.Object, key: String): Boolean = (value.fields[key] as? SyncJson.Bool)?.value ?: throw IllegalArgumentException()
        fun integer(value: SyncJson.Object, key: String): Long {
            val lexeme = (value.fields[key] as? SyncJson.Integer)?.lexeme ?: throw IllegalArgumentException()
            return lexeme.toLong().also { require(it.toString() == lexeme) }
        }
        fun array(value: SyncJson.Object, key: String): List<SyncJson> = (value.fields[key] as? SyncJson.Array)?.values ?: throw IllegalArgumentException()
        fun instant(raw: String): Instant = EnterpriseTime.parseInstant(raw).also { require(EnterpriseTime.formatInstant(it) == raw) }
        fun stamp(at: Instant) = SyncJson.Text(EnterpriseTime.formatInstant(at))
        fun optional(value: String?): SyncJson = value?.let { SyncJson.Text(it) } ?: SyncJson.Null
        fun int(value: Long) = SyncJson.Integer(value.toString())
        fun <E : Enum<E>> enumOf(entries: List<E>, raw: String): E = entries.firstOrNull { it.name == raw } ?: throw IllegalArgumentException()

        fun template(value: ChecklistCachedTemplate): SyncJson = SyncJson.Object(mapOf(
            "template_id" to SyncJson.Text(value.templateId), "name" to SyncJson.Text(value.name), "version" to int(value.version),
            "published_at" to stamp(value.publishedAt),
            "items" to SyncJson.Array(value.items.map {
                SyncJson.Object(mapOf("label" to SyncJson.Text(it.label), "critical" to SyncJson.Bool(it.critical), "optional" to SyncJson.Bool(it.optional),
                    "photo_on_not_ok" to SyncJson.Bool(it.photoOnNotOk)))
            }),
        ))
        fun template(value: SyncJson?): ChecklistCachedTemplate {
            val t = obj(value, setOf("template_id", "name", "version", "published_at", "items"))
            return ChecklistCachedTemplate(text(t, "template_id"), text(t, "name"), integer(t, "version"), instant(text(t, "published_at")),
                array(t, "items").map { raw ->
                    val item = obj(raw, setOf("label", "critical", "optional", "photo_on_not_ok"))
                    ChecklistItem(text(item, "label"), bool(item, "critical"), bool(item, "optional"), bool(item, "photo_on_not_ok"))
                })
        }
        fun policy(value: ChecklistCachedPolicy): SyncJson = SyncJson.Object(mapOf(
            "revision" to int(value.revision), "mode" to SyncJson.Text(value.mode.name), "validity_seconds" to int(value.validitySeconds),
            "effective_at" to stamp(value.effectiveAt),
        ))
        fun policy(value: SyncJson?): ChecklistCachedPolicy {
            val p = obj(value, setOf("revision", "mode", "validity_seconds", "effective_at"))
            return ChecklistCachedPolicy(integer(p, "revision"), enumOf(PolicyMode.entries, text(p, "mode")), integer(p, "validity_seconds"),
                instant(text(p, "effective_at")))
        }
        fun applicable(value: ChecklistApplicable): SyncJson = SyncJson.Object(mapOf(
            "asset_id" to SyncJson.Text(value.assetId), "context" to SyncJson.Text(value.context.name), "match" to SyncJson.Text(value.match.name),
            "template" to (value.template?.let(::template) ?: SyncJson.Null), "policy" to (value.policy?.let(::policy) ?: SyncJson.Null),
            "evaluated_at" to stamp(value.evaluatedAt),
        ))
        fun applicable(value: SyncJson): ChecklistApplicable {
            val a = obj(value, setOf("asset_id", "context", "match", "template", "policy", "evaluated_at"))
            return ChecklistApplicable(text(a, "asset_id"), enumOf(ChecklistContext.entries, text(a, "context")), enumOf(ChecklistMatch.entries, text(a, "match")),
                a.fields["template"].takeIf { it !== SyncJson.Null }?.let(::template), a.fields["policy"].takeIf { it !== SyncJson.Null }?.let(::policy),
                instant(text(a, "evaluated_at")))
        }
        fun server(value: ChecklistServerResult): SyncJson = SyncJson.Object(mapOf(
            "revision" to int(value.revision), "server_policy_revision" to int(value.serverPolicyRevision), "received_at" to stamp(value.receivedAt),
            "device_outcome" to SyncJson.Text(value.deviceOutcome.name), "recheck_outcome" to SyncJson.Text(value.recheckOutcome.name),
            "outcome" to SyncJson.Text(value.outcome.name), "outcome_differs" to SyncJson.Bool(value.outcomeDiffers),
            "review_required" to SyncJson.Bool(value.reviewRequired), "template_superseded" to SyncJson.Bool(value.templateSuperseded),
            "issues" to SyncJson.Array(value.issues.map { int(it.toLong()) }), "critical_issues" to SyncJson.Array(value.criticalIssues.map { int(it.toLong()) }),
            "released_at" to (value.releasedAt?.let(::stamp) ?: SyncJson.Null),
        ))
        private fun positions(value: SyncJson.Object, key: String): List<Int> = array(value, key).map {
            val lexeme = (it as? SyncJson.Integer)?.lexeme ?: throw IllegalArgumentException()
            lexeme.toInt().also { n -> require(n.toString() == lexeme) }
        }
        fun server(value: SyncJson?): ChecklistServerResult {
            val s = obj(value, setOf("revision", "server_policy_revision", "received_at", "device_outcome", "recheck_outcome", "outcome", "outcome_differs",
                "review_required", "template_superseded", "issues", "critical_issues", "released_at"))
            return ChecklistServerResult(integer(s, "revision"), integer(s, "server_policy_revision"), instant(text(s, "received_at")),
                enumOf(ChecklistOutcome.entries, text(s, "device_outcome")), enumOf(ChecklistOutcome.entries, text(s, "recheck_outcome")),
                enumOf(ChecklistOutcome.entries, text(s, "outcome")), bool(s, "outcome_differs"), bool(s, "review_required"), bool(s, "template_superseded"),
                positions(s, "issues"), positions(s, "critical_issues"), nullableText(s, "released_at")?.let(::instant))
        }
        fun run(value: LocalChecklistRun): SyncJson = SyncJson.Object(mapOf(
            "run_id" to SyncJson.Text(value.runId), "asset_id" to SyncJson.Text(value.assetId), "context" to SyncJson.Text(value.context.name),
            "template" to template(value.template), "policy" to policy(value.policy), "started_at" to stamp(value.startedAt),
            "answers" to SyncJson.Array(value.answers.map { SyncJson.Object(mapOf("answer" to optional(it.answer), "note" to optional(it.note))) }),
            "operation_id" to optional(value.operationId), "completed_at" to (value.completedAt?.let(::stamp) ?: SyncJson.Null),
            "device_outcome" to optional(value.deviceOutcome?.name), "acknowledged" to SyncJson.Bool(value.acknowledged),
            "server" to (value.server?.let(::server) ?: SyncJson.Null),
        ))
        fun run(value: SyncJson): LocalChecklistRun {
            val r = obj(value, setOf("run_id", "asset_id", "context", "template", "policy", "started_at", "answers", "operation_id", "completed_at",
                "device_outcome", "acknowledged", "server"))
            return LocalChecklistRun(text(r, "run_id"), text(r, "asset_id"), enumOf(ChecklistContext.entries, text(r, "context")), template(r.fields["template"]),
                policy(r.fields["policy"]), instant(text(r, "started_at")),
                array(r, "answers").map { raw -> obj(raw, setOf("answer", "note")).let { ChecklistSavedAnswer(nullableText(it, "answer"), nullableText(it, "note")) } },
                nullableText(r, "operation_id"), nullableText(r, "completed_at")?.let(::instant),
                nullableText(r, "device_outcome")?.let { enumOf(ChecklistOutcome.entries, it) }, bool(r, "acknowledged"),
                r.fields["server"].takeIf { it !== SyncJson.Null }?.let(::server))
        }
    }
}

/** What the execution and outcome screens may state about one run (B-BR-EMP-443). */
class ChecklistRunView(val run: LocalChecklistRun, val evaluation: ChecklistEvaluation?, val expired: Boolean, val sync: EnterpriseSyncStatus?,
    val code: String?, val receivedAt: Instant?) {
    /** The items that block, by position: the critical issues of a BLOCKING local outcome. */
    val blocking: List<Int> get() = evaluation?.takeIf { it.outcome == ChecklistOutcome.BLOCKING }?.critical.orEmpty()
    /** The governing outcome: the server's once read, else the device's own. */
    val outcome: ChecklistOutcome? get() = run.server?.outcome ?: run.deviceOutcome
    /** "A empresa recalculou o resultado": the server's recheck differs from the device. */
    val differs: Boolean get() = run.server?.outcomeDiffers == true
    override fun toString(): String = "EnterpriseChecklistRunView(redacted)"
}

object ChecklistPresentation {
    /** A completed run needs its own retained queue entry; anything else is refused, never guessed. */
    fun view(run: LocalChecklistRun, entries: Map<String, EnterpriseQueueEntry>, at: Instant): ChecklistRunView {
        val operationId = run.operationId
        if (operationId == null) return ChecklistRunView(run, null, run.expiredAt(at), null, null, null)
        val entry = requireNotNull(entries[operationId]) { "Missing checklist record" }
        require(entry.operation.operationId == operationId) { "Foreign checklist record" }
        val fact = ChecklistRunOperations.read(entry.operation)
        require(fact.runId == run.runId && fact.assetId == run.assetId && fact.deviceOutcome == run.deviceOutcome) { "Foreign checklist record" }
        val evaluation = Checklist.evaluate(run.template.items, run.policy.mode.name, run.policy.validitySeconds,
            run.answers.map { ChecklistResponse(it.answer, it.note ?: "") })
        val receipt = entry.acknowledgement
        val valid = !at.isAfter(run.completedAt!!.plusSeconds(run.policy.validitySeconds))
        return ChecklistRunView(run, evaluation, !valid, EnterpriseSyncProjection.status(EnterpriseSyncProjection.item(entry)), receipt?.code,
            receipt?.receivedAt)
    }

    /** Newest start first; equal instants by run identifier. */
    fun views(book: ChecklistBook, entries: Map<String, EnterpriseQueueEntry>, at: Instant): List<ChecklistRunView> = book.runs.map { view(it, entries, at) }
        .sortedWith(compareByDescending<ChecklistRunView> { it.run.startedAt }.thenBy { it.run.runId })

    /** The latest completed DEPARTURE run of the asset still within its validity and not finally refused. */
    fun latestDeparture(book: ChecklistBook, entries: Map<String, EnterpriseQueueEntry>, assetId: String, at: Instant): ChecklistRunView? =
        book.runs.filter { it.completed && it.assetId == assetId && it.context == ChecklistContext.DEPARTURE }
            .sortedWith(compareByDescending<LocalChecklistRun> { it.completedAt }.thenBy { it.runId })
            .map { view(it, entries, at) }
            .firstOrNull { it.sync != EnterpriseSyncStatus.REJECTED }
            ?.takeIf { !it.expired }

    /** The device effect of BR-CHECK-006: a BLOCKING run disables the trip start on this device. */
    fun startBlocked(view: ChecklistRunView?): Boolean = view?.outcome == ChecklistOutcome.BLOCKING

    /**
     * EMP-TRIP-019 (B-BR-EMP-511): the departure run a trip start names — the [latestDeparture] view when it does not block; the
     * server decides the gate at receipt and the device's cached policy never does.
     */
    fun departureRun(view: ChecklistRunView?): String? = view?.takeIf { !startBlocked(it) }?.run?.runId
}
