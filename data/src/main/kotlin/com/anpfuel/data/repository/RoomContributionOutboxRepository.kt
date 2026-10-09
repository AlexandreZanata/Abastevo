package com.anpfuel.data.repository

import com.anpfuel.application.portable.OutboxCommand
import com.anpfuel.application.portable.OutboxState
import com.anpfuel.application.portable.PortableOutbox
import com.anpfuel.data.local.dao.ContributionOutboxDao
import com.anpfuel.data.local.entity.ContributionOutboxEntity
import com.anpfuel.data.local.outbox.OutboxCommandMapper
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.OwnedContribution
import com.anpfuel.domain.repository.QueuedContribution
import com.anpfuel.domain.repository.PendingContribution
import com.anpfuel.domain.repository.ContributionReceipt
import com.anpfuel.domain.repository.ContributionRemoteStatus
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.exception.DomainException
import org.json.JSONObject
import com.anpfuel.data.worker.ContributionWorkScheduler
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T05 Room-backed durable outbox (BUC-003, B-BR-003/005).
 *
 * Stores the portable record shape field-for-field (no translation drift);
 * one stable command id holds one pending intent. Re-enqueue bumps the
 * revision in place and keeps attempts so an old draft is never relabelled
 * as a fresh first try; stale acks (revision mismatch) never delete the
 * newer revision. Process death survives: state lives in Room, never RAM.
 */
