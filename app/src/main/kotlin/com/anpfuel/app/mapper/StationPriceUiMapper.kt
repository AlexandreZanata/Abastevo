package com.anpfuel.app.mapper

import com.anpfuel.app.ui.model.StationPriceUiModel
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.rule.StationAddressNormalizationRule
import com.anpfuel.domain.valueobject.BrazilianState
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

object StationPriceUiMapper {

    fun toUiModels(
        stations: List<StationPrice>,
        locale: Locale,
        preferredState: BrazilianState?,
        preferredMunicipality: String?,
    ): List<StationPriceUiModel> =
        stations.map { station ->
            toUiModel(
                stationPrice = station,
                locale = locale,
                preferredState = preferredState,
                preferredMunicipality = preferredMunicipality,
            )
        }

    fun toUiModel(
        stationPrice: StationPrice,
        locale: Locale,
        preferredState: BrazilianState? = null,
        preferredMunicipality: String? = null,
    ): StationPriceUiModel {
        val collectedAtLabel = stationPrice.collectedAt?.let { date ->
            DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)
                .withLocale(locale)
                .format(date)
        }

        return StationPriceUiModel(
            cnpjDigits = stationPrice.station.cnpj.digits,
            displayName = stationPrice.station.displayName(),
            brand = stationPrice.station.brand,
            address = stationPrice.station.address,
            priceFormatted = PriceFormatter.formatAmount(stationPrice.price, locale)
                ?: stationPrice.price.value.toPlainString(),
            collectedAtLabel = collectedAtLabel,
            navigationQuery = StationAddressNormalizationRule.buildNavigationQuery(
                station = stationPrice.station,
                preferredMunicipality = preferredMunicipality,
                preferredState = preferredState,
            ),
        )
    }
}
