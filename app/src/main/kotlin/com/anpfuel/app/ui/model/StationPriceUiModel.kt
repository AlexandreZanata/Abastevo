package com.anpfuel.app.ui.model

data class StationPriceUiModel(
    val cnpjDigits: String,
    val displayName: String,
    val brand: String?,
    val address: String,
    val priceFormatted: String,
    val collectedAtLabel: String?,
    val navigationQuery: String,
)
