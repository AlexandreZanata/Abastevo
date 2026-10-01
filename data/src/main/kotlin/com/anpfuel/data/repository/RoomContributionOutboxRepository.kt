package com.anpfuel.data.repository

import com.anpfuel.application.portable.OutboxCommand
import com.anpfuel.application.portable.OutboxState
import com.anpfuel.application.portable.PortableOutbox
import com.anpfuel.data.local.dao.ContributionOutboxDao
import com.anpfuel.data.local.entity.ContributionOutboxEntity
import com.anpfuel.data.local.outbox.OutboxCommandMapper
import com.anpfuel.domain.model.ContributionDraft
import com.anpfuel.domain.repository.ContributionOutboxRepository
import com.anpfuel.domain.repository.QueuedContribution
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
) : ContributionOutboxRepository {

    override suspend fun enqueue(
        draft: ContributionDraft,
        payloadJson: String,
    ): QueuedContribution {
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
        return toQueued(next)
    }

    override suspend fun listDispatchable(nowMillis: Long): List<QueuedContribution> {
        val commands = dao.listAll().map(::toCommand)
        return commands.filter { command ->
            when (command.state) {
                OutboxState.QUEUED -> true
                OutboxState.FAILED -> nowMillis >= command.nextEligibleTick
                OutboxState.IN_FLIGHT,
                OutboxState.CANCELLED,
                OutboxState.ACKED,
                -> false
            }
        }.map(::toQueued)
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
        if (current.revision != revision) return
        dao.deleteById(commandId)
    }

    override suspend fun markFailed(commandId: String, nowMillis: Long) {
        val current = dao.findById(commandId)?.let(::toCommand) ?: return
        val failed = PortableOutbox.markFailed(current, nowMillis)
        dao.upsert(toEntity(failed))
    }

    override suspend fun cancel(commandId: String) {
        val current = dao.findById(commandId)?.let(::toCommand) ?: return
        val cancelled = PortableOutbox.cancel(listOf(current), commandId).first()
        dao.upsert(toEntity(cancelled))
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
        const val KIND_SUBMIT = "contribution.submit"
    }
}
