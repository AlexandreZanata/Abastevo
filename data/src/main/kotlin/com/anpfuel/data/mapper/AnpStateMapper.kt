package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.BrazilianState
import java.text.Normalizer

/**
 * Maps ANP state labels (abbreviation or Portuguese name) to [BrazilianState].
 */
object AnpStateMapper {

    private val DIACRITIC_MARKS_REGEX = Regex("\\p{M}+")

    /**
     * Official Brazilian state names as published in ANP spreadsheets (Portuguese).
     */
    private val ANP_PORTUGUESE_LABELS: Map<String, BrazilianState> = mapOf(
        "Acre" to BrazilianState.ACRE,
        "Alagoas" to BrazilianState.ALAGOAS,
        "Amapá" to BrazilianState.AMAPA,
        "Amazonas" to BrazilianState.AMAZONAS,
        "Bahia" to BrazilianState.BAHIA,
        "Ceará" to BrazilianState.CEARA,
        "Distrito Federal" to BrazilianState.DISTRICT_FEDERAL,
        "Espírito Santo" to BrazilianState.ESPIRITO_SANTO,
        "Goiás" to BrazilianState.GOIAS,
        "Maranhão" to BrazilianState.MARANHAO,
        "Mato Grosso" to BrazilianState.MATO_GROSSO,
        "Mato Grosso do Sul" to BrazilianState.MATO_GROSSO_DO_SUL,
        "Minas Gerais" to BrazilianState.MINAS_GERAIS,
        "Pará" to BrazilianState.PARA,
        "Paraíba" to BrazilianState.PARAIBA,
        "Paraná" to BrazilianState.PARANA,
        "Pernambuco" to BrazilianState.PERNAMBUCO,
        "Piauí" to BrazilianState.PIAUI,
        "Rio de Janeiro" to BrazilianState.RIO_DE_JANEIRO,
        "Rio Grande do Norte" to BrazilianState.RIO_GRANDE_DO_NORTE,
        "Rio Grande do Sul" to BrazilianState.RIO_GRANDE_DO_SUL,
        "Rondônia" to BrazilianState.RONDONIA,
        "Roraima" to BrazilianState.RORAIMA,
        "Santa Catarina" to BrazilianState.SANTA_CATARINA,
        "São Paulo" to BrazilianState.SAO_PAULO,
        "Sergipe" to BrazilianState.SERGIPE,
        "Tocantins" to BrazilianState.TOCANTINS,
    )

    private val labelToState: Map<String, BrazilianState> =
        BrazilianState.entries.associate { normalizeLabel(it.abbreviation) to it } +
            ANP_PORTUGUESE_LABELS.mapKeys { (label, _) -> normalizeLabel(label) }

    fun toBrazilianState(rawLabel: String): BrazilianState? =
        labelToState[normalizeLabel(rawLabel)]

    fun toBrazilianStateOrThrow(rawLabel: String): BrazilianState =
        toBrazilianState(rawLabel)
            ?: error("Unknown ANP state label: $rawLabel")

    fun toAbbreviation(rawLabel: String): String = toBrazilianStateOrThrow(rawLabel).abbreviation

    fun supportedPortugueseLabels(): Set<String> = ANP_PORTUGUESE_LABELS.keys

    private fun normalizeLabel(raw: String): String =
        Normalizer.normalize(raw.trim(), Normalizer.Form.NFD)
            .replace(DIACRITIC_MARKS_REGEX, "")
            .uppercase()
}
