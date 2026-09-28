package com.anpfuel.app.mapper

import androidx.annotation.StringRes
import com.anpfuel.app.R
import com.anpfuel.domain.valueobject.DataAvailability

object DataAvailabilityI18n {
    @StringRes fun toSearchSubtitleStringRes(availability: DataAvailability): Int? = when (availability) {
        DataAvailability.NO_DATA_THIS_WEEK -> R.string.search_no_data_this_week
        else -> null
    }
    @StringRes fun toLocationSubtitleStringRes(availability: DataAvailability): Int? = when (availability) {
        DataAvailability.NO_DATA_THIS_WEEK -> R.string.location_no_data_this_week
        else -> null
    }
    @StringRes fun toEmptyStateStringRes(availability: DataAvailability): Int = when (availability) {
        DataAvailability.NO_DATA_THIS_WEEK -> R.string.prices_empty_no_data_this_week
        DataAvailability.NEVER_IN_ANP -> R.string.prices_empty_never_in_anp
        DataAvailability.HAS_DATA -> R.string.prices_empty_municipality
    }
}
