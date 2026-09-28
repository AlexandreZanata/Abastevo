package com.anpfuel.domain.valueobject

enum class BrazilianState(
    val abbreviation: String,
    val region: BrazilianRegion,
    val displayName: String,
) {
    ACRE("AC", BrazilianRegion.NORTH, displayName = "Acre"),
    ALAGOAS("AL", BrazilianRegion.NORTHEAST, displayName = "Alagoas"),
    AMAPA("AP", BrazilianRegion.NORTH, displayName = "Amapá"),
    AMAZONAS("AM", BrazilianRegion.NORTH, displayName = "Amazonas"),
    BAHIA("BA", BrazilianRegion.NORTHEAST, displayName = "Bahia"),
    CEARA("CE", BrazilianRegion.NORTHEAST, displayName = "Ceará"),
    DISTRICT_FEDERAL("DF", BrazilianRegion.CENTRAL_WEST, displayName = "Distrito Federal"),
    ESPIRITO_SANTO("ES", BrazilianRegion.SOUTHEAST, displayName = "Espírito Santo"),
    GOIAS("GO", BrazilianRegion.CENTRAL_WEST, displayName = "Goiás"),
    MARANHAO("MA", BrazilianRegion.NORTHEAST, displayName = "Maranhão"),
    MATO_GROSSO("MT", BrazilianRegion.CENTRAL_WEST, displayName = "Mato Grosso"),
    MATO_GROSSO_DO_SUL("MS", BrazilianRegion.CENTRAL_WEST, displayName = "Mato Grosso do Sul"),
    MINAS_GERAIS("MG", BrazilianRegion.SOUTHEAST, displayName = "Minas Gerais"),
    PARA("PA", BrazilianRegion.NORTH, displayName = "Pará"),
    PARAIBA("PB", BrazilianRegion.NORTHEAST, displayName = "Paraíba"),
    PARANA("PR", BrazilianRegion.SOUTH, displayName = "Paraná"),
    PERNAMBUCO("PE", BrazilianRegion.NORTHEAST, displayName = "Pernambuco"),
    PIAUI("PI", BrazilianRegion.NORTHEAST, displayName = "Piauí"),
    RIO_DE_JANEIRO("RJ", BrazilianRegion.SOUTHEAST, displayName = "Rio de Janeiro"),
    RIO_GRANDE_DO_NORTE("RN", BrazilianRegion.NORTHEAST, displayName = "Rio Grande do Norte"),
    RIO_GRANDE_DO_SUL("RS", BrazilianRegion.SOUTH, displayName = "Rio Grande do Sul"),
    RONDONIA("RO", BrazilianRegion.NORTH, displayName = "Rondônia"),
    RORAIMA("RR", BrazilianRegion.NORTH, displayName = "Roraima"),
    SANTA_CATARINA("SC", BrazilianRegion.SOUTH, displayName = "Santa Catarina"),
    SAO_PAULO("SP", BrazilianRegion.SOUTHEAST, displayName = "São Paulo"),
    SERGIPE("SE", BrazilianRegion.NORTHEAST, displayName = "Sergipe"),
    TOCANTINS("TO", BrazilianRegion.NORTH, displayName = "Tocantins"),
    ;

    companion object {
        fun fromAbbreviation(abbreviation: String): BrazilianState? =
            entries.firstOrNull { it.abbreviation.equals(abbreviation, ignoreCase = true) }
    }
}
