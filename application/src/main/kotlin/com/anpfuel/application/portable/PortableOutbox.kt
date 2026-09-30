package com.anpfuel.application.portable

import com.anpfuel.domain.exception.DomainException

/**
 * Portable durable outbox for contribution commands (P12-T03).
 *
 * Pure Kotlin with zero `java.*`/Android imports so this file moves unchanged
 * into a future `commonMain` source set. One command ID carries exactly one
 * pending intent: re-enqueueing the same ID bumps its revision instead of
 * duplicating work, acknowledgements match the exact revision (stale acks are
 * ignored), retries use bounded exponential backoff on injected ticks and a
 * fresh nonce is attached per dispatch. Cancellation is terminal and
 * cooperative: [selectNext] dispatches nothing while cancelled.
 *
 * Persistence stays a native adapter concern: [toRecord]/[fromRecord] define
 * the migration-compatible record shape Room stores field-for-field.
 */
enum class OutboxState {
    QUEUED,
    IN_FLIGHT,
    FAILED,
    CANCELLED,
    ACKED,
}

data class OutboxCommand(
    val commandId: String,
    val kind: String,
    val payload: String,
    val revision: Int = 1,
    val state: OutboxState = OutboxState.QUEUED,
    val attempts: Int = 0,
    val nextEligibleTick: Long = 0L,
    val nonce: String = "",
)

object PortableOutbox {

    const val BASE_DELAY_TICKS: Long = 1000L
    const val MAX_DELAY_TICKS: Long = 3_600_000L

    /**
     * Enqueues a command. A repeated [commandId] replaces the payload in
     * place with `revision + 1` and returns to QUEUED; attempts are kept so
     * an old draft is never relabelled as a fresh first try.
     */
    fun enqueue(
        commands: List<OutboxCommand>,
        commandId: String,
        kind: String,
        payload: String,
    ): List<OutboxCommand> {
        require(commandId.isNotBlank()) { "commandId must be non-blank" }
        val existing = commands.indexOfFirst { it.commandId == commandId }
        if (existing < 0) {
            return commands + OutboxCommand(commandId, kind, payload)
        }
        val current = commands[existing]
        val bumped = current.copy(
            kind = kind,
            payload = payload,
            revision = current.revision + 1,
            state = OutboxState.QUEUED,
            nextEligibleTick = 0L,
            nonce = "",
        )
        return commands.toMutableList().also { it[existing] = bumped }
    }

    /**
     * First dispatchable command in stable FIFO order, or null when empty,
     * cancelled, everything is in flight/terminal, or no FAILED command is
     * eligible yet at [tickMillis].
     */
    fun selectNext(
        commands: List<OutboxCommand>,
        tickMillis: Long,
        isCancelled: Boolean,
    ): OutboxCommand? {
        if (isCancelled) return null
        return commands.firstOrNull { command ->
            when (command.state) {
                OutboxState.QUEUED -> true
                OutboxState.FAILED -> tickMillis >= command.nextEligibleTick
                OutboxState.IN_FLIGHT,
                OutboxState.CANCELLED,
                OutboxState.ACKED,
                -> false
            }
        }
    }

    /** Attaches a fresh nonce and marks the command dispatched. */
    fun markInFlight(command: OutboxCommand, nonce: String): OutboxCommand {
        require(nonce.isNotBlank()) { "nonce must be non-blank" }
        return command.copy(state = OutboxState.IN_FLIGHT, nonce = nonce)
    }

    /**
     * Removes the command only when both ID and revision match. A stale ack
     * for a superseded revision leaves the queue unchanged.
     */
    fun markAcked(
        commands: List<OutboxCommand>,
        commandId: String,
        revision: Int,
    ): List<OutboxCommand> {
        return commands.filterNot { it.commandId == commandId && it.revision == revision }
    }

    /**
     * Records a failed attempt with bounded exponential backoff:
     * `BASE * 2^(attempts-1)` capped at [MAX_DELAY_TICKS], overflow-safe.
     */
    fun markFailed(command: OutboxCommand, tickMillis: Long): OutboxCommand {
        val attempts = command.attempts + 1
        var delay = BASE_DELAY_TICKS
        repeat(attempts - 1) {
            delay = if (delay > MAX_DELAY_TICKS / 2) {
                MAX_DELAY_TICKS
            } else {
                (delay * 2).coerceAtMost(MAX_DELAY_TICKS)
            }
        }
        val nextEligible = if (tickMillis > Long.MAX_VALUE - delay) {
            Long.MAX_VALUE
        } else {
            tickMillis + delay
        }
        return command.copy(
            state = OutboxState.FAILED,
            attempts = attempts,
            nextEligibleTick = nextEligible,
            nonce = "",
        )
    }

    /** Terminal cancellation; a cancelled command is never selected. */
    fun cancel(commands: List<OutboxCommand>, commandId: String): List<OutboxCommand> {
        return commands.map { command ->
            if (command.commandId == commandId) {
                command.copy(state = OutboxState.CANCELLED, nonce = "")
            } else {
                command
            }
        }
    }

    /**
     * Migration-compatible record shape. Keys are the future Room column
     * contract; unknown keys are ignored on read (forward compatibility)
     * while missing/corrupt keys fail loudly (never silent data loss).
     */
    fun toRecord(command: OutboxCommand): Map<String, String> {
        return mapOf(
            "command_id" to command.commandId,
            "kind" to command.kind,
            "payload" to command.payload,
            "revision" to command.revision.toString(),
            "state" to command.state.name,
            "attempts" to command.attempts.toString(),
            "next_eligible_tick" to command.nextEligibleTick.toString(),
            "nonce" to command.nonce,
        )
    }

    fun fromRecord(record: Map<String, String>): OutboxCommand {
        fun field(name: String): String {
            return record[name] ?: throw DomainException("outbox record misses $name")
        }
        val revision = field("revision").toIntOrNull()
            ?: throw DomainException("outbox record has corrupt revision")
        val attempts = field("attempts").toIntOrNull()
            ?: throw DomainException("outbox record has corrupt attempts")
        val nextEligibleTick = field("next_eligible_tick").toLongOrNull()
            ?: throw DomainException("outbox record has corrupt next_eligible_tick")
        val state = try {
            OutboxState.valueOf(field("state"))
        } catch (e: IllegalArgumentException) {
            throw DomainException("outbox record has corrupt state")
        }
        return OutboxCommand(
            commandId = field("command_id"),
            kind = field("kind"),
            payload = field("payload"),
            revision = revision,
            state = state,
            attempts = attempts,
            nextEligibleTick = nextEligibleTick,
            nonce = record["nonce"] ?: "",
        )
    }
}
