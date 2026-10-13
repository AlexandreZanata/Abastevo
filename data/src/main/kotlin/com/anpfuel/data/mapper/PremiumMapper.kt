package com.anpfuel.data.mapper

import com.anpfuel.data.local.entity.PremiumCursorEntity
import com.anpfuel.data.local.entity.PremiumOutboxEntity
import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumCursor
import com.anpfuel.domain.premium.PremiumOwnerScope

/**
 * PRM09-T01 typed mapping between durable Room rows and domain intents.
 *
 * Owner/origin travel with every row; mapping refuses cross-owner reuse.
 * Payloads stay opaque JSON: this layer never interprets dataset fields,
 * so unknown future versions fail at the typed dataset parser, not here.
 */
object PremiumMapper {

    fun toEntity(command: PremiumCommand, state: String = "QUEUED"): PremiumOutboxEntity =
        PremiumOutboxEntity(
            commandId = command.operationId,
            ownerAccount = command.scope.accountId,
            origin = command.scope.origin,
            dataset = command.dataset,
            recordId = command.recordId,
            expectedRevision = command.expectedRevision,
            payloadJson = command.payloadJson,
            deleteFlag = command.delete,
            voidFlag = command.void,
            state = state,
            attempts = 0,
            nextEligibleTick = 0L,
            nonce = "",
        )

    fun toDomain(entity: PremiumOutboxEntity): PremiumCommand =
        PremiumCommand(
            operationId = entity.commandId,
            scope = PremiumOwnerScope(entity.ownerAccount, entity.origin),
            dataset = entity.dataset,
            recordId = entity.recordId,
            expectedRevision = entity.expectedRevision,
            delete = entity.deleteFlag,
            void = entity.voidFlag,
            payloadJson = entity.payloadJson,
        )

    fun cursorToEntity(
        scope: PremiumOwnerScope,
        dataset: String,
        cursor: PremiumCursor,
        epoch: Long,
        nowMillis: Long,
    ): PremiumCursorEntity =
        PremiumCursorEntity(
            ownerAccount = scope.accountId,
            origin = scope.origin,
            dataset = dataset,
            cursor = cursor.raw,
            epoch = epoch,
            updatedAtMillis = nowMillis,
        )

    fun cursorToDomain(entity: PremiumCursorEntity): PremiumCursor =
        PremiumCursor.parse(entity.cursor)
}
