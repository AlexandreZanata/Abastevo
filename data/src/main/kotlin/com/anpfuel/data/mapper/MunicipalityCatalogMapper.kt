package com.anpfuel.data.mapper

import com.anpfuel.data.local.entity.MunicipalityCatalogEntity
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry

object MunicipalityCatalogMapper {

    fun toDomain(entity: MunicipalityCatalogEntity): MunicipalityCatalogEntry? {
        val state = BrazilianState.fromAbbreviation(entity.state) ?: return null
        return MunicipalityCatalogEntry(
            state = state,
            municipality = entity.municipality,
            ibgeCode = entity.ibgeCode,
        )
    }
}
