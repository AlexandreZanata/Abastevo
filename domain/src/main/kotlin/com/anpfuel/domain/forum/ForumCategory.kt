package com.anpfuel.domain.forum

/**
 * Forum category taxonomy (B-BR-FOR-02 / BUC-FOR-01).
 *
 * Explicit supported categories; unknown values stay UNKNOWN
 * for forward compatibility with server taxonomy version bumps.
 */
enum class ForumCategory(val wireCode: String) {
    FUELS("FUELS"),
    ELECTRIC("ELECTRIC"),
    TRAFFIC_RULES("TRAFFIC_RULES"),
    VEHICLES_TECH("VEHICLES_TECH"),
    ROADS_MOBILITY("ROADS_MOBILITY"),
    DRIVER_ECONOMY("DRIVER_ECONOMY"),
    ABASTEVO_DATA("ABASTEVO_DATA"),
    UNKNOWN("UNKNOWN");

    companion object {
        fun fromWire(code: String?): ForumCategory {
            if (code.isNullOrBlank()) return UNKNOWN
            return entries.firstOrNull { it.wireCode.equals(code.trim(), ignoreCase = true) } ?: UNKNOWN
        }
    }
}

/**
 * Descriptive category item returned by the server taxonomy endpoint.
 */
data class ForumCategoryItem(
    val code: ForumCategory,
    val rawCode: String,
    val name: String,
    val description: String,
)

/**
 * Categories listing envelope including current taxonomy version.
 */
data class ForumCategoriesResult(
    val items: List<ForumCategoryItem>,
    val taxonomyVersion: Int,
)
