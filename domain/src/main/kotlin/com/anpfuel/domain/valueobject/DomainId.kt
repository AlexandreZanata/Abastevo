package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import java.util.UUID

/**
 * Domain-generated entity identifier (agent core §7 — not DB autoincrement).
 */
class DomainId private constructor(
    val value: String,
) {
    init {
        if (value.isBlank()) {
            throw DomainException("DomainId must not be blank")
        }
    }

    override fun equals(other: Any?): Boolean =
        other is DomainId && value == other.value

    override fun hashCode(): Int = value.hashCode()

    override fun toString(): String = "DomainId(value=$value)"

    companion object {
        fun generate(): DomainId = DomainId(UUID.randomUUID().toString())

        fun from(value: String): DomainId = DomainId(value)

        fun forSurveyWeek(surveyWeek: SurveyWeek): DomainId {
            val key = "${surveyWeek.startDate}_${surveyWeek.endDate}"
            return DomainId(UUID.nameUUIDFromBytes(key.toByteArray()).toString())
        }
    }
}
