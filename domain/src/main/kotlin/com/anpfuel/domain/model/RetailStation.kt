package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj

/**
 * A licensed fuel retail point identified by CNPJ.
 */
class RetailStation private constructor(
    val cnpj: Cnpj,
    val legalName: String?,
    val tradeName: String?,
    val address: String,
    val municipality: String,
    val state: BrazilianState,
    val brand: String?,
) {
    init {
        if (address.isBlank()) {
            throw DomainException("RetailStation address must not be blank")
        }
        if (municipality.isBlank()) {
            throw DomainException("RetailStation municipality must not be blank")
        }
    }

    fun displayName(): String =
        tradeName?.takeIf { it.isNotBlank() }
            ?: legalName?.takeIf { it.isNotBlank() }
            ?: cnpj.formatted()

    fun matchesLocation(state: BrazilianState, municipality: String): Boolean =
        this.state == state && this.municipality.equals(municipality, ignoreCase = true)

    companion object {
        fun create(
            cnpj: Cnpj,
            legalName: String?,
            tradeName: String?,
            address: String,
            municipality: String,
            state: BrazilianState,
            brand: String?,
        ): RetailStation = RetailStation(
            cnpj = cnpj,
            legalName = legalName?.trim(),
            tradeName = tradeName?.trim(),
            address = address.trim(),
            municipality = municipality.trim(),
            state = state,
            brand = brand?.trim(),
        )
    }
}
