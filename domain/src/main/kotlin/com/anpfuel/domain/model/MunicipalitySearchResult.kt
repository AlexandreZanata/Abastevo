package com.anpfuel.domain.model

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DataAvailability

data class MunicipalitySearchResult(
    val municipality: String,
    val state: BrazilianState,
    val dataAvailability: DataAvailability,
)
