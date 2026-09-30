package com.anpfuel.data.local.outbox

import com.anpfuel.application.portable.OutboxCommand
import com.anpfuel.application.portable.PortableOutbox

/**
 * Native persistence boundary for the portable outbox (P12-T03).
 *
 * Pure Kotlin with zero Android/Room imports: the adapter stores the portable
 * record shape field-for-field, so the future Room entity persists exactly
 * these columns with no translation drift. No entity or migration changes in
 * this task; this mapper freezes the column contract the entity must honor.
 */
object OutboxCommandMapper {

    fun toEntityFields(command: OutboxCommand): Map<String, String> {
        return PortableOutbox.toRecord(command)
    }

    fun fromEntityFields(fields: Map<String, String>): OutboxCommand {
        return PortableOutbox.fromRecord(fields)
    }
}