@Singleton
class RoomContributionOutboxRepository @Inject constructor(
    private val dao: ContributionOutboxDao,
    private val scheduler: ContributionWorkScheduler? = null,
) : ContributionOutboxRepository {

    override suspend fun enqueueReview(commands: List<PendingContribution>): List<QueuedContribution> {
        val rows = commands.map { toEntity(OutboxCommand(it.draft.clientSubmissionId, KIND_SUBMIT, it.payloadJson)) }
        val result = dao.freezeReview(rows).map { toQueued(toCommand(it)) }
        result.forEach { runCatching { scheduler?.enqueueContribution(it.commandId) } }
        return result
    }

    override suspend fun claimDispatch(commandId: String, revision: Int, nonce: String, nowMillis: Long): Boolean =
        dao.claim(commandId, revision, nonce, nowMillis, nowMillis + DISPATCH_LEASE_MILLIS) == 1

    override suspend fun recordReceipt(receipt: ContributionReceipt, nowMillis: Long) {
        if (receipt.status == ContributionRemoteStatus.QUEUED ||
            (receipt.observationId.isNullOrBlank() && receipt.status !in setOf(ContributionRemoteStatus.EXPIRED, ContributionRemoteStatus.REJECTED))) {
            throw DomainException("invalid contribution receipt")
        }
        val next = if (receipt.status in setOf(ContributionRemoteStatus.RECEIVED, ContributionRemoteStatus.VALIDATING)) nowMillis + STATUS_POLL_MILLIS else Long.MAX_VALUE
        dao.saveReceipt(receipt.commandId, receipt.revision, receipt.observationId, receipt.status.name, receipt.reason, next)
    }

    override suspend fun receipt(commandId: String): ContributionReceipt? = dao.findById(commandId)?.let { row ->
        row.remoteStatus?.let { status ->
            try { ContributionReceipt(row.commandId, row.revision, ContributionRemoteStatus.valueOf(status), row.observationId, row.failureReason) }
            catch (_: IllegalArgumentException) { throw DomainException("corrupt contribution receipt") }
        }
    }

    override suspend fun failDispatch(commandId: String, revision: Int, nonce: String, nowMillis: Long) {
        val row = dao.findById(commandId) ?: return
        if (row.revision != revision || row.nonce != nonce || row.state != OutboxState.IN_FLIGHT.name) return
        val failed = PortableOutbox.markFailed(toCommand(row), nowMillis)
        dao.failClaim(commandId, revision, nonce, failed.attempts, failed.nextEligibleTick)
    }

    override suspend fun enqueue(
        draft: ContributionDraft,
        payloadJson: String,
    ): QueuedContribution {
        // Production scoped intents are frozen; the old revision adapter is quarantined legacy data.
        if (JSONObject(payloadJson).optJSONObject("scope") != null) {
            return enqueueReview(listOf(PendingContribution(draft, payloadJson))).single()
        }
        val commandId = draft.clientSubmissionId
        val existing = dao.findById(commandId)?.let(::toCommand)
        val next: OutboxCommand = if (existing == null) {
            OutboxCommand(
                commandId = commandId,
                kind = KIND_SUBMIT,
                payload = payloadJson,
            )
        } else {
            val bumped = PortableOutbox.enqueue(
                listOf(existing),
                commandId,
                KIND_SUBMIT,
                payloadJson,
            ).first()
            bumped
        }
        dao.upsert(toEntity(next))
        runCatching { scheduler?.enqueueContribution(next.commandId) }
        return toQueued(next)
    }

    override suspend fun listDispatchable(nowMillis: Long): List<QueuedContribution> {
        return dao.listAll().filter { row ->
            when (toCommand(row).state) {
                OutboxState.QUEUED -> true
                OutboxState.FAILED, OutboxState.IN_FLIGHT -> nowMillis >= row.nextEligibleTick
                OutboxState.ACKED -> row.remoteStatus in setOf("RECEIVED", "VALIDATING") && nowMillis >= row.nextEligibleTick
                OutboxState.CANCELLED -> false
            }
        }.map { toQueued(toCommand(it)) }
    }

    override suspend fun loadPayload(commandId: String): String? =
        dao.findById(commandId)?.payload

    override suspend fun markDispatched(commandId: String, nonce: String) {
        val current = dao.findById(commandId)?.let(::toCommand) ?: return
        val inFlight = PortableOutbox.markInFlight(current, nonce)
        dao.upsert(toEntity(inFlight))
    }

    override suspend fun markAcknowledged(commandId: String, revision: Int) {
        val current = dao.findById(commandId) ?: return
        if (current.revision != revision || current.remoteStatus != null) return
        dao.deleteById(commandId)
    }

    override suspend fun markFailed(commandId: String, nowMillis: Long) {
        val current = dao.findById(commandId)?.let(::toCommand) ?: return
        val failed = PortableOutbox.markFailed(current, nowMillis)
        dao.upsert(toEntity(failed))
    }

    override suspend fun cancel(commandId: String) {
        if (dao.cancelPending(commandId) != 1) throw DomainException("contribution is already sending or received")
        scheduler?.cancelContribution(commandId)
    }

    override suspend fun listOwned(): List<OwnedContribution> =
        dao.listAll().map { entity ->
            if (entity.state == "ACKED" && entity.remoteStatus == null) throw DomainException("acknowledged command lacks receipt")
            val scope = try { JSONObject(entity.payload).optJSONObject("scope")?.let {
                ContributionScope(it.getString("owner_scope"), it.getString("origin"))
            } } catch (_: Exception) { null }
            OwnedContribution(
                commandId = entity.commandId,
                revision = entity.revision,
                attempts = entity.attempts,
                phase = OwnedContribution.phaseOf(entity.state),
                scope = scope,
                remoteStatus = entity.remoteStatus?.let { ContributionRemoteStatus.valueOf(it) },
                reason = entity.failureReason,
            )
        }

    private fun toCommand(entity: ContributionOutboxEntity): OutboxCommand =
        OutboxCommandMapper.fromEntityFields(
            mapOf(
                "command_id" to entity.commandId,
                "kind" to entity.kind,
                "payload" to entity.payload,
                "revision" to entity.revision.toString(),
                "state" to entity.state,
                "attempts" to entity.attempts.toString(),
                "next_eligible_tick" to entity.nextEligibleTick.toString(),
                "nonce" to entity.nonce,
            ),
        )

    private fun toEntity(command: OutboxCommand): ContributionOutboxEntity {
        val fields = OutboxCommandMapper.toEntityFields(command)
        return ContributionOutboxEntity(
            commandId = fields.getValue("command_id"),
            kind = fields.getValue("kind"),
            payload = fields.getValue("payload"),
            revision = fields.getValue("revision").toInt(),
            state = fields.getValue("state"),
            attempts = fields.getValue("attempts").toInt(),
            nextEligibleTick = fields.getValue("next_eligible_tick").toLong(),
            nonce = fields.getValue("nonce"),
        )
    }

    private fun toQueued(command: OutboxCommand): QueuedContribution =
        QueuedContribution(
            commandId = command.commandId,
            revision = command.revision,
            payloadJson = command.payload,
            historical = command.payload.contains("\"freshness\":\"historical\""),
        )

    companion object {
        const val DISPATCH_LEASE_MILLIS = 300_000L
        const val STATUS_POLL_MILLIS = 30_000L
        const val KIND_SUBMIT = "contribution.submit"
    }
}
