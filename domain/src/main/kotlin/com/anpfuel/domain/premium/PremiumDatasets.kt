package com.anpfuel.domain.premium

/**
 * PRM09-T01 typed dataset and value vocabularies.
 *
 * Mirrors the frozen backend contracts (`contracts/openapi/v1.yaml`):
 * cloud datasets `vehicles|favorites|preferences`, ledger datasets
 * `refuel|expense`, seven expense categories, ledger fuels and units.
 * Unknown wire values must be refused, never defaulted, so old clients
 * fail safely on version changes.
 */
enum class CloudDataset(val wire: String) {
    VEHICLES("vehicles"),
    FAVORITES("favorites"),
    PREFERENCES("preferences"),
    ;

    companion object {
        fun parse(wire: String): CloudDataset =
            entries.firstOrNull { it.wire == wire }
                ?: throw PremiumValidationException("unknown-dataset")
    }
}

enum class LedgerDataset(val wire: String) {
    REFUEL("refuel"),
    EXPENSE("expense"),
    CHARGE("charge"),
    ;

    companion object {
        fun parse(wire: String): LedgerDataset =
            entries.firstOrNull { it.wire == wire }
                ?: throw PremiumValidationException("unknown-dataset")
    }
}

enum class ExpenseCategory(val wire: String) {
    MAINTENANCE("maintenance"),
    INSURANCE("insurance"),
    TAXES("taxes"),
    PARKING("parking"),
    TOLLS("tolls"),
    CLEANING("cleaning"),
    OTHER("other"),
    ;

    companion object {
        fun parse(wire: String): ExpenseCategory =
            entries.firstOrNull { it.wire == wire }
                ?: throw PremiumValidationException("unknown-category")
    }
}

enum class PremiumFuel(val wire: String, val unit: PremiumUnit) {
    ETHANOL("ETHANOL", PremiumUnit.LITRE),
    GASOLINE_REGULAR("GASOLINE_REGULAR", PremiumUnit.LITRE),
    GASOLINE_ADDITIVED("GASOLINE_ADDITIVED", PremiumUnit.LITRE),
    GASOLINE_PREMIUM_GRADE("GASOLINE_PREMIUM_GRADE", PremiumUnit.LITRE),
    DIESEL_S500("DIESEL_S500", PremiumUnit.LITRE),
    DIESEL_S10("DIESEL_S10", PremiumUnit.LITRE),
    CNG("CNG", PremiumUnit.CUBIC_METRE),
    ;

    companion object {
        fun parse(wire: String): PremiumFuel =
            entries.firstOrNull { it.wire == wire }
                ?: throw PremiumValidationException("unknown-fuel")
    }
}

enum class PremiumUnit(val wire: String) {
    LITRE("L"),
    CUBIC_METRE("M3"),
    ;

    companion object {
        fun parse(wire: String): PremiumUnit =
            entries.firstOrNull { it.wire == wire }
                ?: throw PremiumValidationException("unknown-unit")
    }
}

class PremiumValidationException(
    val code: String,
    message: String = code,
) : RuntimeException(message)
