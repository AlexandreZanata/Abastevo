package com.anpfuel.domain.profile

/** Mirrors P30 frozen field policy. Server authorizes and validates again. */
object ProfileBusinessFieldsRule {
    val services = setOf("fuel", "convenience", "carwash", "tire-service", "oil-change", "restaurant", "atm", "restroom", "wifi", "parking")
    fun valid(fields: Map<String, String>): Boolean = fields.isNotEmpty() && fields.all { (key, raw) ->
        val value = raw.trim()
        when (key) {
            "services" -> value.isNotEmpty() && value.split(',').all { it.trim() in services }
            "description" -> value.isNotEmpty() && value.codePointCount(0, value.length) <= 280
            "opening_hours", "phone", "website" -> value.isNotEmpty() && value.toByteArray(Charsets.UTF_8).size <= 256
            else -> false
        }
    }
}
