package com.anpfuel.domain.discovery

/** Navigation identity only; a legacy identifier must be resolved before social writes. */
sealed interface StationPageIdentity {
    data class Canonical(val stationId: String) : StationPageIdentity
    data class Legacy(val cnpj: String) : StationPageIdentity
    companion object {
        fun parse(value: String): StationPageIdentity? = when {
            value.matches(Regex("[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}")) -> Canonical(value.lowercase())
            value.matches(Regex("[A-Z0-9]{12}[0-9]{2}")) -> Legacy(value)
            else -> null
        }
    }
}
