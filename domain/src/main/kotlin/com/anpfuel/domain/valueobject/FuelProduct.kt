package com.anpfuel.domain.valueobject

enum class FuelProduct {
    ETHANOL,
    GASOLINE_REGULAR,
    GASOLINE_PREMIUM,
    DIESEL_S500,
    DIESEL_S10,
    CNG,
    LPG_P13,
    // True premium grade; GASOLINE_PREMIUM remains the persisted additive legacy.
    GASOLINE_PREMIUM_GRADE,
}
